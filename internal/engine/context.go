package engine

import (
	"time"

	"pricing-engine/internal/domain"
)

type CalcContext struct {
	Cart     domain.Cart
	Snapshot domain.PriceSnapshot

	Now time.Time

	Promotions []domain.Promotion

	Trace     CalcTrace
	StageLogs []StageLog
}

type StageLog struct {
	StageName string
	StartedAt time.Time
	EndedAt   time.Time
	Error     string
}

func NewContext(cart domain.Cart, now time.Time) *CalcContext {
	return &CalcContext{
		Cart: cart,
		Now:  now,
		Snapshot: domain.PriceSnapshot{
			Version: "pricing-v2",
		},
	}
}

func (c *CalcContext) GetCart() domain.Cart {
	return c.Cart
}

func (c *CalcContext) GetSnapshot() domain.PriceSnapshot {
	return c.Snapshot
}
