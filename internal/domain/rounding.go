package domain

// # Rounding in the Pricing Engine
//
// Rounding is treated as a first-class part of pricing logic, NOT a UI formatting step.
// Rounding differences directly affect:
//   - The payable amount charged to the customer
//   - Tax correctness (taxable base must be rounded consistently)
//   - Reconciliation with payment providers (who enforce currency precision)
//   - Invoice and ledger consistency (rounded totals must match across systems)
//
// Rounding is executed as a dedicated pipeline stage (see stages/rounding.go), placed
// near the end of the pipeline — after discounts, fees, and tax are calculated — so that
// rounding is applied to the final payable amount in a controlled and deterministic way.
//
// Any rounding delta is persisted as an explicit ROUNDING adjustment (see Adjustment in
// adjustment.go), making it auditable and reproducible. Without this, historical totals
// may not reconcile correctly when recalculated later.

// RoundingMethod controls how a value is snapped to the nearest increment.
//
// The choice of method affects financial accuracy and compliance:
//   - HALF_UP: most common default; 0.5 rounds away from zero
//   - HALF_EVEN: "banker's rounding"; eliminates statistical bias in large datasets
//   - FLOOR: always rounds toward zero (conservative)
//   - CEIL: always rounds away from zero (liberal)
type RoundingMethod string

const (
	RoundHalfUp   RoundingMethod = "HALF_UP"   // 0.5 rounds away from zero (most common)
	RoundHalfEven RoundingMethod = "HALF_EVEN" // Banker's rounding: 0.5 rounds to nearest even
	RoundFloor    RoundingMethod = "FLOOR"     // Always round toward zero
	RoundCeil     RoundingMethod = "CEIL"      // Always round away from zero
)

// RoundingScope controls at which granularity rounding is applied.
//
// The scope must be explicitly defined because different jurisdictions and accounting
// practices require different approaches:
//   - ORDER_TOTAL: round the final payable amount once (simplest, most common)
//   - PER_ITEM: round each item's FinalTotal individually (may accumulate small differences)
//   - PER_TAX: round each individual tax component (required by some tax jurisdictions)
type RoundingScope string

const (
	RoundOrderTotal RoundingScope = "ORDER_TOTAL" // Round the final payable order total
	RoundPerItem    RoundingScope = "PER_ITEM"    // Round each item's FinalTotal
	RoundPerTax     RoundingScope = "PER_TAX"     // Round each individual tax component
)

// RoundingPolicy is the full configuration for the rounding pipeline stage.
//
// This policy is configurable per market/currency and drives the behavior of the
// RoundingStage (see stages/rounding.go). The policy ID and Version are persisted
// in the ROUNDING adjustment's metadata for full traceability — so any historical
// order can be traced back to the exact rounding policy that was used.
//
// # Configuration Examples
//
//   - EUR standard: Method=HALF_UP, Scope=ORDER_TOTAL, Increment=1 (nearest cent)
//   - CHF market:   Method=HALF_UP, Scope=ORDER_TOTAL, Increment=5 (nearest 5 Rappen)
//   - JPY market:   Method=HALF_UP, Scope=ORDER_TOTAL, Increment=1 (nearest yen, 0 decimals)
//   - Tax-strict:   Method=HALF_EVEN, Scope=PER_TAX, Increment=1 (banker's rounding per tax line)
type RoundingPolicy struct {
	// ID and Version identify this policy configuration for traceability.
	// These values are stored in the ROUNDING adjustment's metadata so that
	// any order can be traced to the exact policy that produced the rounding delta.
	ID      string
	Version string

	Method RoundingMethod // How to round (HALF_UP, HALF_EVEN, FLOOR, CEIL)
	Scope  RoundingScope  // What to round (ORDER_TOTAL, PER_ITEM, PER_TAX)

	// Increment is the smallest unit to round to, expressed in minor currency units.
	// Examples:
	//   1  → nearest cent (EUR standard)
	//   5  → nearest 5 cents (CHF, some Scandinavian markets)
	//   10 → nearest 10 cents
	// A value ≤ 1 is treated as 1 (no-op for integer arithmetic already in minor units).
	Increment int64
}

// ApplyRounding snaps amount to the nearest multiple of increment using the given method.
// Both amount and increment are in minor currency units (int64).
// Returns the rounded value; the caller can compute delta as: returned − amount.
//
// This is a pure function with no side effects. It is used by the RoundingStage
// (see stages/rounding.go) to compute rounding deltas which are then persisted
// as ROUNDING adjustments for auditability.
//
// # Algorithm
//
// 1. If increment ≤ 1, return amount unchanged (no rounding needed for minor units).
// 2. Decompose amount into sign × absolute value for consistent math.
// 3. Compute remainder = abs % increment.
// 4. If remainder is 0, amount is already aligned — return unchanged.
// 5. Apply the rounding method to determine whether to round down (base) or up (base + increment).
// 6. Reapply the original sign.
func ApplyRounding(amount, increment int64, method RoundingMethod) int64 {
	if increment <= 1 {
		return amount
	}

	// Work with the absolute value so the math is consistent; reapply sign at the end.
	sign := int64(1)
	abs := amount
	if amount < 0 {
		sign = -1
		abs = -amount
	}

	remainder := abs % increment
	if remainder == 0 {
		return amount
	}

	base := abs - remainder

	var rounded int64
	switch method {
	case RoundHalfUp:
		// If the remainder is at least half the increment, round up; otherwise round down.
		if remainder*2 >= increment {
			rounded = base + increment
		} else {
			rounded = base
		}
	case RoundHalfEven:
		if remainder*2 == increment {
			// Exactly halfway — round to nearest even multiple (banker's rounding).
			// This eliminates systematic bias when rounding large numbers of transactions.
			if (base/increment)%2 == 0 {
				rounded = base
			} else {
				rounded = base + increment
			}
		} else if remainder*2 > increment {
			rounded = base + increment
		} else {
			rounded = base
		}
	case RoundCeil:
		// Always round up (away from zero).
		rounded = base + increment
	case RoundFloor:
		// Always round down (toward zero).
		rounded = base
	default:
		// Unknown method — return unmodified absolute value.
		rounded = abs
	}

	return sign * rounded
}