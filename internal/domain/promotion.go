package domain

import "time"

// Promotion represents a configurable pricing rule consisting of conditions and benefits.
//
// # Promotion Evaluation Model (Conditions + Benefits)
//
// Promotions are modeled as a rule system where:
//   - Conditions: eligibility checks that determine whether the promotion applies
//     (e.g., minimum subtotal, SKU presence, user type, time window, coupon required)
//   - Benefits: adjustment generators that produce Adjustments when the promotion is applied
//     (e.g., percent off, fixed discount, buy X get Y, free delivery)
//
// # Evaluation Order and Determinism
//
// Promotions are applied deterministically during the apply_promotions pipeline stage:
//  1. Sorted by Priority (higher priority first)
//  2. Checked for validity window (ValidFrom/ValidTo vs current time)
//  3. Checked for coupon requirement (RequiresCoupon + Code matching)
//  4. Checked for exclusivity group conflicts (only one promo per Group can apply)
//  5. All Conditions are evaluated; all must pass for the promotion to apply
//  6. If all checks pass, Benefits are applied, generating item-level and/or order-level adjustments
//
// # Stacking and Exclusivity
//
//   - Stackable=true: allows additional promotions to be evaluated after this one applies
//   - Stackable=false: stops promotion evaluation after this promotion applies (acts as a "best offer")
//   - Group: exclusivity group name; only one promotion within the same group can apply.
//     This prevents conflicting discounts (e.g., two different "ORDER_DISCOUNT" promotions).
//     Leave empty ("") if the promotion has no exclusivity constraints.
//
// # Traceability
//
// Every promotion evaluation (whether applied or skipped) is recorded in the promotion trace
// (see engine.PromoTrace and domain.PromotionTrace). The trace includes the promotion ID, code,
// status (APPLIED/SKIPPED), and reason for skipping. This is invaluable for:
//   - Debugging pricing disputes
//   - Customer support explanations ("Why didn't my coupon work?")
//   - Validating promotion configuration
//
// # Adding New Promotion Types
//
// To add a new promotion type:
//  1. Implement a new Condition type (if new eligibility logic is needed) — see promos/conditions.go
//  2. Implement a new Benefit type (for the new discount/fee logic) — see promos/benefits.go
//  3. Configure a Promotion with the new condition/benefit — no engine changes required
//
// This extensibility is a key design goal: new pricing features are added by implementing
// new condition/benefit types, without modifying the core engine or persistence format.
type Promotion struct {
	ID        string // Unique promotion identifier for tracing and debugging
	Code      string // Promotion code; also used to match against coupon codes
	Priority  int    // Higher priority = evaluated first; deterministic ordering
	Stackable bool   // If false, no further promotions are evaluated after this one applies

	// Group defines the exclusivity group. Only one promotion in the same group can apply
	// per calculation. Examples: "ORDER_DISCOUNT", "DELIVERY". Leave empty for no constraint.
	Group string

	// RequiresCoupon indicates whether the customer must present a coupon code matching
	// this promotion's Code field for the promotion to apply.
	RequiresCoupon bool

	ValidFrom time.Time // Start of the validity window (inclusive)
	ValidTo   time.Time // End of the validity window (inclusive)

	Conditions []Condition // All conditions must pass for the promotion to apply
	Benefits   []Benefit   // Adjustment generators invoked when the promotion applies
}

// Condition is an eligibility check for a promotion.
//
// Each Condition evaluates the current cart and snapshot state to determine whether
// a promotion should apply. If the condition fails, it returns false along with a
// human/machine-readable reason string that is recorded in the promotion trace.
//
// Implementations are in the promos package:
//   - MinSubtotalCondition: checks if the order subtotal meets a minimum threshold
//   - HasSKUCondition: checks if a specific SKU is present in the cart
//
// To add new conditions (e.g., user-type check, geo-restriction, time-of-day),
// implement this interface and attach it to a Promotion's Conditions slice.
type Condition interface {
	// Evaluate checks whether this condition is met.
	// Returns (true, "ok") if passed, or (false, "<reason>") if not.
	// The reason string is persisted in the promotion trace for debugging.
	Evaluate(ctx PromotionContext) (bool, string)
}

// Benefit is an adjustment generator for a promotion.
//
// When a promotion passes all its conditions, each Benefit is applied to generate
// adjustments. Benefits return two separate slices:
//   - itemAdjustments: attached to specific items (target = "ITEM:<SKU>")
//   - orderAdjustments: attached to the order level (target = "ORDER")
//
// This separation is important because item-level adjustments are stored on the
// ItemSnapshot while order-level adjustments are stored on the PriceSnapshot.
// Item-level adjustments enable accurate per-item refunds, tax calculation, and reporting.
//
// Implementations are in the promos package:
//   - PercentOffOrderBenefit: percentage discount on the entire order subtotal
//   - PercentOffSKUBenefit: percentage discount on a specific SKU's line total
//   - BuyXGetYBenefit: free items when a quantity threshold is met
//   - FreeDeliveryBenefit: signals the delivery stage to waive the delivery fee
//
// To add new benefit types, implement this interface and attach it to a Promotion's Benefits slice.
type Benefit interface {
	// Apply generates adjustments for this benefit.
	// Returns (itemAdjustments, orderAdjustments, error).
	// Item adjustments target "ITEM:<SKU>" and are attached to ItemSnapshot.Adjustments.
	// Order adjustments target "ORDER" and are attached to PriceSnapshot.Adjustments.
	Apply(ctx PromotionContext) ([]Adjustment, []Adjustment, error)
}

// PromotionContext provides read access to the current cart and pricing state
// during promotion evaluation.
//
// This interface is implemented by engine.CalcContext, allowing promotion conditions
// and benefits to inspect the cart contents and the in-progress price snapshot
// (e.g., to compute percentage discounts based on the current subtotal).
type PromotionContext interface {
	GetCart() Cart             // Returns the input cart
	GetSnapshot() PriceSnapshot // Returns the current (in-progress) price snapshot
}