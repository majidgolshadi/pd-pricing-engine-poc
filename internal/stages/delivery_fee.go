package stages

import (
	"pricing-engine/internal/domain"
	"pricing-engine/internal/engine"
)

type DeliveryFeeStage struct {
	BaseFee int64
}

func (s DeliveryFeeStage) Name() string { return "delivery_fee" }

func (s DeliveryFeeStage) Execute(ctx *engine.CalcContext) error {
	fee := domain.NewMoney(s.BaseFee, ctx.Cart.Currency)

	// check if any adjustment requests free delivery override
	for _, adj := range ctx.Snapshot.Adjustments {
		if adj.Type == domain.AdjFee && adj.Metadata["fee_override"] == "true" {
			fee = domain.NewMoney(0, ctx.Cart.Currency)
			break
		}
	}

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
