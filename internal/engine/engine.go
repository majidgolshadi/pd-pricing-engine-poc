package engine

import (
	"fmt"
	"time"
)

type Engine struct {
	stages []Stage
}

func NewEngine(stages ...Stage) *Engine {
	return &Engine{stages: stages}
}

func (e *Engine) Calculate(ctx *CalcContext) error {
	for _, stage := range e.stages {
		start := time.Now()
		err := stage.Execute(ctx)
		end := time.Now()

		log := StageLog{
			StageName: stage.Name(),
			StartedAt: start,
			EndedAt:   end,
		}

		if err != nil {
			log.Error = err.Error()
			ctx.StageLogs = append(ctx.StageLogs, log)
			return fmt.Errorf("stage %q failed: %w", stage.Name(), err)
		}

		ctx.StageLogs = append(ctx.StageLogs, log)
	}

	return nil
}
