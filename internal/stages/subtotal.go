package stages

import (
	"pricing-engine/internal/domain"
	"pricing-engine/internal/engine"
)

type SubtotalStage struct{}

func (s SubtotalStage) Name() string { return "subtotal" }

func (s SubtotalStage) Execute(ctx *engine.CalcContext) error {
	subtotal := domain.NewMoney(0, ctx.Cart.Currency)

	items := make([]domain.ItemSnapshot, 0, len(ctx.Cart.Items))

	for _, item := range ctx.Cart.Items {
		lineTotal := domain.NewMoney(item.UnitPrice.Amount*item.Quantity, item.UnitPrice.Currency)

		items = append(items, domain.ItemSnapshot{
			SKU:        item.SKU,
			Quantity:   item.Quantity,
			UnitPrice:  item.UnitPrice,
			LineTotal:  lineTotal,
			FinalTotal: lineTotal,
		})

		subtotal = subtotal.Add(lineTotal)
	}

	ctx.Snapshot.Items = items
	ctx.Snapshot.Subtotal = subtotal
	return nil
}
