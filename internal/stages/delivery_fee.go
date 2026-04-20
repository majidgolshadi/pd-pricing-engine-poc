package stages

import (
	"pricing-engine/internal/domain"
	"pricing-engine/internal/engine"
)

// DeliveryFeeStage is the fourth stage in the pricing pipeline.
// It computes the delivery fee and adds it as an order-level FEE adjustment.
//
// # Delivery Fee as an Adjustment
//
// The delivery fee is treated as a first-class adjustment (type=FEE, target=ORDER)
// rather than a hardcoded field on the order. This ensures:
//   - The fee appears in the adjustment list alongside discounts and taxes
//   - It is included in the total derivation: Total = Subtotal + Discounts + Fees + Tax + Rounding
//   - It can be overridden by promotions (e.g., free delivery coupon)
//   - It is fully auditable and traceable
//
// # Free Delivery Override
//
// Instead of hardcoding free delivery logic, the stage checks for a "fee_override"
// marker in existing order-level adjustments. The FreeDeliveryBenefit (see promos/benefits.go)
// emits a FEE adjustment with metadata {"fee_override": "true"} during the promotions stage.
//
// When this marker is found, the delivery fee is set to 0. This keeps the delivery
// logic decoupled from promotion logic — the delivery stage doesn't need to know
// about specific promotions; it just respects the override signal.
//
// # Configuration
//
// BaseFee is the default delivery fee in minor currency units (e.g., 299 = €2.99).
// In production, this would typically be loaded from a configuration store based on
// delivery zone, store location, or order characteristics.
//
// # Dependencies
//
// Must run AFTER ApplyPromotionsStage (free delivery override must already be in the adjustments).
// Must run BEFORE TaxStage (delivery fee is included in the tax base).
type DeliveryFeeStage struct {
	BaseFee int64 // Default delivery fee in minor currency units (e.g., 299 = €2.99)
}

func (s DeliveryFeeStage) Name() string { return "delivery_fee" }

func (s DeliveryFeeStage) Execute(ctx *engine.CalcContext) error {
	fee := domain.NewMoney(s.BaseFee, ctx.Cart.Currency)

	// Check if any existing adjustment signals a free delivery override.
	// This is set by FreeDeliveryBenefit during the promotions stage.
	for _, adj := range ctx.Snapshot.Adjustments {
		if adj.Type == domain.AdjFee && adj.Metadata["fee_override"] == "true" {
			fee = domain.NewMoney(0, ctx.Cart.Currency)
			break
		}
	}

	// Add the delivery fee as an order-level FEE adjustment.
	ctx.Snapshot.Adjustments = append(ctx.Snapshot.Adjustments, domain.Adjustment{
		ID:          "DELIVERY_BASE",
		Type:        domain.AdjFee,
		Target:      "ORDER",
		Amount:      fee,
		ReasonCode:  "DELIVERY_BASE",
		Description: "Base delivery fee",
	})

	return nil
}