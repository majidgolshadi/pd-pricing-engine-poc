// Package domain defines the core types for the adjustment-based pricing model.
//
// # Adjustment-Based Accounting Model
//
// This pricing system uses an adjustment-based accounting model where ALL price-impacting
// operations are represented as explicit charge components ("adjustments"), rather than
// being embedded as implicit arithmetic inside totals.
//
// The flow is:
//  1. The cart generates a base subtotal (items × quantity × unit price).
//  2. All subsequent price modifications (discounts, fees, taxes, rounding) are recorded as adjustments.
//  3. The final order total is derived from: base subtotal + sum(adjustments).
//
// This pattern is widely used in ordering and commerce systems because it produces a pricing
// result that is explainable, auditable, and reconcilable.
//
// # Why Adjustments Matter
//
//   - Auditability: The system can always answer "Why is the total €18.40?" by enumerating adjustments.
//   - Extensibility: New pricing features only require new adjustment producers (new promo benefits,
//     new fee logic), without changing the persistence format or calculation pipeline.
//   - Correctness: Finance and reporting require exact breakdowns. Adjustments provide a stable
//     representation for downstream billing, invoices, analytics, and reconciliation.
//   - Refunds: Per-item adjustments make partial refunds straightforward — remove the cancelled item
//     and its related adjustments, then recompute totals safely.
//   - Tax compliance: Per-item adjustments ensure correct taxable base per tax category (e.g.,
//     food vs alcohol vs service may have different VAT rates).
package domain

// AdjustmentType classifies the nature of a price modification.
//
// Every pricing rule in the system produces one or more adjustments, each tagged with a type
// that determines how it is aggregated in the final totals:
//   - DISCOUNT: reduces the payable amount (Amount is negative)
//   - FEE: adds a charge such as delivery or service fee (Amount is positive)
//   - TAX: adds a tax charge such as VAT/GST (Amount is positive)
//   - ROUNDING: corrects for currency rounding (Amount is typically ±1 minor unit)
type AdjustmentType string

const (
	// AdjDiscount represents a price reduction (e.g., voucher, promotion, coupon).
	// The Amount is always negative. Examples:
	//   - Voucher discount → DISCOUNT adjustment targeting ORDER
	//   - Buy X Get Y     → DISCOUNT adjustment targeting ITEM:<SKU>
	AdjDiscount AdjustmentType = "DISCOUNT"

	// AdjFee represents an additional charge (e.g., delivery fee, service fee).
	// The Amount is always positive. Examples:
	//   - Delivery fee → FEE adjustment targeting ORDER
	//   - Service fee  → FEE adjustment targeting ORDER
	AdjFee AdjustmentType = "FEE"

	// AdjTax represents a tax charge (e.g., VAT, GST).
	// The Amount is always positive. Example:
	//   - VAT → TAX adjustment targeting ORDER
	AdjTax AdjustmentType = "TAX"

	// AdjRounding represents a rounding correction applied to align the total
	// with the currency's smallest denomination (e.g., round to nearest 5 cents).
	// The Amount is typically ±1 minor unit. Example:
	//   - Rounding delta → ROUNDING adjustment targeting ORDER
	AdjRounding AdjustmentType = "ROUNDING"
)

// Adjustment is the universal pricing component. Every price-impacting operation in the
// system — discounts, fees, taxes, rounding — is expressed as an Adjustment.
//
// This unified structure allows the system to model any pricing effect consistently and
// makes the final total fully explainable as: subtotal + sum(all adjustments).
//
// # Item-Level vs Order-Level Adjustments
//
// Adjustments can target either the entire order ("ORDER") or a specific item ("ITEM:<SKU>").
//
// Multi-item promotions (e.g., "20% off all burgers") are typically "scattered" into multiple
// item-level adjustments, even though the promotion is logically a single rule. This is because:
//   - Refunds/partial cancellations become straightforward (remove item + its adjustments)
//   - Tax compliance requires knowing the discount per tax category
//   - Customer-facing invoices can show a clean breakdown per line item
//   - Accounting/BI can report discount by SKU/category accurately
//   - Audit replay is easier when item-level adjustments match item totals directly
//
// Some discounts are truly order-level and should stay as one adjustment:
//   - "€5 off the entire order"
//   - "10% off total basket"
//   - "free delivery"
//   - "payment method discount"
//
// # Fields
//
//   - ID: unique identifier for traceability and debugging
//   - Type: DISCOUNT / FEE / TAX / ROUNDING
//   - Target: "ORDER" for order-level, or "ITEM:<SKU>" for item-level
//   - Amount: monetary delta in minor currency units (int64); discounts are negative
//   - ReasonCode: machine-readable identifier (promotion code, tax code, fee rule ID)
//   - Description: human-readable explanation for support/debugging
//   - Metadata: extensible key-value pairs (promotion ID, campaign ID, rule version,
//     tax category, policy version, etc.)
type Adjustment struct {
	ID          string            // Unique identifier for traceability
	Type        AdjustmentType    // Classification: DISCOUNT, FEE, TAX, or ROUNDING
	Target      string            // Scope: "ORDER" or "ITEM:<SKU>"
	Amount      Money             // Monetary delta in minor units; discounts are negative
	ReasonCode  string            // Machine-readable reason (e.g., "PROMO10", "VAT", "DELIVERY_BASE")
	Description string            // Human-readable explanation
	Metadata    map[string]string // Extensible attributes (promotion_id, campaign, policy_version, etc.)
}