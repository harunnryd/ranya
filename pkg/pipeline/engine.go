package pipeline

import (
	"context"
	"log/slog"

	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/metrics"
)

type FrameProcessor interface {
	Process(frames.Frame) ([]frames.Frame, error)
	Name() string
}

type BackpressureMode int

const (
	BackpressureDrop BackpressureMode = iota
	BackpressureWait
)

type Config struct {
	Async         bool
	StageBuffer   int
	HighCapacity  int
	LowCapacity   int
	FairnessRatio int
	Backpressure  BackpressureMode
}

type PipelineConfig struct {
	Config     Config
	Processors []FrameProcessor
}

type EngineConfig struct {
	SampleRate      int `mapstructure:"samplerate"`
	STTReplayChunks int `mapstructure:"stt_replay_chunks"`
}

func LogConfiguration(cfg EngineConfig) {
	slog.Info("engine_config",
		"sample_rate", cfg.SampleRate,
		"stt_replay_chunks", cfg.STTReplayChunks,
	)
}

type Orchestrator interface {
	Start() error
	Stop() error
	In() chan frames.Frame
	Out() chan frames.Frame
	AddProcessor(p FrameProcessor) error
	SetContext(ctx context.Context)
	SetSink(sink func(frames.Frame))
	SetObserver(obs metrics.Observer)
}
