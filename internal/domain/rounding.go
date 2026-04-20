package domain

// RoundingMethod controls how a value is snapped to the nearest increment.
type RoundingMethod string

const (
	RoundHalfUp   RoundingMethod = "HALF_UP"   // 0.5 rounds away from zero
	RoundHalfEven RoundingMethod = "HALF_EVEN" // Banker's rounding: 0.5 rounds to nearest even
	RoundFloor    RoundingMethod = "FLOOR"      // Always round toward zero
	RoundCeil     RoundingMethod = "CEIL"       // Always round away from zero
)

// RoundingScope controls at which granularity rounding is applied.
type RoundingScope string

const (
	RoundOrderTotal RoundingScope = "ORDER_TOTAL" // Round the final payable order total
	RoundPerItem    RoundingScope = "PER_ITEM"    // Round each item's FinalTotal
	RoundPerTax     RoundingScope = "PER_TAX"     // Round each individual tax component
)

// RoundingPolicy is the full configuration for the rounding stage.
type RoundingPolicy struct {
	// ID and Version identify this policy configuration for traceability in adjustment metadata.
	ID      string
	Version string

	Method RoundingMethod
	Scope  RoundingScope

	// Increment is the smallest unit to round to, expressed in minor currency units.
	// Examples:
	//   1  → nearest cent (EUR standard)
	//   5  → nearest 5 cents (CHF, some Scandinavian markets)
	//   10 → nearest 10 cents
	// A value ≤ 1 is treated as 1 (no-op for integer arithmetic already in minor units).
	Increment int64
}

// ApplyRounding snaps amount to the nearest multiple of increment using the given method.
// amount and increment are both in minor currency units (int64).
// Returns the rounded value; delta = returned value − amount.
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
		if remainder*2 >= increment {
			rounded = base + increment
		} else {
			rounded = base
		}
	case RoundHalfEven:
		if remainder*2 == increment {
			// Exactly halfway — round to nearest even multiple.
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
		rounded = base + increment
	case RoundFloor:
		rounded = base
	default:
		rounded = abs
	}

	return sign * rounded
}
