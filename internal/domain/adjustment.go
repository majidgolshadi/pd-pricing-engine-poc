package domain

type AdjustmentType string

const (
	AdjDiscount AdjustmentType = "DISCOUNT"
	AdjFee      AdjustmentType = "FEE"
	AdjTax      AdjustmentType = "TAX"
	AdjRounding AdjustmentType = "ROUNDING"
)

type Adjustment struct {
	ID          string
	Type        AdjustmentType
	Target      string // "ORDER" or "ITEM:<SKU>"
	Amount      Money
	ReasonCode  string
	Description string
	Metadata    map[string]string
}
