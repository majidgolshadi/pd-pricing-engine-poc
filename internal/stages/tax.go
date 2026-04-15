package stages

import (
	"pricing-engine/internal/domain"
	"pricing-engine/internal/engine"
)

type TaxStage struct {
	VATPercent int64
}

func (s TaxStage) Name() string { return "tax" }

func (s TaxStage) Execute(ctx *engine.CalcContext) error {
	base := domain.NewMoney(0, ctx.Cart.Currency)

	// tax base from item final totals
	for _, it := range ctx.Snapshot.Items {
		base = base.Add(it.FinalTotal)
	}

	// include delivery fee
	for _, adj := range ctx.Snapshot.Adjustments {
		if adj.Type == domain.AdjFee {
			base = base.Add(adj.Amount)
		}
	}

	if base.Amount < 0 {
		base.Amount = 0
	}

	taxAmount := base.Amount * s.VATPercent / 100
	tax := domain.NewMoney(taxAmount, ctx.Cart.Currency)

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
