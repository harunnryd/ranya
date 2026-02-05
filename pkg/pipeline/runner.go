package pipeline

import (
	"context"
	"time"

	"github.com/harunnryd/ranya/pkg/runner"
)

type Runner struct {
	orch Orchestrator
	lc   *runner.LifecycleRunner
}

func NewRunner(orch Orchestrator, hooks runner.Hooks) *Runner {
	drainer := DrainerFunc(func() error { return orch.Stop() })
	lc := runner.NewLifecycleRunner(drainer, hooks, 0)
	return &Runner{orch: orch, lc: lc}
}

func (r *Runner) Run(ctx context.Context) error { return r.lc.Run(ctx) }
func (r *Runner) Stop() error                   { return r.lc.Stop() }
func (r *Runner) Restart(ctx context.Context) error {
	_ = r.lc.Stop()
	return r.lc.Run(ctx)
}

type DrainerFunc func() error

func (r DrainerFunc) Drain() error { return r() }

func NewDrainRunner(drainer runner.Drainer, hooks runner.Hooks, timeout time.Duration) *Runner {
	lc := runner.NewLifecycleRunner(drainer, hooks, timeout)
	return &Runner{lc: lc}
}
