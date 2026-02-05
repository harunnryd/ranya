package processors

import (
	"strings"

	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/pipeline"
)

type TextNormalizerConfig struct {
	Replacements map[string]string
	Source       string
}

// TextNormalizer performs simple phrase replacements to normalize domain terms.
type TextNormalizer struct {
	replacements map[string]string
	source       string
}

func NewTextNormalizer(cfg TextNormalizerConfig) *TextNormalizer {
	if cfg.Source == "" {
		cfg.Source = "stt"
	}
	return &TextNormalizer{
		replacements: cfg.Replacements,
		source:       cfg.Source,
	}
}

func (t *TextNormalizer) Name() string { return "text_normalizer" }

func (t *TextNormalizer) Process(f frames.Frame) ([]frames.Frame, error) {
	if f.Kind() != frames.KindText {
		return []frames.Frame{f}, nil
	}
	tf := f.(frames.TextFrame)
	meta := tf.Meta()
	if t.source != "" && meta[frames.MetaSource] != t.source {
		return []frames.Frame{f}, nil
	}
	if len(t.replacements) == 0 {
		return []frames.Frame{f}, nil
	}
	normalized := strings.ToLower(tf.Text())
	for from, to := range t.replacements {
		if from == "" {
			continue
		}
		normalized = strings.ReplaceAll(normalized, strings.ToLower(from), to)
	}
	if normalized == tf.Text() {
		return []frames.Frame{f}, nil
	}
	meta[frames.MetaNormalized] = "true"
	return []frames.Frame{frames.NewTextFrame(meta[frames.MetaStreamID], tf.PTS(), normalized, meta)}, nil
}

var _ pipeline.FrameProcessor = (*TextNormalizer)(nil)
