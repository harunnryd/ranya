package ranya

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/llm"
	"github.com/harunnryd/ranya/pkg/pipeline"
)

type ToolDispatcher struct {
	registry llm.ToolRegistry
	in       chan frames.Frame
	tasks    chan map[string]string
	opts     ToolDispatcherOptions

	mu          sync.Mutex
	streamLocks map[string]*sync.Mutex
}

type ToolDispatcherOptions struct {
	Concurrency       int
	Timeout           time.Duration
	Retries           int
	RetryBackoff      time.Duration
	SerializeByStream bool
}

var ErrToolTimeout = errors.New("tool timeout")

func NewToolDispatcher(registry llm.ToolRegistry, in chan frames.Frame) *ToolDispatcher {
	return NewToolDispatcherWithOptions(registry, in, ToolDispatcherOptions{})
}

func NewToolDispatcherWithOptions(registry llm.ToolRegistry, in chan frames.Frame, opts ToolDispatcherOptions) *ToolDispatcher {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 4
	}
	if opts.RetryBackoff <= 0 {
		opts.RetryBackoff = 150 * time.Millisecond
	}
	d := &ToolDispatcher{
		registry:    registry,
		in:          in,
		tasks:       make(chan map[string]string, 64),
		opts:        opts,
		streamLocks: make(map[string]*sync.Mutex),
	}
	for i := 0; i < opts.Concurrency; i++ {
		go d.worker()
	}
	return d
}

func (d *ToolDispatcher) Name() string { return "tool_dispatcher" }

func (d *ToolDispatcher) SetInput(in chan frames.Frame) { d.in = in }

func (d *ToolDispatcher) Process(f frames.Frame) ([]frames.Frame, error) {
	if f.Kind() != frames.KindControl {
		return []frames.Frame{f}, nil
	}
	cf := f.(frames.ControlFrame)
	if cf.Code() != frames.ControlToolCall {
		return []frames.Frame{f}, nil
	}
	meta := cf.Meta()
	if d.registry == nil || d.in == nil {
		return []frames.Frame{f}, nil
	}
	select {
	case d.tasks <- meta:
	default:
		slog.Warn("tool_dispatcher_queue_full", "tool_name", meta[frames.MetaToolName])
	}
	return []frames.Frame{f}, nil
}

func (d *ToolDispatcher) worker() {
	for meta := range d.tasks {
		d.exec(meta)
	}
}

func (d *ToolDispatcher) exec(meta map[string]string) {
	callID := meta[frames.MetaToolCallID]
	name := meta[frames.MetaToolName]
	argsRaw := meta[frames.MetaToolArgs]
	if callID == "" || name == "" {
		return
	}
	args := map[string]any{}
	_ = json.Unmarshal([]byte(argsRaw), &args)
	if _, ok := args[frames.MetaIdempotency]; !ok {
		args[frames.MetaIdempotency] = d.idempotencyKey(meta)
	}
	var result string
	var err error
	status := "ok"
	if d.opts.SerializeByStream {
		lock := d.streamLock(meta[frames.MetaStreamID])
		lock.Lock()
		result, err = d.callWithRetry(name, args)
		lock.Unlock()
	} else {
		result, err = d.callWithRetry(name, args)
	}
	if err != nil {
		status = "error"
		if errors.Is(err, ErrToolTimeout) {
			status = "timeout"
		}
		if result == "" {
			result = "error"
		}
	}
	outMeta := map[string]string{
		frames.MetaStreamID:   meta[frames.MetaStreamID],
		frames.MetaToolCallID: callID,
		frames.MetaToolName:   name,
		frames.MetaToolResult: result,
		frames.MetaToolStatus: status,
	}
	if err != nil {
		outMeta[frames.MetaToolError] = err.Error()
	}
	if callSID := meta[frames.MetaCallSID]; callSID != "" {
		outMeta[frames.MetaCallSID] = callSID
	}
	if traceID := meta[frames.MetaTraceID]; traceID != "" {
		outMeta[frames.MetaTraceID] = traceID
	}
	if lang := meta[frames.MetaLanguage]; lang != "" {
		outMeta[frames.MetaLanguage] = lang
	}
	sf := frames.NewSystemFrame(meta[frames.MetaStreamID], time.Now().UnixNano(), "tool_result", outMeta)
	select {
	case d.in <- sf:
	default:
	}
}

func (d *ToolDispatcher) callWithRetry(name string, args map[string]any) (string, error) {
	attempts := d.opts.Retries + 1
	var lastErr error
	for i := 0; i < attempts; i++ {
		result, err := d.callWithTimeout(name, args)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if i < attempts-1 {
			time.Sleep(d.opts.RetryBackoff * time.Duration(i+1))
		}
	}
	if lastErr == nil {
		lastErr = errors.New("tool error")
	}
	return "", lastErr
}

func (d *ToolDispatcher) callWithTimeout(name string, args map[string]any) (string, error) {
	if d.registry == nil {
		return "", errors.New("missing registry")
	}
	if d.opts.Timeout <= 0 {
		return d.registry.HandleTool(name, args)
	}
	type result struct {
		text string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		res, err := d.registry.HandleTool(name, args)
		ch <- result{text: res, err: err}
	}()
	select {
	case out := <-ch:
		return out.text, out.err
	case <-time.After(d.opts.Timeout):
		return "", ErrToolTimeout
	}
}

func (d *ToolDispatcher) streamLock(streamID string) *sync.Mutex {
	if streamID == "" {
		return &sync.Mutex{}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	lock, ok := d.streamLocks[streamID]
	if !ok {
		lock = &sync.Mutex{}
		d.streamLocks[streamID] = lock
	}
	return lock
}

func (d *ToolDispatcher) idempotencyKey(meta map[string]string) string {
	streamID := meta[frames.MetaStreamID]
	callID := meta[frames.MetaToolCallID]
	if streamID == "" && callID == "" {
		return fmt.Sprintf("tool-%d", time.Now().UnixNano())
	}
	return streamID + ":" + callID
}

var _ pipeline.FrameProcessor = (*ToolDispatcher)(nil)
