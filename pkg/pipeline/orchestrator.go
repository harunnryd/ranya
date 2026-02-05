package pipeline

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/metrics"
	"github.com/harunnryd/ranya/pkg/priority"
)

type orchestrator struct {
	in      chan frames.Frame
	out     chan frames.Frame
	pq      *priority.PriorityQueue
	procs   []FrameProcessor
	cfg     Config
	ctx     context.Context
	cancel  context.CancelFunc
	stageCh []chan frames.Frame
	sink    func(frames.Frame)
	obs     metrics.Observer
}

func New(cfg Config) Orchestrator {
	o := &orchestrator{
		in:  make(chan frames.Frame, cfg.HighCapacity+cfg.LowCapacity),
		out: make(chan frames.Frame, cfg.HighCapacity+cfg.LowCapacity),
		cfg: cfg,
	}
	o.pq = priority.New(cfg.HighCapacity, cfg.LowCapacity, cfg.FairnessRatio)
	o.ctx, o.cancel = context.WithCancel(context.Background())
	return o
}

func NewWithPipelineConfig(pc PipelineConfig) Orchestrator {
	orch := New(pc.Config)
	logPipeline(pc.Processors)
	for _, p := range pc.Processors {
		_ = orch.AddProcessor(p)
	}
	return orch
}

func (o *orchestrator) SetContext(ctx context.Context) {
	if ctx == nil {
		return
	}
	o.ctx, o.cancel = context.WithCancel(ctx)
}

func (o *orchestrator) In() chan frames.Frame            { return o.in }
func (o *orchestrator) Out() chan frames.Frame           { return o.out }
func (o *orchestrator) SetSink(sink func(frames.Frame))  { o.sink = sink }
func (o *orchestrator) SetObserver(obs metrics.Observer) { o.obs = obs }

func (o *orchestrator) AddProcessor(p FrameProcessor) error {
	o.procs = append(o.procs, p)
	return nil
}

func (o *orchestrator) Start() error {
	if o.cfg.Async {
		return o.startAsync()
	}
	return o.startSync()
}

func (o *orchestrator) Stop() error {
	o.cancel()
	// allow goroutines to exit and drain
	time.Sleep(5 * time.Millisecond)
	close(o.out)
	return nil
}

func (o *orchestrator) startSync() error {
	go func() {
		for {
			select {
			case <-o.ctx.Done():
				return
			case f := <-o.in:
				if f.Kind() == frames.KindControl {
					if !o.pq.TryPushHigh(f) {
						frames.ReleaseAudioFrame(f)
						o.recordDrop(f)
					}
				} else {
					if !o.pq.TryPushLow(f) {
						frames.ReleaseAudioFrame(f)
						o.recordDrop(f)
					}
				}
				o.recordIn(f)
			}
		}
	}()
	go func() {
		for {
			select {
			case <-o.ctx.Done():
				return
			default:
				fAny, _ := o.pq.Pop()
				f := fAny.(frames.Frame)
				if shouldDropForLag(f, 500*time.Millisecond) {
					frames.ReleaseAudioFrame(f)
					o.recordDrop(f)
					continue
				}
				out := []frames.Frame{f}
				for _, p := range o.procs {
					var next []frames.Frame
					for _, cur := range out {
						start := time.Now()
						r, err := p.Process(cur)
						if err != nil || r == nil {
							frames.ReleaseAudioFrame(cur)
							continue
						}
						o.recordStage(p.Name(), cur, start)
						next = append(next, r...)
					}
					out = next
					if out == nil {
						break
					}
				}
				for _, e := range out {
					o.recordOut(e)
					o.emit(e)
				}
			}
		}
	}()
	return nil
}

func (o *orchestrator) startAsync() error {
	o.stageCh = make([]chan frames.Frame, len(o.procs)+1)
	for i := range o.stageCh {
		o.stageCh[i] = make(chan frames.Frame, o.cfg.StageBuffer)
	}
	for i, p := range o.procs {
		inCh, outCh := o.stageCh[i], o.stageCh[i+1]
		go func(proc FrameProcessor, in, out chan frames.Frame) {
			for {
				select {
				case <-o.ctx.Done():
					return
				case f := <-in:
					start := time.Now()
					r, err := proc.Process(f)
					if err != nil || r == nil {
						frames.ReleaseAudioFrame(f)
						continue
					}
					o.recordStage(proc.Name(), f, start)
					for _, e := range r {
						o.push(out, e)
					}
				}
			}
		}(p, inCh, outCh)
	}
	// feeder from in -> high/low pq
	go func() {
		for {
			select {
			case <-o.ctx.Done():
				return
			case f := <-o.in:
				if f.Kind() == frames.KindControl {
					if !o.pq.TryPushHigh(f) {
						frames.ReleaseAudioFrame(f)
						o.recordDrop(f)
					}
				} else {
					if !o.pq.TryPushLow(f) {
						frames.ReleaseAudioFrame(f)
						o.recordDrop(f)
					}
				}
				o.recordIn(f)
			}
		}
	}()
	// pop from pq to stage0 honoring fairness
	go func() {
		for {
			select {
			case <-o.ctx.Done():
				return
			default:
				fAny, _ := o.pq.Pop()
				f := fAny.(frames.Frame)
				if shouldDropForLag(f, 500*time.Millisecond) {
					frames.ReleaseAudioFrame(f)
					o.recordDrop(f)
					continue
				}
				o.push(o.stageCh[0], f)
			}
		}
	}()
	// final stage to out
	go func() {
		final := o.stageCh[len(o.stageCh)-1]
		for {
			select {
			case <-o.ctx.Done():
				return
			case e := <-final:
				o.recordOut(e)
				o.emit(e)
			}
		}
	}()
	return nil
}

func (o *orchestrator) emit(f frames.Frame) {
	if o.sink != nil {
		o.sink(f)
		frames.ReleaseAudioFrame(f)
		return
	}
	o.push(o.out, f)
}

func (o *orchestrator) push(ch chan frames.Frame, f frames.Frame) {
	if shouldDropForLag(f, 500*time.Millisecond) {
		frames.ReleaseAudioFrame(f)
		o.recordDrop(f)
		return
	}
	switch o.cfg.Backpressure {
	case BackpressureWait:
		select {
		case <-o.ctx.Done():
			frames.ReleaseAudioFrame(f)
			return
		case ch <- f:
		}
	default:
		select {
		case ch <- f:
		default:
			frames.ReleaseAudioFrame(f)
			o.recordDrop(f)
		}
	}
}

func (o *orchestrator) recordStage(name string, f frames.Frame, start time.Time) {
	if o.obs == nil {
		return
	}
	o.obs.RecordEvent(metrics.MetricsEvent{
		Name:  "stage_latency_us",
		Time:  time.Now(),
		Value: float64(time.Since(start).Microseconds()),
		Tags: map[string]string{
			"processor":         name,
			frames.MetaStreamID: streamIDFromFrame(f),
			frames.MetaTraceID:  traceIDFromFrame(f),
			frames.MetaAgent:    agentFromFrame(f),
		},
	})
}

func (o *orchestrator) recordIn(f frames.Frame) {
	if o.obs == nil {
		return
	}
	tags := map[string]string{
		frames.MetaStreamID: streamIDFromFrame(f),
		frames.MetaTraceID:  traceIDFromFrame(f),
		frames.MetaAgent:    agentFromFrame(f),
		"kind":              kindFromFrame(f),
	}
	addFrameDetailTags(tags, f)
	o.obs.RecordEvent(metrics.MetricsEvent{
		Name: "frame_in",
		Time: time.Now(),
		Tags: tags,
	})
}

func (o *orchestrator) recordOut(f frames.Frame) {
	if o.obs == nil {
		return
	}
	tags := map[string]string{
		frames.MetaStreamID: streamIDFromFrame(f),
		frames.MetaTraceID:  traceIDFromFrame(f),
		frames.MetaAgent:    agentFromFrame(f),
		"kind":              kindFromFrame(f),
	}
	addFrameDetailTags(tags, f)
	o.obs.RecordEvent(metrics.MetricsEvent{
		Name: "frame_out",
		Time: time.Now(),
		Tags: tags,
	})
}

func (o *orchestrator) recordDrop(f frames.Frame) {
	if o.obs == nil {
		return
	}
	o.obs.RecordEvent(metrics.MetricsEvent{
		Name: "frame_drop",
		Time: time.Now(),
		Tags: map[string]string{
			frames.MetaStreamID: streamIDFromFrame(f),
			frames.MetaTraceID:  traceIDFromFrame(f),
			frames.MetaAgent:    agentFromFrame(f),
			"kind":              kindFromFrame(f),
		},
	})
}

func streamIDFromFrame(f frames.Frame) string {
	if f == nil {
		return ""
	}
	m := f.Meta()
	if m == nil {
		return ""
	}
	return m[frames.MetaStreamID]
}

func traceIDFromFrame(f frames.Frame) string {
	if f == nil {
		return ""
	}
	m := f.Meta()
	if m == nil {
		return ""
	}
	return m[frames.MetaTraceID]
}

func logPipeline(procs []FrameProcessor) {
	if len(procs) == 0 {
		return
	}
	names := make([]string, 0, len(procs))
	for _, p := range procs {
		names = append(names, p.Name())
	}
	slog.Info("pipeline", "order", strings.Join(names, " -> "))
}

func agentFromFrame(f frames.Frame) string {
	if f == nil {
		return ""
	}
	m := f.Meta()
	if m == nil {
		return ""
	}
	return m[frames.MetaAgent]
}

func kindFromFrame(f frames.Frame) string {
	if f == nil {
		return ""
	}
	return string(f.Kind())
}

func addFrameDetailTags(tags map[string]string, f frames.Frame) {
	if tags == nil || f == nil {
		return
	}
	meta := f.Meta()
	if meta != nil {
		if source := meta[frames.MetaSource]; source != "" {
			tags["source"] = source
		}
	}
	switch f.Kind() {
	case frames.KindControl:
		cf := f.(frames.ControlFrame)
		tags["control_code"] = string(cf.Code())
		if meta != nil {
			if reason := meta[frames.MetaReason]; reason != "" {
				tags["control_reason"] = reason
			}
		}
	case frames.KindSystem:
		sf := f.(frames.SystemFrame)
		if name := sf.Name(); name != "" {
			tags["system_name"] = name
		}
	}
}

func shouldDropForLag(f frames.Frame, maxLag time.Duration) bool {
	if f == nil || f.Kind() != frames.KindAudio {
		return false
	}
	pts := f.PTS()
	if pts <= 0 {
		return false
	}
	if pts < 1_000_000_000_000 {
		return false
	}
	lag := time.Since(time.Unix(0, pts))
	return lag > maxLag
}
