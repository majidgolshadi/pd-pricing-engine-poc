package stages

import (
	"fmt"

	"pricing-engine/internal/engine"
)

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
