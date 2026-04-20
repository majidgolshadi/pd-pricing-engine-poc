package stages

import (
	"fmt"

	"pricing-engine/internal/domain"
	"pricing-engine/internal/engine"
)

// RoundingStage applies a configurable rounding policy to the in-progress snapshot.
// It must run after TaxStage and before FinalizeStage so that rounding is applied
// to the fully-adjusted amount and is persisted as an explicit ROUNDING adjustment.
type RoundingStage struct {
	Policy domain.RoundingPolicy
}

func (s RoundingStage) Name() string { return "rounding" }

func (s RoundingStage) Execute(ctx *engine.CalcContext) error {
	switch s.Policy.Scope {
	case domain.RoundOrderTotal:
		return s.roundOrderTotal(ctx)
	case domain.RoundPerItem:
		return s.roundPerItem(ctx)
	case domain.RoundPerTax:
		return s.roundPerTax(ctx)
	}
	return nil
}

// roundOrderTotal rounds the entire running total and emits a single ROUNDING adjustment.
func (s RoundingStage) roundOrderTotal(ctx *engine.CalcContext) error {
	raw := computeRunningTotal(ctx)

	increment := s.Policy.Increment
	if increment <= 0 {
		increment = 1
	}

	rounded := domain.ApplyRounding(raw.Amount, increment, s.Policy.Method)
	delta := rounded - raw.Amount

	if delta == 0 {
		return nil
	}

	ctx.Snapshot.Adjustments = append(ctx.Snapshot.Adjustments, s.makeAdj(delta, "ORDER", ctx.Cart.Currency))
	return nil
}

// roundPerItem rounds each item's FinalTotal and emits an item-level ROUNDING adjustment
// for items where a delta exists. FinalTotal is updated in place so downstream stages
// (finalize) and the output snapshot reflect the rounded value.
func (s RoundingStage) roundPerItem(ctx *engine.CalcContext) error {
	increment := s.Policy.Increment
	if increment <= 0 {
		increment = 1
	}

	for i := range ctx.Snapshot.Items {
		item := &ctx.Snapshot.Items[i]
		rounded := domain.ApplyRounding(item.FinalTotal.Amount, increment, s.Policy.Method)
		delta := rounded - item.FinalTotal.Amount

		if delta == 0 {
			continue
		}

		adj := s.makeAdj(delta, "ITEM:"+item.SKU, ctx.Cart.Currency)
		item.Adjustments = append(item.Adjustments, adj)
		item.FinalTotal = domain.NewMoney(rounded, ctx.Cart.Currency)
	}

	return nil
}

// roundPerTax rounds each individual TAX-type order adjustment, updating the adjustment
// amount and emitting a corresponding ROUNDING adjustment for the delta.
func (s RoundingStage) roundPerTax(ctx *engine.CalcContext) error {
	increment := s.Policy.Increment
	if increment <= 0 {
		increment = 1
	}

	var roundingAdjs []domain.Adjustment

	for i := range ctx.Snapshot.Adjustments {
		adj := &ctx.Snapshot.Adjustments[i]
		if adj.Type != domain.AdjTax {
			continue
		}

		rounded := domain.ApplyRounding(adj.Amount.Amount, increment, s.Policy.Method)
		delta := rounded - adj.Amount.Amount

		if delta == 0 {
			continue
		}

		adj.Amount = domain.NewMoney(rounded, ctx.Cart.Currency)
		roundingAdjs = append(roundingAdjs, s.makeAdj(delta, "ORDER", ctx.Cart.Currency))
	}

	ctx.Snapshot.Adjustments = append(ctx.Snapshot.Adjustments, roundingAdjs...)
	return nil
}

// makeAdj builds a ROUNDING adjustment with full policy metadata for traceability.
func (s RoundingStage) makeAdj(delta int64, target, currency string) domain.Adjustment {
	return domain.Adjustment{
		ID:          "ROUNDING",
		Type:        domain.AdjRounding,
		Target:      target,
		Amount:      domain.NewMoney(delta, currency),
		ReasonCode:  "ROUNDING",
		Description: "Rounding adjustment",
		Metadata: map[string]string{
			"policy_id":      s.Policy.ID,
			"policy_version": s.Policy.Version,
			"method":         string(s.Policy.Method),
			"scope":          string(s.Policy.Scope),
			"increment":      fmt.Sprintf("%d", s.Policy.Increment),
		},
	}
}

// computeRunningTotal accumulates the order total exactly as FinalizeStage would,
// but without writing to the snapshot. Used by ORDER_TOTAL scope to determine the
// pre-rounding amount.
func computeRunningTotal(ctx *engine.CalcContext) domain.Money {
	total := domain.NewMoney(0, ctx.Cart.Currency)

	for _, it := range ctx.Snapshot.Items {
		total = total.Add(it.LineTotal)
		for _, adj := range it.Adjustments {
			total = total.Add(adj.Amount)
		}
	}

	for _, adj := range ctx.Snapshot.Adjustments {
		total = total.Add(adj.Amount)
	}

	return total
}
