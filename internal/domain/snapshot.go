package domain

import "time"

// ItemSnapshot holds the pricing result for a single line item.
type ItemSnapshot struct {
	SKU         string
	Quantity    int64
	UnitPrice   Money
	LineTotal   Money
	Adjustments []Adjustment
	FinalTotal  Money
}

// PromotionTrace records whether a promotion was applied or skipped.
type PromotionTrace struct {
	PromotionID string
	Code        string
	Status      string // APPLIED / SKIPPED
	Reason      string
}

// PriceSnapshot is the immutable pricing result produced by the calculation engine.
type PriceSnapshot struct {
	Items []ItemSnapshot

	Subtotal    Money
	Discounts   Money
	DeliveryFee Money
	Tax         Money
	Rounding    Money
	Total       Money

	Adjustments []Adjustment // order-level adjustments only
	Version     string

	PromotionTraces []PromotionTrace
}

// OrderSnapshot wraps a PriceSnapshot with order-level metadata for persistence.
type OrderSnapshot struct {
	OrderCode string
	Version   int
	CreatedAt time.Time
	Currency  string
	Snapshot  PriceSnapshot
}