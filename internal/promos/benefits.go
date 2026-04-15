package promos

import (
	"fmt"

	"pricing-engine/internal/domain"
)

// PercentOffOrderBenefit applies a percentage discount to the entire order subtotal.
type PercentOffOrderBenefit struct {
	Percent int64
	Code    string
}

func (b PercentOffOrderBenefit) Apply(ctx domain.PromotionContext) ([]domain.Adjustment, []domain.Adjustment, error) {
	snap := ctx.GetSnapshot()
	cart := ctx.GetCart()

	discount := snap.Subtotal.Amount * b.Percent / 100

	orderAdj := domain.Adjustment{
		ID:          "PROMO_" + b.Code,
		Type:        domain.AdjDiscount,
		Target:      "ORDER",
		Amount:      domain.NewMoney(-discount, cart.Currency),
		ReasonCode:  b.Code,
		Description: "Percent off order",
	}

	return nil, []domain.Adjustment{orderAdj}, nil
}

// PercentOffSKUBenefit applies a percentage discount to a specific SKU.
type PercentOffSKUBenefit struct {
	SKU     string
	Percent int64
	Code    string
}

func (b PercentOffSKUBenefit) Apply(ctx domain.PromotionContext) ([]domain.Adjustment, []domain.Adjustment, error) {
	cart := ctx.GetCart()

	var adjs []domain.Adjustment

	for _, item := range cart.Items {
		if item.SKU != b.SKU {
			continue
		}

		lineTotal := item.UnitPrice.Amount * item.Quantity
		discount := lineTotal * b.Percent / 100

		adjs = append(adjs, domain.Adjustment{
			ID:          "PROMO_ITEM_" + b.Code,
			Type:        domain.AdjDiscount,
			Target:      "ITEM:" + item.SKU,
			Amount:      domain.NewMoney(-discount, cart.Currency),
			ReasonCode:  b.Code,
			Description: "Percent off SKU",
		})
	}

	return adjs, nil, nil
}

// BuyXGetYBenefit gives free items when a quantity threshold is met.
type BuyXGetYBenefit struct {
	SKU  string
	Buy  int64
	Free int64
	Code string
}

func (b BuyXGetYBenefit) Apply(ctx domain.PromotionContext) ([]domain.Adjustment, []domain.Adjustment, error) {
	cart := ctx.GetCart()

	for _, item := range cart.Items {
		if item.SKU != b.SKU {
			continue
		}

		if item.Quantity < b.Buy {
			return nil, nil, nil
		}

		freeCount := (item.Quantity / b.Buy) * b.Free
		if freeCount <= 0 {
			return nil, nil, nil
		}

		discount := freeCount * item.UnitPrice.Amount

		adj := domain.Adjustment{
			ID:          "PROMO_BXGY_" + b.Code,
			Type:        domain.AdjDiscount,
			Target:      "ITEM:" + item.SKU,
			Amount:      domain.NewMoney(-discount, cart.Currency),
			ReasonCode:  b.Code,
			Description: "Buy X Get Y Free",
			Metadata: map[string]string{
				"free_count": fmt.Sprintf("%d", freeCount),
			},
		}

		return []domain.Adjustment{adj}, nil, nil
	}

	return nil, nil, nil
}

// FreeDeliveryBenefit signals the delivery stage to waive the delivery fee.
type FreeDeliveryBenefit struct {
	Code string
}

func (b FreeDeliveryBenefit) Apply(ctx domain.PromotionContext) ([]domain.Adjustment, []domain.Adjustment, error) {
	cart := ctx.GetCart()

	adj := domain.Adjustment{
		ID:          "PROMO_FREE_DELIVERY_" + b.Code,
		Type:        domain.AdjFee,
		Target:      "ORDER",
		Amount:      domain.NewMoney(0, cart.Currency),
		ReasonCode:  b.Code,
		Description: "Free delivery",
		Metadata: map[string]string{
			"fee_override": "true",
		},
	}

	return nil, []domain.Adjustment{adj}, nil
}
