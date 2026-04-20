// Pricing Engine — Adjustment-Based Order Pricing System
//
// This is a demonstration / proof-of-concept for a pipeline-based pricing calculation engine
// that uses an adjustment-based accounting model. The engine computes order totals by
// processing a cart through a series of deterministic pipeline stages, producing an
// immutable PriceSnapshot that can be persisted and audited.
//
// # How to Run
//
// Prerequisites:
//  1. Start LocalStack (DynamoDB emulator): docker-compose up -d
//  2. Create the DynamoDB table: ./scripts/init-aws.sh
//  3. Run the demo: go run main.go
//
// # What This Demo Does
//
//  1. Connects to LocalStack DynamoDB
//  2. Builds two different carts for the same order (simulating cart modifications)
//  3. Configures promotions (10% off order, Buy 2 Get 1 Free on burgers, free delivery coupon)
//  4. Configures the pricing pipeline (normalize → subtotal → promotions → delivery → tax → rounding → finalize)
//  5. Calculates pricing for each cart and persists the snapshots
//  6. Demonstrates querying: all versions, specific version, latest version
//
// # Architecture Overview
//
// The system follows these key principles:
//
// Adjustment-Based Accounting:
// All price modifications (discounts, fees, taxes, rounding) are represented as explicit
// Adjustment objects rather than embedded arithmetic. The final total is always:
//   total = subtotal + sum(all adjustments)
// This makes pricing explainable, auditable, and reconcilable.
//
// Pipeline-Based Calculation:
// Pricing is computed through 7 sequential stages, each with a single responsibility:
//   normalize → subtotal → apply_promotions → delivery_fee → tax → rounding → finalize
//
// Promotion Rule System:
// Promotions are configurable rules with conditions (eligibility checks) and benefits
// (adjustment generators). They support priority ordering, stacking, and exclusivity groups.
//
// Immutable Snapshots:
// The pricing result is persisted as an immutable snapshot at checkout time. Historical
// orders are never recomputed with new pricing logic.
//
// For detailed documentation, see the comments in each package:
//   - internal/domain/    — Core types (Adjustment, Cart, Money, Promotion, Snapshot)
//   - internal/engine/    — Pipeline engine (Engine, Stage, CalcContext)
//   - internal/promos/    — Promotion conditions and benefits
//   - internal/stages/    — Pipeline stage implementations
//   - internal/infra/     — DynamoDB persistence
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
	// LocalStack provides a local AWS emulator for development.
	// The DynamoDB table must be created first via scripts/init-aws.sh.
	ddbClient, err := dynamo.NewLocalClient(ctx, "http://localhost:4566")
	if err != nil {
		log.Fatalf("creating DynamoDB client: %v", err)
	}
	repo := dynamo.NewSnapshotRepository(ddbClient)

	orderCode := "order-9001"

	// ── Build two carts to simulate two snapshots for the same order ────
	// This demonstrates versioned snapshot persistence: each cart produces
	// a new snapshot version, allowing the system to track price evolution.
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
			Coupon: &domain.CouponInput{Code: "FREEDEL"}, // Free delivery coupon
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
			Coupon: nil, // No coupon for the second cart
		},
	}

	// ── Configure promotions ───────────────────────────────────────────
	// Promotions are evaluated in priority order (higher priority first).
	// Each promotion has conditions (eligibility checks) and benefits (adjustment generators).
	promotions := []domain.Promotion{
		{
			// 10% off the entire order when subtotal ≥ €10.00
			// This is an order-level DISCOUNT adjustment.
			ID:             "promo-10off",
			Code:           "PROMO10",
			Priority:       50,
			Stackable:      true,                                       // Allows other promotions to also apply
			Group:          "ORDER_DISCOUNT",                           // Only one ORDER_DISCOUNT promo can apply
			RequiresCoupon: false,                                      // Auto-applied, no coupon needed
			ValidFrom:      time.Now().Add(-24 * time.Hour),
			ValidTo:        time.Now().Add(24 * time.Hour),
			Conditions: []domain.Condition{
				promos.MinSubtotalCondition{MinAmount: 1000}, // Subtotal must be ≥ 1000 (€10.00)
			},
			Benefits: []domain.Benefit{
				promos.PercentOffOrderBenefit{Percent: 10, Code: "PROMO10"},
			},
		},
		{
			// Buy 2 Get 1 Free on burgers
			// This is an item-level DISCOUNT adjustment targeting "ITEM:burger".
			ID:             "promo-bxgy",
			Code:           "BUY2GET1",
			Priority:       100,                                        // Higher priority → evaluated before PROMO10
			Stackable:      true,
			Group:          "",                                         // No exclusivity group
			RequiresCoupon: false,
			ValidFrom:      time.Now().Add(-24 * time.Hour),
			ValidTo:        time.Now().Add(24 * time.Hour),
			Conditions: []domain.Condition{
				promos.HasSKUCondition{SKU: "burger"}, // Cart must contain burgers
			},
			Benefits: []domain.Benefit{
				promos.BuyXGetYBenefit{SKU: "burger", Buy: 2, Free: 1, Code: "BUY2GET1"},
			},
		},
		{
			// Free delivery when coupon "FREEDEL" is applied
			// This emits a FEE override that the DeliveryFeeStage respects.
			ID:             "promo-free-delivery",
			Code:           "FREEDEL",
			Priority:       999,                                        // Highest priority
			Stackable:      false,                                      // Non-stackable (stops further promo evaluation)
			Group:          "DELIVERY",                                 // Exclusivity group for delivery promos
			RequiresCoupon: true,                                       // Requires coupon code "FREEDEL"
			ValidFrom:      time.Now().Add(-24 * time.Hour),
			ValidTo:        time.Now().Add(24 * time.Hour),
			Benefits: []domain.Benefit{
				promos.FreeDeliveryBenefit{Code: "FREEDEL"},
			},
		},
	}

	// ── Configure the pricing pipeline ─────────────────────────────────
	// Stages execute in order. Each stage has a single responsibility.
	// The order is critical for correctness (see engine.NewEngine docs).
	e := engine.NewEngine(
		stages.NormalizeStage{},                         // 1. Validate cart
		stages.SubtotalStage{},                          // 2. Compute item line totals + subtotal
		stages.ApplyPromotionsStage{},                   // 3. Evaluate promotions → discount adjustments
		stages.DeliveryFeeStage{BaseFee: 299},           // 4. Delivery fee (€2.99, waived if free delivery promo)
		stages.TaxStage{VATPercent: 7},                  // 5. VAT at 7% on adjusted totals
		stages.RoundingStage{                            // 6. Round to nearest 5 cents (EUR)
			Policy: domain.RoundingPolicy{
				ID:        "rounding-eur-v1",
				Version:   "1.0.0",
				Method:    domain.RoundHalfUp,
				Scope:     domain.RoundOrderTotal,
				Increment: 5, // Round to nearest 5 cents
			},
		},
		stages.FinalizeStage{},                          // 7. Aggregate totals + validate
	)

	// ── Calculate and persist two snapshots ─────────────────────────────
	for i, cart := range carts {
		// Create a new calculation context for each cart.
		calcCtx := engine.NewContext(cart, time.Now())
		calcCtx.Promotions = promotions

		// Execute the pricing pipeline.
		if err := e.Calculate(calcCtx); err != nil {
			log.Fatalf("calculation error (cart %d): %v", i+1, err)
		}

		// Copy promotion traces from the engine trace to the snapshot for persistence.
		// This records which promotions were applied/skipped and why.
		for _, pt := range calcCtx.Trace.Promotions {
			calcCtx.Snapshot.PromotionTraces = append(calcCtx.Snapshot.PromotionTraces, domain.PromotionTrace{
				PromotionID: pt.PromoID,
				Code:        pt.PromoCode,
				Status:      pt.Status,
				Reason:      pt.Reason,
			})
		}

		// Persist the snapshot as a new version for this order.
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