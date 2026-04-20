package promos

import (
	"fmt"

	"pricing-engine/internal/domain"
)

// MinSubtotalCondition checks whether the order subtotal meets a minimum threshold.
//
// This is a common promotion eligibility condition used to gate discounts behind a
// minimum spend requirement (e.g., "10% off orders over €10.00").
//
// # Evaluation
//
// The condition compares the current snapshot's Subtotal.Amount against MinAmount.
// If the subtotal is below the threshold, it returns (false, "subtotal_below_<amount>").
// The reason string is recorded in the promotion trace for debugging.
//
// # Important
//
// MinAmount is in minor currency units (e.g., 1000 = €10.00). The subtotal used
// for comparison is the pre-discount subtotal (sum of item line totals), computed
// by the SubtotalStage before promotions are applied.
//
// # Adding Similar Conditions
//
// To add new eligibility conditions (e.g., MaxSubtotal, UserType, GeoRestriction):
//  1. Create a new struct implementing domain.Condition
//  2. Implement Evaluate(ctx domain.PromotionContext) (bool, string)
//  3. Return (true, "ok") if the condition is met, or (false, "<reason>") if not
//  4. Attach the condition to a Promotion's Conditions slice
type MinSubtotalCondition struct {
	MinAmount int64 // Minimum subtotal in minor currency units (e.g., 1000 = €10.00)
}

func (c MinSubtotalCondition) Evaluate(ctx domain.PromotionContext) (bool, string) {
	if ctx.GetSnapshot().Subtotal.Amount < c.MinAmount {
		return false, fmt.Sprintf("subtotal_below_%d", c.MinAmount)
	}
	return true, "ok"
}

// HasSKUCondition checks whether a specific SKU is present in the cart.
//
// This is used to gate promotions that only apply when a particular product is ordered
// (e.g., "Buy 2 Get 1 Free on burgers" requires burgers in the cart).
//
// # Evaluation
//
// Iterates through the cart items and checks if any item's SKU matches the target.
// Returns (false, "sku_not_found") if the SKU is not in the cart.
// The reason string is recorded in the promotion trace for debugging.
//
// Note: This checks presence only (quantity ≥ 1). For quantity-based thresholds,
// the benefit itself (e.g., BuyXGetYBenefit) performs the quantity check.
type HasSKUCondition struct {
	SKU string // Target SKU that must be present in the cart
}

func (c HasSKUCondition) Evaluate(ctx domain.PromotionContext) (bool, string) {
	cart := ctx.GetCart()
	for _, it := range cart.Items {
		if it.SKU == c.SKU {
			return true, "ok"
		}
	}
	return false, "sku_not_found"
}