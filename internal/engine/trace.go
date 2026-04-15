package engine

type PromoTrace struct {
	PromoID   string
	PromoCode string
	Status    string // APPLIED / SKIPPED
	Reason    string
}

type CalcTrace struct {
	Promotions []PromoTrace
}
