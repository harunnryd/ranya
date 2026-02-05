package processors

import (
	"strings"
	"sync"

	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/pipeline"
)

type RecoveryConfig struct {
	MaxAttempts      int
	PromptText       string
	PromptByLanguage map[string]string
	Phrases          []string
}

// RecoveryProcessor injects a clarification prompt when the agent signals confusion/fallback.
type RecoveryProcessor struct {
	cfg    RecoveryConfig
	mu     sync.Mutex
	counts map[string]int
}

func NewRecoveryProcessor(cfg RecoveryConfig) *RecoveryProcessor {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 2
	}
	if cfg.PromptText == "" {
		cfg.PromptText = "Maaf, saya belum menangkapnya. Bisa jelaskan ulang secara singkat?"
	}
	if len(cfg.Phrases) == 0 {
		cfg.Phrases = []string{
			"maaf saya tidak mengerti",
			"saya belum paham",
			"saya tidak paham",
			"could you repeat",
			"i didn't understand",
		}
	}
	return &RecoveryProcessor{
		cfg:    cfg,
		counts: make(map[string]int),
	}
}

func (r *RecoveryProcessor) Name() string { return "recovery_processor" }

func (r *RecoveryProcessor) Process(f frames.Frame) ([]frames.Frame, error) {
	streamID := f.Meta()[frames.MetaStreamID]
	if streamID == "" {
		return []frames.Frame{f}, nil
	}
	switch f.Kind() {
	case frames.KindSystem:
		sf := f.(frames.SystemFrame)
		if sf.Name() == "call_end" {
			r.reset(streamID)
		}
	case frames.KindControl:
		cf := f.(frames.ControlFrame)
		if cf.Code() == frames.ControlFallback {
			if r.bump(streamID) {
				meta := cf.Meta()
				meta[frames.MetaSource] = "system"
				meta[frames.MetaRecoveryReason] = "fallback"
				prompt := frames.NewTextFrame(streamID, cf.PTS(), r.promptFor(meta), meta)
				return []frames.Frame{prompt, f}, nil
			}
		}
	case frames.KindText:
		tf := f.(frames.TextFrame)
		meta := tf.Meta()
		if meta[frames.MetaSource] == "llm" {
			if r.isConfusion(tf.Text()) {
				if r.bump(streamID) {
					meta[frames.MetaSource] = "system"
					meta[frames.MetaRecoveryReason] = "confusion"
					prompt := frames.NewTextFrame(streamID, tf.PTS(), r.promptFor(meta), meta)
					return []frames.Frame{prompt}, nil
				}
			} else {
				r.reset(streamID)
			}
		}
	}
	return []frames.Frame{f}, nil
}

func (r *RecoveryProcessor) promptFor(meta map[string]string) string {
	if r.cfg.PromptByLanguage != nil {
		if lang := strings.ToLower(strings.TrimSpace(meta[frames.MetaLanguage])); lang != "" {
			if prompt := r.cfg.PromptByLanguage[lang]; prompt != "" {
				return prompt
			}
		}
	}
	return r.cfg.PromptText
}

func (r *RecoveryProcessor) isConfusion(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	for _, p := range r.cfg.Phrases {
		if strings.Contains(t, p) {
			return true
		}
	}
	return false
}

func (r *RecoveryProcessor) bump(streamID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counts[streamID]++
	return r.counts[streamID] <= r.cfg.MaxAttempts
}

func (r *RecoveryProcessor) reset(streamID string) {
	r.mu.Lock()
	delete(r.counts, streamID)
	r.mu.Unlock()
}

var _ pipeline.FrameProcessor = (*RecoveryProcessor)(nil)
