// Package promos provides concrete implementations of domain.Condition and domain.Benefit
// for the promotion rule system.
//
// # Promotion Rule System
//
// Promotions are modeled as a rule system consisting of:
//   - Conditions (conditions.go): eligibility checks that gate whether a promotion applies
//   - Benefits (this file): adjustment generators that produce pricing adjustments when applied
//
// This separation allows new promotion types to be added by simply implementing new
// Condition or Benefit types, without modifying the engine or pipeline code.
//
// # Available Benefit Types
//
//   - PercentOffOrderBenefit: percentage discount on the entire order subtotal → ORDER adjustment
//   - PercentOffSKUBenefit: percentage discount on a specific SKU → ITEM:<SKU> adjustment(s)
//   - BuyXGetYBenefit: free items when quantity threshold met → ITEM:<SKU> adjustment
//   - FreeDeliveryBenefit: signals the delivery stage to waive the fee → ORDER adjustment
//
// # Adding New Benefit Types
//
// To add a new benefit type (e.g., FixedAmountOffOrder, PercentOffCategory):
//  1. Create a new struct implementing domain.Benefit
//  2. Implement Apply(ctx domain.PromotionContext) ([]domain.Adjustment, []domain.Adjustment, error)
//  3. Return item-level adjustments in the first slice, order-level in the second
//  4. Use the new benefit in a domain.Promotion definition
//
// No changes to the engine, pipeline, or persistence are needed.
package promos

import (
	"fmt"

	"pricing-engine/internal/domain"
)

// PercentOffOrderBenefit applies a percentage discount to the entire order subtotal.
//
// This generates a single ORDER-level DISCOUNT adjustment. The discount amount is
// calculated as: subtotal × Percent / 100 (integer division, truncating fractions).
//
// Example: If subtotal is 1397 (€13.97) and Percent is 10, the discount is 139 (€1.39).
//
// The adjustment is stored with:
//   - Type: DISCOUNT
//   - Target: "ORDER"
//   - Amount: negative (discount reduces the total)
//   - ReasonCode: the benefit's Code field
type PercentOffOrderBenefit struct {
	Percent int64  // Discount percentage (e.g., 10 = 10%)
	Code    string // Reason code for the adjustment (e.g., "PROMO10")
}

func (b PercentOffOrderBenefit) Apply(ctx domain.PromotionContext) ([]domain.Adjustment, []domain.Adjustment, error) {
	snap := ctx.GetSnapshot()
	cart := ctx.GetCart()

	// Compute discount as integer percentage of the current subtotal.
	// Uses integer division which truncates (floors) the result.
	discount := snap.Subtotal.Amount * b.Percent / 100

	orderAdj := domain.Adjustment{
		ID:          "PROMO_" + b.Code,
		Type:        domain.AdjDiscount,
		Target:      "ORDER",
		Amount:      domain.NewMoney(-discount, cart.Currency),
		ReasonCode:  b.Code,
		Description: "Percent off order",
	}

	// Return as order-level adjustment (second slice).
	return nil, []domain.Adjustment{orderAdj}, nil
}

// PercentOffSKUBenefit applies a percentage discount to a specific SKU's line total.
//
// This generates ITEM-level DISCOUNT adjustment(s) for the matching SKU. The discount
// is calculated from the item's full line total (unitPrice × quantity), ensuring the
// discount is proportional to the quantity ordered.
//
// # Why Item-Level Adjustments?
//
// Item-level adjustments (rather than a single order-level adjustment) are critical for:
//   - Partial refunds: if the item is cancelled, its discount is removed cleanly
//   - Tax compliance: the discount reduces the taxable base for this item's tax category
//   - Invoice clarity: customers see the exact discount per line item
//   - Reporting: discount analysis by SKU/category is accurate
//
// Example: burger (599 × 2 = 1198), 20% off → discount = 239 → adjustment Amount = -239
type PercentOffSKUBenefit struct {
	SKU     string // Target SKU to discount
	Percent int64  // Discount percentage (e.g., 20 = 20%)
	Code    string // Reason code for the adjustment
}

func (b PercentOffSKUBenefit) Apply(ctx domain.PromotionContext) ([]domain.Adjustment, []domain.Adjustment, error) {
	cart := ctx.GetCart()

	var adjs []domain.Adjustment

	for _, item := range cart.Items {
		if item.SKU != b.SKU {
			continue
		}

		// Compute discount on the full line total (unitPrice × quantity).
		lineTotal := item.UnitPrice.Amount * item.Quantity
		discount := lineTotal * b.Percent / 100

		adjs = append(adjs, domain.Adjustment{
			ID:          "PROMO_ITEM_" + b.Code,
			Type:        domain.AdjDiscount,
			Target:      "ITEM:" + item.SKU,
			Amount:      domain.NewMoney(-discount, cart.Currency),
			ReasonCode:  b.Code,
			Description: "Percent off SKU",
		})
	}

	// Return as item-level adjustments (first slice).
	return adjs, nil, nil
}

// BuyXGetYBenefit gives free items when a quantity threshold is met.
//
// For every Buy items purchased, Free items are given at no charge. The discount
// amount equals: freeCount × unitPrice (each free item is fully discounted).
//
// Example: Buy=2, Free=1, SKU="burger" with quantity=3
//   - freeCount = (3 / 2) × 1 = 1 free burger
//   - discount = 1 × 599 = 599
//
// The adjustment includes metadata with the computed free_count for auditability.
//
// If the item quantity is less than Buy, no adjustment is generated (the threshold
// isn't met). The adjustment targets "ITEM:<SKU>" for per-item tracking.
type BuyXGetYBenefit struct {
	SKU  string // Target SKU for the buy-X-get-Y deal
	Buy  int64  // Number of items to buy to trigger the deal
	Free int64  // Number of free items granted per Buy threshold
	Code string // Reason code for the adjustment
}

func (b BuyXGetYBenefit) Apply(ctx domain.PromotionContext) ([]domain.Adjustment, []domain.Adjustment, error) {
	cart := ctx.GetCart()

	for _, item := range cart.Items {
		if item.SKU != b.SKU {
			continue
		}

		// Check if the quantity threshold is met.
		if item.Quantity < b.Buy {
			return nil, nil, nil
		}

		// Calculate how many free items the customer earns.
		// For every Buy items, they get Free items free.
		freeCount := (item.Quantity / b.Buy) * b.Free
		if freeCount <= 0 {
			return nil, nil, nil
		}

		// Each free item is discounted by its full unit price.
		discount := freeCount * item.UnitPrice.Amount

		adj := domain.Adjustment{
			ID:          "PROMO_BXGY_" + b.Code,
			Type:        domain.AdjDiscount,
			Target:      "ITEM:" + item.SKU,
			Amount:      domain.NewMoney(-discount, cart.Currency),
			ReasonCode:  b.Code,
			Description: "Buy X Get Y Free",
			Metadata: map[string]string{
				"free_count": fmt.Sprintf("%d", freeCount),
			},
		}

		return []domain.Adjustment{adj}, nil, nil
	}

	return nil, nil, nil
}

// FreeDeliveryBenefit signals the delivery stage to waive the delivery fee.
//
// Instead of directly modifying the delivery fee (which would couple promotions
// to the delivery stage), this benefit emits a FEE adjustment with Amount=0 and
// metadata {"fee_override": "true"}. The DeliveryFeeStage checks for this marker
// and sets the delivery fee to zero when found.
//
// This design keeps the delivery logic clean and independent while allowing
// business-controlled promotional behavior through the adjustment model.
//
// The adjustment is:
//   - Type: FEE (not DISCOUNT, because it modifies the fee behavior)
//   - Target: "ORDER"
//   - Amount: 0 (the actual fee waiver happens in DeliveryFeeStage)
//   - Metadata: {"fee_override": "true"} — the signal to DeliveryFeeStage
type FreeDeliveryBenefit struct {
	Code string // Reason code for the adjustment (e.g., "FREEDEL")
}

func (b FreeDeliveryBenefit) Apply(ctx domain.PromotionContext) ([]domain.Adjustment, []domain.Adjustment, error) {
	cart := ctx.GetCart()

	adj := domain.Adjustment{
		ID:          "PROMO_FREE_DELIVERY_" + b.Code,
		Type:        domain.AdjFee,
		Target:      "ORDER",
		Amount:      domain.NewMoney(0, cart.Currency),
		ReasonCode:  b.Code,
		Description: "Free delivery",
		Metadata: map[string]string{
			"fee_override": "true",
		},
	}

	return nil, []domain.Adjustment{adj}, nil
}