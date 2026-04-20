package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"pricing-engine/internal/domain"
	"pricing-engine/internal/engine"
	"pricing-engine/internal/infra/dynamo"
	"pricing-engine/internal/promos"
	"pricing-engine/internal/stages"
)

func main() {
	ctx := context.Background()

	// ── Connect to LocalStack DynamoDB ──────────────────────────────────
	ddbClient, err := dynamo.NewLocalClient(ctx, "http://localhost:4566")
	if err != nil {
		log.Fatalf("creating DynamoDB client: %v", err)
	}
	repo := dynamo.NewSnapshotRepository(ddbClient)

	orderCode := "order-9001"

	// ── Build two carts to simulate two snapshots for the same order ────
	carts := []domain.Cart{
		{
			ID:       "cart-1",
			StoreID:  "store-berlin",
			UserID:   "user-123",
			Currency: "EUR",
			Items: []domain.LineItem{
				{SKU: "burger", Name: "Burger", Quantity: 3, UnitPrice: domain.NewMoney(599, "EUR")},
				{SKU: "cola", Name: "Cola", Quantity: 2, UnitPrice: domain.NewMoney(199, "EUR")},
			},
			Coupon: &domain.CouponInput{Code: "FREEDEL"},
		},
		{
			ID:       "cart-1",
			StoreID:  "store-berlin",
			UserID:   "user-123",
			Currency: "EUR",
			Items: []domain.LineItem{
				{SKU: "burger", Name: "Burger", Quantity: 2, UnitPrice: domain.NewMoney(599, "EUR")},
				{SKU: "cola", Name: "Cola", Quantity: 3, UnitPrice: domain.NewMoney(199, "EUR")},
			},
			Coupon: nil,
		},
	}

	promotions := []domain.Promotion{
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
		stages.RoundingStage{
			Policy: domain.RoundingPolicy{
				ID:        "rounding-eur-v1",
				Version:   "1.0.0",
				Method:    domain.RoundHalfUp,
				Scope:     domain.RoundOrderTotal,
				Increment: 5,
			},
		},
		stages.FinalizeStage{},
	)

	// ── Calculate and persist two snapshots ─────────────────────────────
	for i, cart := range carts {
		calcCtx := engine.NewContext(cart, time.Now())
		calcCtx.Promotions = promotions

		if err := e.Calculate(calcCtx); err != nil {
			log.Fatalf("calculation error (cart %d): %v", i+1, err)
		}

		// Copy promotion traces from engine trace to snapshot
		for _, pt := range calcCtx.Trace.Promotions {
			calcCtx.Snapshot.PromotionTraces = append(calcCtx.Snapshot.PromotionTraces, domain.PromotionTrace{
				PromotionID: pt.PromoID,
				Code:        pt.PromoCode,
				Status:      pt.Status,
				Reason:      pt.Reason,
			})
		}

		saved, err := repo.Save(ctx, orderCode, cart.Currency, calcCtx.Snapshot)
		if err != nil {
			log.Fatalf("saving snapshot (cart %d): %v", i+1, err)
		}
		fmt.Printf("✅ Saved snapshot: order=%s version=%d\n", saved.OrderCode, saved.Version)
	}

	// ── Demo Query 1: Get all snapshots for the order ───────────────────
	fmt.Println("\n═══ Query 1: All snapshots for order", orderCode, "═══")
	allVersions, err := repo.GetAllVersions(ctx, orderCode)
	if err != nil {
		log.Fatalf("GetAllVersions: %v", err)
	}
	for _, s := range allVersions {
		fmt.Printf("  version=%d  total=%d  created=%s\n",
			s.Version, s.Snapshot.Total.Amount, s.CreatedAt.Format(time.RFC3339))
	}

	// ── Demo Query 2: Get specific version ──────────────────────────────
	fmt.Println("\n═══ Query 2: Specific version (order=", orderCode, ", version=1) ═══")
	v1, err := repo.GetByVersion(ctx, orderCode, 1)
	if err != nil {
		log.Fatalf("GetByVersion: %v", err)
	}
	if v1 != nil {
		out, _ := json.MarshalIndent(v1, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Println("  not found")
	}

	// ── Demo Query 3: Get latest version ────────────────────────────────
	fmt.Println("\n═══ Query 3: Latest version for order", orderCode, "═══")
	latest, err := repo.GetLatest(ctx, orderCode)
	if err != nil {
		log.Fatalf("GetLatest: %v", err)
	}
	if latest != nil {
		out, _ := json.MarshalIndent(latest, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Println("  not found")
	}
}