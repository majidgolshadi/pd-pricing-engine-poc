package domain

// LineItem represents a single product line in the shopping cart.
//
// In the pricing pipeline, each LineItem is converted into an ItemSnapshot (see snapshot.go)
// during the subtotal stage. The line total is computed as: UnitPrice.Amount × Quantity.
//
// After promotions are applied, item-level adjustments (discounts, rounding) are attached
// to the ItemSnapshot, and the FinalTotal reflects the line total plus all item-level adjustments.
type LineItem struct {
	SKU       string // Unique product identifier (e.g., "burger", "cola")
	Name      string // Human-readable product name for display/debugging
	Quantity  int64  // Number of units ordered; must be > 0 (validated by NormalizeStage)
	UnitPrice Money  // Price per single unit in minor currency units (e.g., 599 = €5.99)
}

// CouponInput carries a coupon code submitted by the customer at checkout.
//
// The coupon code is matched against promotions that have RequiresCoupon=true.
// During the apply_promotions stage, if a promotion requires a coupon and the cart's
// coupon code does not match the promotion's Code, the promotion is skipped with
// reason "coupon_not_present" (recorded in the promotion trace for debugging).
type CouponInput struct {
	Code string // Customer-submitted coupon code (e.g., "PROMO10", "FREEDEL")
}

// Cart is the primary input to the pricing engine pipeline.
//
// It represents the customer's shopping cart at the time of price calculation.
// The engine processes this Cart through a series of pipeline stages (see engine.Engine)
// to produce a PriceSnapshot — an immutable pricing result containing the full
// breakdown of subtotals, adjustments, and the final payable amount.
//
// # Pipeline Input
//
// The Cart is passed to engine.NewContext() along with the current timestamp to create
// a CalcContext, which is then mutated by each pipeline stage:
//
//	ctx := engine.NewContext(cart, time.Now())
//	ctx.Promotions = promotions  // attach available promotions
//	engine.Calculate(ctx)        // run the pipeline
//	result := ctx.Snapshot       // read the immutable PriceSnapshot
//
// # Fields
//
//   - ID: unique cart identifier for tracing
//   - StoreID: identifies the store/restaurant (may affect delivery fees, tax rules)
//   - UserID: identifies the customer (may affect eligibility for promotions)
//   - Currency: ISO 4217 currency code (e.g., "EUR", "USD"); required, validated by NormalizeStage
//   - Items: list of products with quantities and unit prices; must not be empty
//   - Coupon: optional coupon code for coupon-gated promotions; nil if none provided
type Cart struct {
	ID       string       // Unique cart identifier
	StoreID  string       // Store/restaurant identifier (for location-specific pricing rules)
	UserID   string       // Customer identifier (for user-specific promotions)
	Currency string       // ISO 4217 currency code (e.g., "EUR"); all Money values use this currency
	Items    []LineItem   // Products in the cart; must contain at least one item
	Coupon   *CouponInput // Optional coupon code; nil if no coupon was applied
}