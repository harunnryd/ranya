package metrics

import "time"

type MetricsEvent struct {
	Name   string
	Time   time.Time
	Value  float64
	Tags   map[string]string
	Fields map[string]any
}

type Observer interface {
	RecordEvent(ev MetricsEvent)
}

type Flusher interface {
	Flush() error
}

type NoopObserver struct{}

func (NoopObserver) RecordEvent(MetricsEvent) {}
