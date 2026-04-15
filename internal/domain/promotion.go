package domain

import "time"

type Promotion struct {
	ID        string
	Code      string
	Priority  int
	Stackable bool

	// Exclusivity group: only one promo in same group can apply
	Group string

	// Coupon required?
	RequiresCoupon bool

	ValidFrom time.Time
	ValidTo   time.Time

	Conditions []Condition
	Benefits   []Benefit
}

type Condition interface {
	Evaluate(ctx PromotionContext) (bool, string)
}

type Benefit interface {
	Apply(ctx PromotionContext) ([]Adjustment, []Adjustment, error)
	// returns (itemAdjustments, orderAdjustments, error)
}

type PromotionContext interface {
	GetCart() Cart
	GetSnapshot() PriceSnapshot
}
