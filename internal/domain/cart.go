package domain

type LineItem struct {
	SKU       string
	Name      string
	Quantity  int64
	UnitPrice Money
}

type CouponInput struct {
	Code string
}

type Cart struct {
	ID       string
	StoreID  string
	UserID   string
	Currency string
	Items    []LineItem
	Coupon   *CouponInput
}
