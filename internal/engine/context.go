package engine

import (
	"time"

	"pricing-engine/internal/domain"
)

// CalcContext is the shared mutable state that flows through the pricing pipeline.
//
// # Pipeline-Based Calculation Engine
//
// The pricing engine executes a deterministic pipeline where each stage has a single
// responsibility and mutates this shared calculation context. The pipeline stages are:
//
//  1. NormalizeStage:        Validate cart (currency, items, quantities, prices)
//  2. SubtotalStage:         Compute item line totals and order subtotal
//  3. ApplyPromotionsStage:  Evaluate promotions and generate discount adjustments
//  4. DeliveryFeeStage:      Compute delivery and service fee adjustments
//  5. TaxStage:              Compute tax based on adjusted totals
//  6. RoundingStage:         Apply rounding rules and emit rounding adjustments
//  7. FinalizeStage:         Aggregate all adjustments into final totals and validate
//
// Each stage reads from and writes to CalcContext. The Cart field is the immutable input;
// the Snapshot field accumulates the pricing result as stages execute.
//
// # Implements domain.PromotionContext
//
// CalcContext implements the domain.PromotionContext interface (GetCart, GetSnapshot),
// allowing promotion conditions and benefits to inspect the current cart and in-progress
// snapshot without needing a direct dependency on the engine package.
//
// # Tracing
//
// CalcContext carries two trace mechanisms:
//   - Trace: records promotion evaluation results (applied/skipped with reasons)
//   - StageLogs: records timing and error information for each pipeline stage
//
// These traces are invaluable for debugging pricing issues and understanding
// exactly how a total was computed.
type CalcContext struct {
	// Cart is the immutable input to the pricing pipeline.
	// Set once during NewContext() and never modified by stages.
	Cart domain.Cart

	// Snapshot is the mutable pricing result built up by pipeline stages.
	// Each stage appends adjustments, updates totals, or adds trace information.
	// After the pipeline completes, this contains the final PriceSnapshot.
	Snapshot domain.PriceSnapshot

	// Now is the reference timestamp for the calculation.
	// Used by the promotion stage to check validity windows (ValidFrom/ValidTo).
	// Should be set to the current time at the start of calculation.
	Now time.Time

	// Promotions is the list of available promotions to evaluate.
	// These are set by the caller before running the engine (not by any stage).
	// The ApplyPromotionsStage processes these in priority order.
	Promotions []domain.Promotion

	// Trace records promotion evaluation results (applied/skipped with reasons).
	// Populated by the ApplyPromotionsStage. After calculation, these are typically
	// copied to Snapshot.PromotionTraces for persistence.
	Trace CalcTrace

	// StageLogs records timing and error information for each pipeline stage.
	// Populated by the Engine as it executes stages. Useful for performance
	// monitoring and debugging slow or failing stages.
	StageLogs []StageLog
}

// StageLog records execution metadata for a single pipeline stage.
//
// Each stage execution produces a StageLog entry with:
//   - StageName: identifies which stage ran (e.g., "normalize", "subtotal", "apply_promotions")
//   - StartedAt/EndedAt: wall-clock timestamps for performance monitoring
//   - Error: error message if the stage failed (empty string on success)
type StageLog struct {
	StageName string    // Stage identifier (from Stage.Name())
	StartedAt time.Time // When the stage started executing
	EndedAt   time.Time // When the stage finished executing
	Error     string    // Error message if failed; empty on success
}

// NewContext creates a CalcContext initialized with the given cart and timestamp.
//
// The Snapshot is initialized with the engine version string ("pricing-v2") for
// traceability. In production, this version should match the deployed build version
// so that any persisted snapshot can be traced to the exact code that produced it.
//
// Usage:
//
//	ctx := engine.NewContext(cart, time.Now())
//	ctx.Promotions = promotions  // attach available promotions
//	err := engine.Calculate(ctx) // run the pipeline
//	result := ctx.Snapshot       // read the pricing result
func NewContext(cart domain.Cart, now time.Time) *CalcContext {
	return &CalcContext{
		Cart: cart,
		Now:  now,
		Snapshot: domain.PriceSnapshot{
			Version: "pricing-v2",
		},
	}
}

// GetCart returns the input cart. Implements domain.PromotionContext.
func (c *CalcContext) GetCart() domain.Cart {
	return c.Cart
}

// GetSnapshot returns the current (in-progress) price snapshot. Implements domain.PromotionContext.
// During promotion evaluation, this allows conditions and benefits to inspect the
// current subtotal and other computed values.
func (c *CalcContext) GetSnapshot() domain.PriceSnapshot {
	return c.Snapshot
}