package observers

import (
	"context"
	"log/slog"

	"github.com/harunnryd/ranya/pkg/metrics"
)

type LoggerObserver struct {
	log *slog.Logger
}

func NewLoggerObserver(log *slog.Logger) *LoggerObserver {
	if log == nil {
		log = slog.Default()
	}
	return &LoggerObserver{log: log}
}

func (o *LoggerObserver) RecordEvent(ev metrics.MetricsEvent) {
	attrs := []slog.Attr{
		slog.String("name", ev.Name),
		slog.Time("time", ev.Time),
		slog.Float64("value", ev.Value),
	}
	for k, v := range ev.Tags {
		attrs = append(attrs, slog.String(k, v))
	}
	for k, v := range ev.Fields {
		attrs = append(attrs, slog.Any(k, v))
	}
	o.log.LogAttrs(context.TODO(), slog.LevelDebug, "metrics", attrs...)
}

type MultiObserver struct {
	list []metrics.Observer
}

func NewMultiObserver(list ...metrics.Observer) *MultiObserver {
	return &MultiObserver{list: list}
}

func (m *MultiObserver) RecordEvent(ev metrics.MetricsEvent) {
	for _, obs := range m.list {
		if obs != nil {
			obs.RecordEvent(ev)
		}
	}
}
