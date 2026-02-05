package metrics

import (
	"sync"
	"sync/atomic"
)

type AsyncObserver struct {
	inner   Observer
	ch      chan MetricsEvent
	dropped int64
	closed  atomic.Bool
	once    sync.Once
}

func NewAsyncObserver(inner Observer, buffer int) *AsyncObserver {
	if buffer <= 0 {
		buffer = 256
	}
	a := &AsyncObserver{
		inner: inner,
		ch:    make(chan MetricsEvent, buffer),
	}
	go a.loop()
	return a
}

func (a *AsyncObserver) RecordEvent(ev MetricsEvent) {
	if a == nil || a.closed.Load() {
		return
	}
	select {
	case a.ch <- ev:
	default:
		atomic.AddInt64(&a.dropped, 1)
	}
}

func (a *AsyncObserver) Dropped() int64 {
	return atomic.LoadInt64(&a.dropped)
}

func (a *AsyncObserver) Close() {
	if a == nil {
		return
	}
	a.once.Do(func() {
		a.closed.Store(true)
		close(a.ch)
	})
}

func (a *AsyncObserver) loop() {
	for ev := range a.ch {
		a.inner.RecordEvent(ev)
	}
}
