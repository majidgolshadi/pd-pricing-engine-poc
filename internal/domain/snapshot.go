package domain

type ItemSnapshot struct {
	SKU         string
	Quantity    int64
	UnitPrice   Money
	LineTotal   Money
	Adjustments []Adjustment
	FinalTotal  Money
}

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
}
