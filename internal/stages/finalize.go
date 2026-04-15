package stages

import (
	"fmt"

	"pricing-engine/internal/domain"
	"pricing-engine/internal/engine"
)

type FinalizeStage struct{}

func (s FinalizeStage) Name() string { return "finalize" }

func (s FinalizeStage) Execute(ctx *engine.CalcContext) error {
	subtotal := domain.NewMoney(0, ctx.Cart.Currency)
	for _, it := range ctx.Snapshot.Items {
		subtotal = subtotal.Add(it.LineTotal)
	}
	ctx.Snapshot.Subtotal = subtotal

	discounts := domain.NewMoney(0, ctx.Cart.Currency)
	fees := domain.NewMoney(0, ctx.Cart.Currency)
	taxes := domain.NewMoney(0, ctx.Cart.Currency)

	// item discounts
	for _, it := range ctx.Snapshot.Items {
		for _, adj := range it.Adjustments {
			if adj.Type == domain.AdjDiscount {
				discounts = discounts.Add(adj.Amount)
			}
		}
	}

	// order-level adjustments
	for _, adj := range ctx.Snapshot.Adjustments {
		switch adj.Type {
		case domain.AdjDiscount:
			discounts = discounts.Add(adj.Amount)
		case domain.AdjFee:
			fees = fees.Add(adj.Amount)
		case domain.AdjTax:
			taxes = taxes.Add(adj.Amount)
		}
	}

	ctx.Snapshot.Discounts = discounts
	ctx.Snapshot.DeliveryFee = fees
	ctx.Snapshot.Tax = taxes

	total := subtotal.Add(discounts).Add(fees).Add(taxes)

	if total.Amount < 0 {
		return fmt.Errorf("invalid total: %d", total.Amount)
	}

	ctx.Snapshot.Total = total
	return nil
}
