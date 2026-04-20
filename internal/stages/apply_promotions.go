package stages

import (
	"sort"

	"pricing-engine/internal/domain"
	"pricing-engine/internal/engine"
)

// ApplyPromotionsStage is the third stage in the pricing pipeline.
// It evaluates all available promotions against the current cart and snapshot,
// applying eligible promotions as discount adjustments.
//
// # Promotion Evaluation Algorithm
//
// Promotions are processed deterministically using the following algorithm:
//
//  1. Sort promotions by Priority (descending — higher priority first)
//  2. For each promotion, check (in order):
//     a. Validity window: is ctx.Now within [ValidFrom, ValidTo]?
//     b. Coupon requirement: if RequiresCoupon, does Cart.Coupon.Code match Promo.Code?
//     c. Group exclusivity: if the promo has a Group, has another promo in that Group already applied?
//     d. Conditions: do all Condition.Evaluate() calls return true?
//  3. If all checks pass, apply all Benefits to generate adjustments
//  4. If the promotion is not Stackable, stop evaluating further promotions
//
// Every evaluated promotion (applied or skipped) produces a trace entry in
// ctx.Trace.Promotions for debugging and auditability.
//
// # Stacking and Exclusivity
//
//   - Stackable=true: allows the next promotion to be evaluated after this one applies
//   - Stackable=false: halts promotion evaluation entirely (acts as a "best offer" gate)
//   - Group: only one promotion per group can apply (e.g., "ORDER_DISCOUNT" group ensures
//     only one order-level discount applies). A second promo in the same group is skipped
//     with reason "group_exclusive_conflict"
//
// # Item-Level vs Order-Level Adjustments
//
// Benefits return two slices: (itemAdjustments, orderAdjustments).
//   - Item adjustments are matched to ItemSnapshots by target ("ITEM:<SKU>") and appended
//     to the matching ItemSnapshot.Adjustments
//   - Order adjustments are appended to Snapshot.Adjustments
//
// After all promotions are processed, recalculateItemTotals() updates each item's
// FinalTotal to reflect: LineTotal + sum(item-level adjustments).
//
// # Dependencies
//
// Must run AFTER SubtotalStage (needs Snapshot.Subtotal and Snapshot.Items).
// Must run BEFORE DeliveryFeeStage (free delivery is a promotion benefit).
// Must run BEFORE TaxStage (tax is computed on post-discount amounts).
type ApplyPromotionsStage struct{}

func (s ApplyPromotionsStage) Name() string { return "apply_promotions" }

func (s ApplyPromotionsStage) Execute(ctx *engine.CalcContext) error {
	// Sort promotions by priority descending (highest priority evaluated first).
	// This ensures deterministic evaluation order.
	sort.Slice(ctx.Promotions, func(i, j int) bool {
		return ctx.Promotions[i].Priority > ctx.Promotions[j].Priority
	})

	// Track which exclusivity groups have already had a promotion applied.
	appliedGroups := map[string]bool{}

	for _, promo := range ctx.Promotions {
		// ── Check 1: Validity window ──────────────────────────────────────
		// Skip promotions whose time window doesn't include the current time.
		if ctx.Now.Before(promo.ValidFrom) || ctx.Now.After(promo.ValidTo) {
			ctx.Trace.Promotions = append(ctx.Trace.Promotions, engine.PromoTrace{
				PromoID: promo.ID, PromoCode: promo.Code,
				Status: "SKIPPED", Reason: "outside_validity_window",
			})
			continue
		}

		// ── Check 2: Coupon requirement ───────────────────────────────────
		// If the promotion requires a coupon, verify the cart has a matching coupon code.
		if promo.RequiresCoupon {
			if ctx.Cart.Coupon == nil || ctx.Cart.Coupon.Code != promo.Code {
				ctx.Trace.Promotions = append(ctx.Trace.Promotions, engine.PromoTrace{
					PromoID: promo.ID, PromoCode: promo.Code,
					Status: "SKIPPED", Reason: "coupon_not_present",
				})
				continue
			}
		}

		// ── Check 3: Group exclusivity ────────────────────────────────────
		// Only one promotion per exclusivity group can apply.
		if promo.Group != "" && appliedGroups[promo.Group] {
			ctx.Trace.Promotions = append(ctx.Trace.Promotions, engine.PromoTrace{
				PromoID: promo.ID, PromoCode: promo.Code,
				Status: "SKIPPED", Reason: "group_exclusive_conflict",
			})
			continue
		}

		// ── Check 4: Evaluate all conditions ──────────────────────────────
		// All conditions must pass for the promotion to apply.
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

		// ── Apply benefits ────────────────────────────────────────────────
		// Each benefit generates item-level and/or order-level adjustments.
		for _, benefit := range promo.Benefits {
			itemAdjs, orderAdjs, err := benefit.Apply(ctx)
			if err != nil {
				return err
			}

			// Route item-level adjustments to the matching ItemSnapshot.
			applyItemAdjustments(ctx, itemAdjs)
			// Append order-level adjustments to the snapshot.
			ctx.Snapshot.Adjustments = append(ctx.Snapshot.Adjustments, orderAdjs...)
		}

		// Mark this promotion's group as used (for exclusivity enforcement).
		if promo.Group != "" {
			appliedGroups[promo.Group] = true
		}

		// Record successful application in the trace.
		ctx.Trace.Promotions = append(ctx.Trace.Promotions, engine.PromoTrace{
			PromoID: promo.ID, PromoCode: promo.Code,
			Status: "APPLIED", Reason: "ok",
		})

		// If this promotion is not stackable, stop evaluating further promotions.
		// This implements "best offer" behavior where the highest-priority non-stackable
		// promotion wins and no further discounts can apply.
		if !promo.Stackable {
			break
		}
	}

	// After all promotions are processed, recalculate each item's FinalTotal
	// to reflect the item-level adjustments that were applied.
	recalculateItemTotals(ctx)
	return nil
}

// applyItemAdjustments routes item-level adjustments to their matching ItemSnapshots.
//
// Each adjustment's Target field (e.g., "ITEM:burger") is matched against the
// ItemSnapshot's SKU. When a match is found, the adjustment is appended to that
// item's Adjustments slice.
//
// This "scattering" of adjustments to individual items is essential for:
//   - Partial refunds (remove item + its adjustments)
//   - Per-item tax calculation (discount reduces taxable base per item)
//   - Invoice line-item breakdown
//   - Reporting by SKU/category
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

// recalculateItemTotals updates each item's FinalTotal after item-level adjustments
// have been applied.
//
// FinalTotal = LineTotal + sum(all item-level adjustment amounts)
//
// This must be called after all item-level adjustments are applied (after promotions
// and rounding). The FinalTotal is used by downstream stages:
//   - TaxStage uses FinalTotal to compute the tax base
//   - FinalizeStage uses FinalTotal to verify the order total
func recalculateItemTotals(ctx *engine.CalcContext) {
	for i := range ctx.Snapshot.Items {
		total := ctx.Snapshot.Items[i].LineTotal
		for _, adj := range ctx.Snapshot.Items[i].Adjustments {
			total = total.Add(adj.Amount)
		}
		ctx.Snapshot.Items[i].FinalTotal = total
	}
}