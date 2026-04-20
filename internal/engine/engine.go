// Package engine implements the pipeline-based pricing calculation engine.
//
// # Architecture Overview
//
// The pricing engine uses a deterministic pipeline architecture where pricing is computed
// by executing a sequence of stages, each with a single responsibility. Each stage mutates
// a shared CalcContext that carries the input cart, the in-progress price snapshot, promotion
// definitions, and trace/logging data.
//
// # Pipeline Flow
//
// The recommended pipeline order is:
//
//  1. NormalizeStage        — Validate and normalize the input cart
//  2. SubtotalStage         — Compute item line totals (qty × unit price) and order subtotal
//  3. ApplyPromotionsStage  — Evaluate promotions (conditions + benefits) and emit discount adjustments
//  4. DeliveryFeeStage      — Compute delivery/service fee adjustments
//  5. TaxStage              — Compute tax adjustments based on adjusted totals
//  6. RoundingStage         — Apply currency rounding rules and emit rounding adjustments
//  7. FinalizeStage         — Aggregate all adjustments into final totals and validate the result
//
// This pipeline design ensures:
//   - Modularity: each stage is independently testable
//   - Determinism: same input always produces the same output
//   - Extensibility: new stages can be inserted without modifying existing ones
//   - Traceability: stage logs record timing and errors for each step
//
// # Usage
//
//	e := engine.NewEngine(
//	    stages.NormalizeStage{},
//	    stages.SubtotalStage{},
//	    stages.ApplyPromotionsStage{},
//	    stages.DeliveryFeeStage{BaseFee: 299},
//	    stages.TaxStage{VATPercent: 7},
//	    stages.RoundingStage{Policy: roundingPolicy},
//	    stages.FinalizeStage{},
//	)
//	ctx := engine.NewContext(cart, time.Now())
//	ctx.Promotions = promotions
//	if err := e.Calculate(ctx); err != nil { ... }
//	snapshot := ctx.Snapshot // the final PriceSnapshot
package engine

import (
	"fmt"
	"time"
)

// Engine orchestrates the pricing calculation pipeline.
//
// It holds an ordered list of Stage implementations and executes them sequentially
// against a shared CalcContext. If any stage returns an error, the pipeline halts
// immediately and the error is returned (with the failing stage name for diagnostics).
//
// The Engine is stateless and safe for concurrent use — all mutable state lives in
// the CalcContext passed to Calculate().
type Engine struct {
	stages []Stage // Ordered list of pipeline stages to execute
}

// NewEngine creates a pricing engine with the given stages in the specified order.
//
// The order of stages is critical for correctness:
//   - Subtotal must run before promotions (promotions need the subtotal to compute % discounts)
//   - Promotions must run before delivery fee (free delivery is a promotion benefit)
//   - Tax must run after promotions and delivery (tax base = adjusted item totals + fees)
//   - Rounding must run after tax (rounding applies to the fully-adjusted amount)
//   - Finalize must run last (it aggregates all adjustments into final totals)
func NewEngine(stages ...Stage) *Engine {
	return &Engine{stages: stages}
}

// Calculate executes all pipeline stages in order against the given CalcContext.
//
// Each stage is timed and logged in ctx.StageLogs. If a stage fails, the error
// is recorded in the stage log and returned immediately with the stage name.
//
// After successful completion, ctx.Snapshot contains the final PriceSnapshot
// with all adjustments, totals, and traces populated.
func (e *Engine) Calculate(ctx *CalcContext) error {
	for _, stage := range e.stages {
		start := time.Now()
		err := stage.Execute(ctx)
		end := time.Now()

		log := StageLog{
			StageName: stage.Name(),
			StartedAt: start,
			EndedAt:   end,
		}

		if err != nil {
			log.Error = err.Error()
			ctx.StageLogs = append(ctx.StageLogs, log)
			return fmt.Errorf("stage %q failed: %w", stage.Name(), err)
		}

		ctx.StageLogs = append(ctx.StageLogs, log)
	}

	return nil
}