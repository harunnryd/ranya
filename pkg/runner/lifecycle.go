package runner

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

type LifecycleRunner struct {
	state    int32
	ctx      context.Context
	cancel   context.CancelFunc
	onceStop sync.Once
	hooks    Hooks
	drainer  Drainer
	stopErr  error
	timeout  time.Duration
}

func NewLifecycleRunner(drainer Drainer, hooks Hooks, timeout time.Duration) *LifecycleRunner {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &LifecycleRunner{
		state:   int32(StateNew),
		ctx:     ctx,
		cancel:  cancel,
		hooks:   hooks,
		drainer: drainer,
		timeout: timeout,
	}
}

func (r *LifecycleRunner) Run(ctx context.Context) error {
	if !r.casState(StateNew, StateStarting) {
		return errors.New("invalid state transition")
	}
	PrintBanner()
	if ctx != nil {
		r.ctx, r.cancel = context.WithCancel(ctx)
	}
	if r.hooks.OnStart != nil {
		r.hooks.OnStart()
	}
	r.setState(StateRunning)
	<-r.ctx.Done()
	return r.stop()
}

func (r *LifecycleRunner) Stop() error {
	r.cancel()
	return r.stop()
}

func (r *LifecycleRunner) State() State {
	return State(atomic.LoadInt32(&r.state))
}

func (r *LifecycleRunner) stop() error {
	r.onceStop.Do(func() {
		r.setState(StateDraining)
		if r.drainer != nil {
			done := make(chan struct{})
			go func() {
				_ = r.drainer.Drain()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(r.timeout):
				r.stopErr = errors.New("drain timeout")
			}
		}
		if r.hooks.OnStop != nil {
			r.hooks.OnStop()
		}
		r.setState(StateStopped)
	})
	return r.stopErr
}

func (r *LifecycleRunner) casState(from, to State) bool {
	return atomic.CompareAndSwapInt32(&r.state, int32(from), int32(to))
}

func (r *LifecycleRunner) setState(s State) {
	atomic.StoreInt32(&r.state, int32(s))
}
