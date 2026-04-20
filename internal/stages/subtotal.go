package stages

import (
	"pricing-engine/internal/domain"
	"pricing-engine/internal/engine"
)

// SubtotalStage is the second stage in the pricing pipeline.
// It computes item line totals and the overall order subtotal.
//
// # What It Does
//
// For each LineItem in the cart:
//  1. Computes the line total: UnitPrice.Amount × Quantity
//  2. Creates an ItemSnapshot with LineTotal and FinalTotal both set to the line total
//     (FinalTotal will be adjusted later by promotion and rounding stages)
//
// Then sums all line totals to produce the order Subtotal.
//
// # Output (written to CalcContext.Snapshot)
//
//   - Snapshot.Items: one ItemSnapshot per cart item, with LineTotal and FinalTotal set
//   - Snapshot.Subtotal: sum of all item LineTotals
//
// # Example
//
// Cart: burger (599 × 2) + cola (199 × 1)
//   - burger LineTotal = 1198
//   - cola LineTotal = 199
//   - Subtotal = 1397
//
// # Dependencies
//
// Must run AFTER NormalizeStage (cart is validated).
// Must run BEFORE ApplyPromotionsStage (promotions need the subtotal for % calculations).
type SubtotalStage struct{}

func (s SubtotalStage) Name() string { return "subtotal" }

func (s SubtotalStage) Execute(ctx *engine.CalcContext) error {
	subtotal := domain.NewMoney(0, ctx.Cart.Currency)

	items := make([]domain.ItemSnapshot, 0, len(ctx.Cart.Items))

	for _, item := range ctx.Cart.Items {
		// Compute line total: unit price × quantity.
		lineTotal := domain.NewMoney(item.UnitPrice.Amount*item.Quantity, item.UnitPrice.Currency)

		// Create the item snapshot. FinalTotal starts equal to LineTotal and will be
		// adjusted by later stages (promotions may add item-level discount adjustments,
		// rounding may add item-level rounding adjustments).
		items = append(items, domain.ItemSnapshot{
			SKU:        item.SKU,
			Quantity:   item.Quantity,
			UnitPrice:  item.UnitPrice,
			LineTotal:  lineTotal,
			FinalTotal: lineTotal, // Will be recalculated after item-level adjustments
		})

		subtotal = subtotal.Add(lineTotal)
	}

	ctx.Snapshot.Items = items
	ctx.Snapshot.Subtotal = subtotal
	return nil
}