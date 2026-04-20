// Package stages provides the concrete pipeline stage implementations for the pricing engine.
//
// # Pipeline Stages
//
// Each stage implements the engine.Stage interface (Name() + Execute()) and performs
// a single step in the pricing calculation pipeline. Stages are executed sequentially
// by the engine.Engine and mutate the shared engine.CalcContext.
//
// The recommended pipeline order is:
//  1. NormalizeStage        — Validate and normalize the input cart
//  2. SubtotalStage         — Compute item line totals and order subtotal
//  3. ApplyPromotionsStage  — Evaluate promotions and generate discount adjustments
//  4. DeliveryFeeStage      — Compute delivery/service fee adjustments
//  5. TaxStage              — Compute tax based on adjusted totals
//  6. RoundingStage         — Apply rounding rules and emit rounding adjustments
//  7. FinalizeStage         — Aggregate all adjustments into final totals and validate
//
// # Adding a New Stage
//
// To add a new stage (e.g., ServiceFeeStage, LoyaltyPointsStage):
//  1. Create a new file in this package (e.g., service_fee.go)
//  2. Define a struct implementing engine.Stage (Name + Execute)
//  3. Register it in the engine pipeline at the appropriate position in main.go
//
// No changes to the engine core are needed — just implement the Stage interface.
package stages

import (
	"fmt"

	"pricing-engine/internal/engine"
)

// NormalizeStage is the first stage in the pricing pipeline.
// It validates the input cart to ensure all required fields are present and valid
// before any pricing computation begins.
//
// # Validations Performed
//
//   - Cart.Currency must not be empty (required for all Money values)
//   - Cart.Items must not be empty (at least one item is required)
//   - Each item's Quantity must be > 0 (zero or negative quantities are invalid)
//   - Each item's UnitPrice.Amount must be ≥ 0 (negative prices are invalid)
//
// If any validation fails, the stage returns an error and the pipeline halts
// immediately (no further stages execute).
//
// # Why Validate First?
//
// Placing validation at the start of the pipeline ensures that all downstream stages
// can safely assume the cart data is well-formed. This eliminates defensive checks
// in every subsequent stage and prevents nonsensical pricing results from invalid input.
type NormalizeStage struct{}

func (s NormalizeStage) Name() string { return "normalize" }

func (s NormalizeStage) Execute(ctx *engine.CalcContext) error {
	if ctx.Cart.Currency == "" {
		return fmt.Errorf("cart currency is required")
	}
	if len(ctx.Cart.Items) == 0 {
		return fmt.Errorf("cart has no items")
	}
	for _, item := range ctx.Cart.Items {
		if item.Quantity <= 0 {
			return fmt.Errorf("item %q has invalid quantity %d", item.SKU, item.Quantity)
		}
		if item.UnitPrice.Amount < 0 {
			return fmt.Errorf("item %q has negative unit price", item.SKU)
		}
	}
	return nil
}