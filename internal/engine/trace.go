package engine

// PromoTrace records the evaluation result of a single promotion during the
// apply_promotions pipeline stage.
//
// Every promotion that is evaluated (whether applied or skipped) produces a PromoTrace
// entry. This data is stored in CalcContext.Trace during calculation, and after the
// pipeline completes, it is typically copied to the PriceSnapshot.PromotionTraces
// (as domain.PromotionTrace) for persistence.
//
// # Why Trace Promotions?
//
// Promotion tracing is critical for:
//   - Debugging: understanding exactly which promotions fired and why others didn't
//   - Customer support: answering "Why didn't my coupon work?"
//   - Validation: verifying that promotion configurations behave as expected
//   - Audit: providing a complete record of pricing decisions for compliance
//
// # Status Values
//
//   - "APPLIED": the promotion passed all checks and its benefits were applied
//   - "SKIPPED": the promotion was not applied, with the reason recorded in Reason
//
// # Common Skip Reasons
//
//   - "outside_validity_window": current time is before ValidFrom or after ValidTo
//   - "coupon_not_present": promotion requires a coupon code that wasn't provided
//   - "group_exclusive_conflict": another promotion in the same exclusivity group already applied
//   - Custom condition reasons from Condition.Evaluate() (e.g., "subtotal_below_1000", "sku_not_found")
type PromoTrace struct {
	PromoID   string // Unique promotion identifier
	PromoCode string // Promotion code (for display/debugging)
	Status    string // "APPLIED" or "SKIPPED"
	Reason    string // "ok" if applied; skip reason if skipped
}

// CalcTrace aggregates all promotion evaluation traces for a single calculation run.
//
// This is populated by the ApplyPromotionsStage as it iterates through promotions
// in priority order. After calculation, the caller should copy these traces to
// the PriceSnapshot for persistence:
//
//	for _, pt := range calcCtx.Trace.Promotions {
//	    calcCtx.Snapshot.PromotionTraces = append(calcCtx.Snapshot.PromotionTraces,
//	        domain.PromotionTrace{
//	            PromotionID: pt.PromoID,
//	            Code:        pt.PromoCode,
//	            Status:      pt.Status,
//	            Reason:      pt.Reason,
//	        })
//	}
type CalcTrace struct {
	Promotions []PromoTrace // All promotion evaluation results in evaluation order
}