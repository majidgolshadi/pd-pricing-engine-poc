package stages

import (
	"pricing-engine/internal/domain"
	"pricing-engine/internal/engine"
)

// TaxStage is the fifth stage in the pricing pipeline.
// It computes tax (e.g., VAT/GST) based on the adjusted totals and adds it
// as an order-level TAX adjustment.
//
// # Tax Calculation Strategy
//
// Tax is calculated AFTER promotions and delivery fees are applied, ensuring tax
// is based on what the customer actually pays. This is consistent with real-world
// VAT/GST models where tax applies to the net amount after discounts.
//
// # Tax Base Computation
//
// The tax base is computed from two sources:
//  1. Item final totals: sum of each item's FinalTotal (which includes item-level discounts)
//  2. Fee adjustments: delivery fees and service fees (order-level FEE adjustments)
//
// The tax base is clamped to 0 if it would be negative (possible if discounts
// exceed the subtotal + fees).
//
// Tax amount = taxBase × VATPercent / 100 (integer division, truncating fractions).
//
// # Simplification Note
//
// This implementation applies a single VAT rate to the entire order. In production,
// different items may have different tax rates (e.g., food vs alcohol vs delivery).
// To support per-category tax rates:
//   - Use per-item tax categories and rates
//   - Compute tax per item/category separately
//   - Emit multiple TAX adjustments (one per rate)
//   - Consider using RoundingScope=PER_TAX to round each tax component separately
//
// # Output
//
// Appends a single TAX adjustment to Snapshot.Adjustments:
//   - Type: TAX
//   - Target: "ORDER"
//   - Amount: computed tax in minor units (positive value)
//   - ReasonCode: "VAT"
//
// # Dependencies
//
// Must run AFTER ApplyPromotionsStage (item FinalTotals must reflect discounts).
// Must run AFTER DeliveryFeeStage (delivery fee is part of the tax base).
// Must run BEFORE RoundingStage (rounding applies to the fully-adjusted amount including tax).
type TaxStage struct {
	VATPercent int64 // VAT rate as integer percentage (e.g., 7 = 7%, 19 = 19%)
}

func (s TaxStage) Name() string { return "tax" }

func (s TaxStage) Execute(ctx *engine.CalcContext) error {
	base := domain.NewMoney(0, ctx.Cart.Currency)

	// Sum item final totals as the primary tax base.
	// FinalTotal already includes item-level discounts applied by the promotions stage.
	for _, it := range ctx.Snapshot.Items {
		base = base.Add(it.FinalTotal)
	}

	// Include fee adjustments (delivery, service fees) in the tax base.
	// This means tax applies to the delivery fee as well.
	for _, adj := range ctx.Snapshot.Adjustments {
		if adj.Type == domain.AdjFee {
			base = base.Add(adj.Amount)
		}
	}

	// Clamp tax base to 0 if negative (edge case: discounts exceed subtotal + fees).
	if base.Amount < 0 {
		base.Amount = 0
	}

	// Compute tax using integer percentage division (truncates fractions).
	taxAmount := base.Amount * s.VATPercent / 100
	tax := domain.NewMoney(taxAmount, ctx.Cart.Currency)

	// Add tax as an order-level TAX adjustment.
	ctx.Snapshot.Adjustments = append(ctx.Snapshot.Adjustments, domain.Adjustment{
		ID:          "VAT",
		Type:        domain.AdjTax,
		Target:      "ORDER",
		Amount:      tax,
		ReasonCode:  "VAT",
		Description: "VAT tax",
	})

	return nil
}