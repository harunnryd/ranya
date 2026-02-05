package metrics

import (
	"context"
	"io"
	"log/slog"
)

type JSONLObserver struct {
	logger *slog.Logger
}

func NewJSONLObserver(w io.Writer) *JSONLObserver {
	if w == nil {
		return &JSONLObserver{logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	}
	return &JSONLObserver{logger: slog.New(slog.NewJSONHandler(w, nil))}
}

func (o *JSONLObserver) RecordEvent(ev MetricsEvent) {
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
	o.logger.LogAttrs(context.TODO(), slog.LevelInfo, "metrics", attrs...)
}
