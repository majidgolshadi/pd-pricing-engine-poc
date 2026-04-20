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
	rounding := domain.NewMoney(0, ctx.Cart.Currency)

	// item discounts and rounding
	for _, it := range ctx.Snapshot.Items {
		for _, adj := range it.Adjustments {
			switch adj.Type {
			case domain.AdjDiscount:
				discounts = discounts.Add(adj.Amount)
			case domain.AdjRounding:
				rounding = rounding.Add(adj.Amount)
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
		case domain.AdjRounding:
			rounding = rounding.Add(adj.Amount)
		}
	}

	ctx.Snapshot.Discounts = discounts
	ctx.Snapshot.DeliveryFee = fees
	ctx.Snapshot.Tax = taxes
	ctx.Snapshot.Rounding = rounding

	total := subtotal.Add(discounts).Add(fees).Add(taxes).Add(rounding)

	if total.Amount < 0 {
		return fmt.Errorf("invalid total: %d", total.Amount)
	}

	ctx.Snapshot.Total = total
	return nil
}
