package domain

import "time"

// ItemSnapshot holds the pricing result for a single line item after the calculation pipeline.
//
// Each LineItem from the Cart is converted into an ItemSnapshot during the subtotal stage.
// As the pipeline progresses, item-level adjustments (discounts, rounding) are appended
// to the Adjustments slice, and FinalTotal is updated to reflect: LineTotal + sum(item adjustments).
//
// # Why Per-Item Snapshots?
//
// Storing pricing results at the item level (rather than only order-level totals) is essential for:
//   - Partial refunds: remove the cancelled item + its adjustments, recompute totals safely
//   - Tax compliance: different items may have different tax rates (food vs alcohol vs service)
//   - Customer invoices: show a clean price breakdown per line item
//   - Reporting/BI: discount analysis by SKU, category, or campaign
//   - Audit replay: item-level adjustments match item totals directly for verification
//
// # Fields
//
//   - SKU: product identifier, matches LineItem.SKU
//   - Quantity: number of units, copied from LineItem
//   - UnitPrice: price per unit in minor currency units
//   - LineTotal: UnitPrice × Quantity (before any adjustments)
//   - Adjustments: all item-level adjustments (discounts, rounding) applied to this item
//   - FinalTotal: LineTotal + sum(adjustment amounts); the actual amount for this line
type ItemSnapshot struct {
	SKU         string       // Product identifier
	Quantity    int64        // Number of units
	UnitPrice   Money        // Price per unit (minor currency units)
	LineTotal   Money        // UnitPrice × Quantity (pre-adjustment)
	Adjustments []Adjustment // Item-level adjustments (DISCOUNT, ROUNDING)
	FinalTotal  Money        // LineTotal + sum(adjustments); the effective line amount
}

// PromotionTrace records whether a promotion was applied or skipped during calculation.
//
// This trace is extremely valuable for debugging and customer support. It answers questions like:
//   - "Why didn't my coupon work?" → Status=SKIPPED, Reason="coupon_not_present"
//   - "Why didn't I get the discount?" → Status=SKIPPED, Reason="subtotal_below_1000"
//   - "Which promotions were applied?" → filter by Status=APPLIED
//
// The trace is populated during the apply_promotions stage and persisted as part of the
// PriceSnapshot. Every evaluated promotion produces a trace entry, regardless of outcome.
//
// Possible skip reasons include:
//   - "outside_validity_window": current time is outside ValidFrom/ValidTo
//   - "coupon_not_present": promotion requires a coupon that wasn't provided
//   - "group_exclusive_conflict": another promotion in the same exclusivity group already applied
//   - Custom condition reasons (e.g., "subtotal_below_1000", "sku_not_found")
type PromotionTrace struct {
	PromotionID string // Unique promotion identifier
	Code        string // Promotion code (for display/debugging)
	Status      string // "APPLIED" or "SKIPPED"
	Reason      string // "ok" if applied; skip reason otherwise
}

// PriceSnapshot is the immutable pricing result produced by the calculation engine.
//
// # Immutable Price Snapshot at Checkout
//
// At the time of checkout, the system persists a complete PriceSnapshot including:
//   - Per-item totals and adjustments (Items)
//   - Order-level adjustments: discounts, fees, taxes, rounding (Adjustments)
//   - Aggregated totals: subtotal, discounts, fees, tax, rounding, total (the Money fields)
//   - Engine version for traceability (Version)
//   - Promotion execution trace (PromotionTraces)
//
// Once persisted (wrapped in an OrderSnapshot), this snapshot is treated as immutable
// and becomes the source of truth for:
//   - Invoice generation
//   - Customer support explanations
//   - Order history display
//   - Financial reconciliation
//
// IMPORTANT: Historical orders should NEVER be recomputed using the latest pricing logic.
// The snapshot captures the exact pricing state at the time of calculation.
//
// # Total Derivation
//
// The Total field is derived as:
//   Total = Subtotal + Discounts + DeliveryFee + Tax + Rounding
//
// Where Discounts is negative (sum of all DISCOUNT adjustments), and DeliveryFee, Tax
// are positive (sum of FEE and TAX adjustments respectively). Rounding is typically ±1 minor unit.
//
// # Versioning for Traceability
//
// The Version field stores the pricing engine version (e.g., "pricing-v2") so that any
// order can be traced to the exact code that produced the result. In production, this
// should also be accompanied by rule/config versions (promotion config, tax rules,
// delivery fee rules, rounding policy) stored in adjustment metadata.
type PriceSnapshot struct {
	Items []ItemSnapshot // Per-item pricing results with item-level adjustments

	Subtotal    Money // Sum of all item LineTotals (before adjustments)
	Discounts   Money // Sum of all DISCOUNT adjustments (negative value)
	DeliveryFee Money // Sum of all FEE adjustments (delivery + service fees)
	Tax         Money // Sum of all TAX adjustments
	Rounding    Money // Sum of all ROUNDING adjustments (typically ±1 minor unit)
	Total       Money // Final payable amount: Subtotal + Discounts + Fees + Tax + Rounding

	Adjustments []Adjustment // Order-level adjustments only (discounts, fees, taxes, rounding)
	Version     string       // Pricing engine version for traceability (e.g., "pricing-v2")

	PromotionTraces []PromotionTrace // Trace of all evaluated promotions (applied and skipped)
}

// OrderSnapshot wraps a PriceSnapshot with order-level metadata for persistence.
//
// This is the top-level entity stored in the database (see SnapshotRepository).
// It associates a PriceSnapshot with an order code and supports versioning so that
// multiple pricing snapshots can exist for the same order (e.g., price recalculation
// after cart modification before final checkout).
//
// # Versioning Strategy
//
// Each Save() call auto-increments the Version number. This allows:
//   - Tracking price changes as the cart is modified
//   - Safe rollback to a previous pricing state
//   - Audit trail of how the price evolved before final checkout
//
// The combination of OrderCode + Version uniquely identifies a snapshot.
type OrderSnapshot struct {
	OrderCode string         // Order identifier (partition key in persistence)
	Version   int            // Auto-incremented version number (sort key in persistence)
	CreatedAt time.Time      // UTC timestamp when this snapshot was created
	Currency  string         // ISO 4217 currency code for all monetary values
	Snapshot  PriceSnapshot  // The complete, immutable pricing result
}