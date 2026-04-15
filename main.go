package main

import (
	"encoding/json"
	"fmt"
	"time"

	"pricing-engine/internal/domain"
	"pricing-engine/internal/engine"
	"pricing-engine/internal/promos"
	"pricing-engine/internal/stages"
)

func main() {
	cart := domain.Cart{
		ID:       "cart-1",
		StoreID:  "store-berlin",
		UserID:   "user-123",
		Currency: "EUR",
		Items: []domain.LineItem{
			{
				SKU:       "burger",
				Name:      "Burger",
				Quantity:  3,
				UnitPrice: domain.NewMoney(599, "EUR"),
			},
			{
				SKU:       "cola",
				Name:      "Cola",
				Quantity:  2,
				UnitPrice: domain.NewMoney(199, "EUR"),
			},
		},
		Coupon: &domain.CouponInput{Code: "FREEDEL"},
	}

	ctx := engine.NewContext(cart, time.Now())

	ctx.Promotions = []domain.Promotion{
		{
			ID:             "promo-10off",
			Code:           "PROMO10",
			Priority:       50,
			Stackable:      true,
			Group:          "ORDER_DISCOUNT",
			RequiresCoupon: false,
			ValidFrom:      time.Now().Add(-24 * time.Hour),
			ValidTo:        time.Now().Add(24 * time.Hour),
			Conditions: []domain.Condition{
				promos.MinSubtotalCondition{MinAmount: 1000},
			},
			Benefits: []domain.Benefit{
				promos.PercentOffOrderBenefit{Percent: 10, Code: "PROMO10"},
			},
		},
		{
			ID:             "promo-bxgy",
			Code:           "BUY2GET1",
			Priority:       100,
			Stackable:      true,
			Group:          "",
			RequiresCoupon: false,
			ValidFrom:      time.Now().Add(-24 * time.Hour),
			ValidTo:        time.Now().Add(24 * time.Hour),
			Conditions: []domain.Condition{
				promos.HasSKUCondition{SKU: "burger"},
			},
			Benefits: []domain.Benefit{
				promos.BuyXGetYBenefit{SKU: "burger", Buy: 2, Free: 1, Code: "BUY2GET1"},
			},
		},
		{
			ID:             "promo-free-delivery",
			Code:           "FREEDEL",
			Priority:       999,
			Stackable:      false,
			Group:          "DELIVERY",
			RequiresCoupon: true,
			ValidFrom:      time.Now().Add(-24 * time.Hour),
			ValidTo:        time.Now().Add(24 * time.Hour),
			Benefits: []domain.Benefit{
				promos.FreeDeliveryBenefit{Code: "FREEDEL"},
			},
		},
	}

	e := engine.NewEngine(
		stages.NormalizeStage{},
		stages.SubtotalStage{},
		stages.ApplyPromotionsStage{},
		stages.DeliveryFeeStage{BaseFee: 299},
		stages.TaxStage{VATPercent: 7},
		stages.FinalizeStage{},
	)

	if err := e.Calculate(ctx); err != nil {
		panic(err)
	}

	out, _ := json.MarshalIndent(ctx.Snapshot, "", "  ")
	fmt.Println(string(out))

	fmt.Println("\n----- promotion trace -----")
	for _, t := range ctx.Trace.Promotions {
		fmt.Printf("%s (%s) => %s (%s)\n", t.PromoCode, t.PromoID, t.Status, t.Reason)
	}
}
