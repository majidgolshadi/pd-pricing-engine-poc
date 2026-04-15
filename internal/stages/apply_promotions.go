package stages

import (
	"sort"

	"pricing-engine/internal/domain"
	"pricing-engine/internal/engine"
)

type ApplyPromotionsStage struct{}

func (s ApplyPromotionsStage) Name() string { return "apply_promotions" }

func (s ApplyPromotionsStage) Execute(ctx *engine.CalcContext) error {
	sort.Slice(ctx.Promotions, func(i, j int) bool {
		return ctx.Promotions[i].Priority > ctx.Promotions[j].Priority
	})

	appliedGroups := map[string]bool{}

	for _, promo := range ctx.Promotions {
		// validity window
		if ctx.Now.Before(promo.ValidFrom) || ctx.Now.After(promo.ValidTo) {
			ctx.Trace.Promotions = append(ctx.Trace.Promotions, engine.PromoTrace{
				PromoID: promo.ID, PromoCode: promo.Code,
				Status: "SKIPPED", Reason: "outside_validity_window",
			})
			continue
		}

		// coupon requirement
		if promo.RequiresCoupon {
			if ctx.Cart.Coupon == nil || ctx.Cart.Coupon.Code != promo.Code {
				ctx.Trace.Promotions = append(ctx.Trace.Promotions, engine.PromoTrace{
					PromoID: promo.ID, PromoCode: promo.Code,
					Status: "SKIPPED", Reason: "coupon_not_present",
				})
				continue
			}
		}

		// group exclusivity
		if promo.Group != "" && appliedGroups[promo.Group] {
			ctx.Trace.Promotions = append(ctx.Trace.Promotions, engine.PromoTrace{
				PromoID: promo.ID, PromoCode: promo.Code,
				Status: "SKIPPED", Reason: "group_exclusive_conflict",
			})
			continue
		}

		ok := true
		reason := ""

		for _, cond := range promo.Conditions {
			pass, r := cond.Evaluate(ctx)
			if !pass {
				ok = false
				reason = r
				break
			}
		}

		if !ok {
			ctx.Trace.Promotions = append(ctx.Trace.Promotions, engine.PromoTrace{
				PromoID: promo.ID, PromoCode: promo.Code,
				Status: "SKIPPED", Reason: reason,
			})
			continue
		}

		// apply benefits
		for _, benefit := range promo.Benefits {
			itemAdjs, orderAdjs, err := benefit.Apply(ctx)
			if err != nil {
				return err
			}

			applyItemAdjustments(ctx, itemAdjs)
			ctx.Snapshot.Adjustments = append(ctx.Snapshot.Adjustments, orderAdjs...)
		}

		if promo.Group != "" {
			appliedGroups[promo.Group] = true
		}

		ctx.Trace.Promotions = append(ctx.Trace.Promotions, engine.PromoTrace{
			PromoID: promo.ID, PromoCode: promo.Code,
			Status: "APPLIED", Reason: "ok",
		})

		if !promo.Stackable {
			break
		}
	}

	recalculateItemTotals(ctx)
	return nil
}

func applyItemAdjustments(ctx *engine.CalcContext, adjs []domain.Adjustment) {
	for _, adj := range adjs {
		for i := range ctx.Snapshot.Items {
			target := "ITEM:" + ctx.Snapshot.Items[i].SKU
			if adj.Target == target {
				ctx.Snapshot.Items[i].Adjustments = append(ctx.Snapshot.Items[i].Adjustments, adj)
			}
		}
	}
}

func recalculateItemTotals(ctx *engine.CalcContext) {
	for i := range ctx.Snapshot.Items {
		total := ctx.Snapshot.Items[i].LineTotal
		for _, adj := range ctx.Snapshot.Items[i].Adjustments {
			total = total.Add(adj.Amount)
		}
		ctx.Snapshot.Items[i].FinalTotal = total
	}
}
