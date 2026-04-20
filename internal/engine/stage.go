package engine

// Stage is the interface that all pricing pipeline stages must implement.
//
// Each stage represents a single step in the pricing calculation pipeline.
// Stages are executed sequentially by the Engine in the order they were registered.
// Each stage mutates the shared CalcContext to build up the final PriceSnapshot.
//
// # Implementing a New Stage
//
// To add a new pipeline stage:
//  1. Create a struct in the stages package (e.g., ServiceFeeStage)
//  2. Implement Name() to return a unique identifier (e.g., "service_fee")
//  3. Implement Execute() to read from and write to the CalcContext
//  4. Register the stage in the Engine pipeline at the appropriate position
//
// # Stage Ordering
//
// The order in which stages are registered in NewEngine() is critical:
//   - normalize → subtotal → apply_promotions → delivery_fee → tax → rounding → finalize
//
// Each stage may depend on values computed by previous stages (e.g., tax depends on
// the adjusted item totals computed by promotions and delivery fee stages).
//
// # Error Handling
//
// If Execute() returns an error, the Engine halts the pipeline immediately and
// returns the error wrapped with the stage name for diagnostics. The error is
// also recorded in CalcContext.StageLogs.
//
// # Current Stage Implementations (in the stages package)
//
//   - NormalizeStage: validates cart has currency, items, valid quantities and prices
//   - SubtotalStage: computes item line totals and order subtotal
//   - ApplyPromotionsStage: evaluates promotions and generates discount adjustments
//   - DeliveryFeeStage: computes delivery fee (with free-delivery override support)
//   - TaxStage: computes VAT/tax based on adjusted totals
//   - RoundingStage: applies configurable rounding policy and emits rounding adjustments
//   - FinalizeStage: aggregates all adjustments into final totals and validates the result
type Stage interface {
	// Name returns a unique identifier for this stage (e.g., "normalize", "subtotal").
	// Used in error messages, stage logs, and debugging output.
	Name() string

	// Execute performs this stage's computation, reading from and writing to the CalcContext.
	// Returns an error if the stage fails (e.g., validation error, computation error).
	// On error, the pipeline halts and no further stages are executed.
	Execute(ctx *CalcContext) error
}