package engine

type Stage interface {
	Name() string
	Execute(ctx *CalcContext) error
}
