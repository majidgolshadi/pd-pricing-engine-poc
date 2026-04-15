package promos

import (
	"fmt"

	"pricing-engine/internal/domain"
)

type MinSubtotalCondition struct {
	MinAmount int64
}

func (c MinSubtotalCondition) Evaluate(ctx domain.PromotionContext) (bool, string) {
	if ctx.GetSnapshot().Subtotal.Amount < c.MinAmount {
		return false, fmt.Sprintf("subtotal_below_%d", c.MinAmount)
	}
	return true, "ok"
}

type HasSKUCondition struct {
	SKU string
}

func (c HasSKUCondition) Evaluate(ctx domain.PromotionContext) (bool, string) {
	cart := ctx.GetCart()
	for _, it := range cart.Items {
		if it.SKU == c.SKU {
			return true, "ok"
		}
	}
	return false, "sku_not_found"
}
