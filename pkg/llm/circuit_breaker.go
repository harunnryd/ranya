package llm

import (
	"context"
	"sync"
	"time"

	"github.com/harunnryd/ranya/pkg/metrics"
	"github.com/harunnryd/ranya/pkg/resilience"
)

// CircuitBreakerAdapter wraps an LLMAdapter with rate-limit circuit breaking.
type CircuitBreakerAdapter struct {
	inner   LLMAdapter
	breaker *resilience.CircuitBreaker
	obs     metrics.Observer
	open    bool
	mu      sync.Mutex
}

func NewCircuitBreakerAdapter(inner LLMAdapter, breaker *resilience.CircuitBreaker) *CircuitBreakerAdapter {
	if breaker == nil {
		breaker = resilience.NewCircuitBreaker(3, 30*time.Second)
	}
	return &CircuitBreakerAdapter{inner: inner, breaker: breaker}
}

func (a *CircuitBreakerAdapter) Name() string { return a.inner.Name() }

// SetObserver allows metrics emission for breaker events.
func (a *CircuitBreakerAdapter) SetObserver(obs metrics.Observer) { a.obs = obs }

func (a *CircuitBreakerAdapter) Generate(ctx context.Context, input Context) (Response, error) {
	if !a.breaker.Allow() {
		a.setOpen(true)
		a.record(metrics.EventBreakerDenied)
		return Response{}, resilience.RateLimitError{Provider: a.Name(), Message: "degraded"}
	}
	a.setOpen(false)
	resp, err := a.inner.Generate(ctx, input)
	if err != nil {
		if resilience.IsRateLimit(err) {
			a.record(metrics.EventRateLimit)
		}
		a.breaker.OnError(err)
		return Response{}, err
	}
	a.breaker.OnSuccess()
	return resp, nil
}

func (a *CircuitBreakerAdapter) Stream(ctx context.Context, input Context) (<-chan string, error) {
	if !a.breaker.Allow() {
		a.setOpen(true)
		a.record(metrics.EventBreakerDenied)
		return nil, resilience.RateLimitError{Provider: a.Name(), Message: "degraded"}
	}
	a.setOpen(false)
	ch, err := a.inner.Stream(ctx, input)
	if err != nil {
		if resilience.IsRateLimit(err) {
			a.record(metrics.EventRateLimit)
		}
		a.breaker.OnError(err)
		return nil, err
	}
	a.breaker.OnSuccess()
	return ch, nil
}

func (a *CircuitBreakerAdapter) MapTools(tools []Tool) (any, error) {
	return a.inner.MapTools(tools)
}

func (a *CircuitBreakerAdapter) ToProviderFormat(ctx Context) (any, error) {
	return a.inner.ToProviderFormat(ctx)
}

func (a *CircuitBreakerAdapter) FromProviderFormat(raw any) (Response, error) {
	return a.inner.FromProviderFormat(raw)
}

func (a *CircuitBreakerAdapter) record(name string) {
	if a.obs == nil {
		return
	}
	a.obs.RecordEvent(metrics.MetricsEvent{
		Name: name,
		Time: time.Now(),
		Tags: map[string]string{
			"provider":  a.inner.Name(),
			"component": "llm",
		},
	})
}

func (a *CircuitBreakerAdapter) setOpen(open bool) {
	a.mu.Lock()
	changed := a.open != open
	a.open = open
	a.mu.Unlock()
	if !changed {
		return
	}
	if open {
		a.record(metrics.EventBreakerOpen)
		return
	}
	a.record(metrics.EventBreakerClose)
}
