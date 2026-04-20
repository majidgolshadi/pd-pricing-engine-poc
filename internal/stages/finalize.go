package stages

import (
	"fmt"

	"pricing-engine/internal/domain"
	"pricing-engine/internal/engine"
)

// FinalizeStage is the seventh and last stage in the pricing pipeline.
// It aggregates all adjustments into the final totals and validates the result.
//
// # What It Does
//
// 1. Recomputes the order Subtotal as the sum of all item LineTotals
// 2. Iterates through all adjustments (item-level and order-level) and aggregates them
//    by type into separate totals: Discounts, DeliveryFee (Fees), Tax, Rounding
// 3. Computes the final Total as: Subtotal + Discounts + Fees + Tax + Rounding
// 4. Validates that the Total is not negative (which would indicate a pricing error)
//
// # Aggregation Rules
//
// Item-level adjustments (from ItemSnapshot.Adjustments):
//   - DISCOUNT adjustments → summed into Snapshot.Discounts (negative values)
//   - ROUNDING adjustments → summed into Snapshot.Rounding
//
// Order-level adjustments (from Snapshot.Adjustments):
//   - DISCOUNT adjustments → summed into Snapshot.Discounts (negative values)
//   - FEE adjustments → summed into Snapshot.DeliveryFee (positive values)
//   - TAX adjustments → summed into Snapshot.Tax (positive values)
//   - ROUNDING adjustments → summed into Snapshot.Rounding
//
// # Output (written to CalcContext.Snapshot)
//
//   - Snapshot.Subtotal: sum of all item LineTotals
//   - Snapshot.Discounts: sum of all DISCOUNT adjustments (negative)
//   - Snapshot.DeliveryFee: sum of all FEE adjustments (positive)
//   - Snapshot.Tax: sum of all TAX adjustments (positive)
//   - Snapshot.Rounding: sum of all ROUNDING adjustments
//   - Snapshot.Total: final payable amount (must be ≥ 0)
//
// # Example
//
// Given:
//   - Subtotal = 1397 (burger 1198 + cola 199)
//   - Item discounts = -240 (20% off burger)
//   - Order discounts = -140 (10% off order via PROMO10)
//   - Fees = +449 (delivery 299 + service 150)
//   - Tax = 0
//   - Rounding = +2
//
// Total = 1397 + (-240) + (-140) + 449 + 0 + 2 = 1468
//
// # Dependencies
//
// Must run LAST in the pipeline (after all adjustments have been computed).
type FinalizeStage struct{}

func (s FinalizeStage) Name() string { return "finalize" }

func (s FinalizeStage) Execute(ctx *engine.CalcContext) error {
	// Recompute subtotal from item LineTotals (pre-adjustment base).
	subtotal := domain.NewMoney(0, ctx.Cart.Currency)
	for _, it := range ctx.Snapshot.Items {
		subtotal = subtotal.Add(it.LineTotal)
	}
	ctx.Snapshot.Subtotal = subtotal

	// Initialize aggregate accumulators for each adjustment type.
	discounts := domain.NewMoney(0, ctx.Cart.Currency)
	fees := domain.NewMoney(0, ctx.Cart.Currency)
	taxes := domain.NewMoney(0, ctx.Cart.Currency)
	rounding := domain.NewMoney(0, ctx.Cart.Currency)

	// Aggregate item-level adjustments (discounts and rounding from ItemSnapshots).
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

	// Aggregate order-level adjustments (discounts, fees, taxes, rounding).
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

	// Store the aggregated totals in the snapshot.
	ctx.Snapshot.Discounts = discounts
	ctx.Snapshot.DeliveryFee = fees
	ctx.Snapshot.Tax = taxes
	ctx.Snapshot.Rounding = rounding

	// Compute the final payable total.
	// Total = Subtotal + Discounts(negative) + Fees(positive) + Tax(positive) + Rounding(±small)
	total := subtotal.Add(discounts).Add(fees).Add(taxes).Add(rounding)

	// Validate: the total should never be negative. A negative total would indicate
	// a pricing error (e.g., discounts exceeding the order value without proper capping).
	if total.Amount < 0 {
		return fmt.Errorf("invalid total: %d", total.Amount)
	}

	ctx.Snapshot.Total = total
	return nil
}