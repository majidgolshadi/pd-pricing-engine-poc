package stages

import (
	"fmt"

	"pricing-engine/internal/domain"
	"pricing-engine/internal/engine"
)

// RoundingStage is the sixth stage in the pricing pipeline.
// It applies a configurable rounding policy to the in-progress snapshot and emits
// explicit ROUNDING adjustments for any deltas.
//
// # Why Rounding is a Dedicated Stage
//
// Rounding must be treated as a first-class part of pricing logic, NOT a UI formatting
// step. Rounding differences affect:
//   - The payable amount (what is charged to the customer)
//   - Tax correctness (the tax base must be consistently rounded)
//   - Reconciliation with payment providers (who enforce currency precision)
//   - Invoice and ledger consistency (rounded totals must match across systems)
//
// By placing rounding near the end of the pipeline (after discounts, fees, and tax),
// rounding is applied to the fully-adjusted amount in a controlled, deterministic way.
//
// # Rounding as an Explicit Adjustment
//
// Any rounding delta is persisted as a ROUNDING adjustment with full policy metadata
// (policy ID, version, method, scope, increment). This ensures:
//   - Rounding is auditable and reproducible
//   - Historical totals reconcile correctly when recalculated
//   - The exact rounding policy can be traced for any order
//
// # Supported Scopes (see domain.RoundingScope)
//
//   - ORDER_TOTAL: rounds the running total and emits a single ROUNDING adjustment
//   - PER_ITEM: rounds each item's FinalTotal and emits item-level ROUNDING adjustments
//   - PER_TAX: rounds each TAX adjustment individually and emits corresponding ROUNDING adjustments
//
// # Dependencies
//
// Must run AFTER TaxStage (rounding applies to the fully-adjusted amount including tax).
// Must run BEFORE FinalizeStage (finalize aggregates all adjustments including rounding).
type RoundingStage struct {
	Policy domain.RoundingPolicy // Configurable rounding policy (method, scope, increment)
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
//
// This is the simplest and most common rounding scope. It computes the pre-rounding
// total (exactly as FinalizeStage would), applies the rounding method, and if there's
// a delta, emits an ORDER-level ROUNDING adjustment.
//
// Example: running total = 1463, increment = 5, method = HALF_UP
//   → rounded = 1465, delta = +2 → ROUNDING adjustment with Amount = +2
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

// roundPerItem rounds each item's FinalTotal and emits item-level ROUNDING adjustments
// for items where a delta exists.
//
// FinalTotal is updated in place so that downstream stages (finalize) and the output
// snapshot reflect the rounded value. This scope may produce multiple small rounding
// adjustments (one per item) rather than a single order-level one.
//
// This scope is useful when per-item precision is required (e.g., for itemized invoices).
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
// amount in place and emitting a corresponding ROUNDING adjustment for the delta.
//
// This scope is required by some tax jurisdictions where each tax component must be
// rounded independently before being summed into the total.
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

		// Update the tax adjustment amount in place to the rounded value.
		adj.Amount = domain.NewMoney(rounded, ctx.Cart.Currency)
		// Emit a ROUNDING adjustment recording the delta for auditability.
		roundingAdjs = append(roundingAdjs, s.makeAdj(delta, "ORDER", ctx.Cart.Currency))
	}

	ctx.Snapshot.Adjustments = append(ctx.Snapshot.Adjustments, roundingAdjs...)
	return nil
}

// makeAdj builds a ROUNDING adjustment with full policy metadata for traceability.
//
// The metadata includes the policy ID, version, method, scope, and increment so that
// any historical order's rounding can be fully traced and reproduced.
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
// but without writing to the snapshot.
//
// It sums:
//   - All item LineTotals + item-level adjustments (which equals item FinalTotals)
//   - All order-level adjustments (discounts, fees, taxes)
//
// This is used by the ORDER_TOTAL scope to determine the pre-rounding amount.
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