package pipeline

type VoiceAgentBuilder struct {
	pre  []FrameProcessor
	core []FrameProcessor
	post []FrameProcessor
}

func NewVoiceAgentBuilder() *VoiceAgentBuilder {
	return &VoiceAgentBuilder{}
}

func (b *VoiceAgentBuilder) WithProcessor(p FrameProcessor) *VoiceAgentBuilder {
	b.core = append(b.core, p)
	return b
}

func (b *VoiceAgentBuilder) WithProcessorList(list []FrameProcessor) *VoiceAgentBuilder {
	for _, p := range list {
		if p != nil {
			b.core = append(b.core, p)
		}
	}
	return b
}

func (b *VoiceAgentBuilder) WithSTT(p FrameProcessor) *VoiceAgentBuilder {
	return b.WithProcessor(p)
}

func (b *VoiceAgentBuilder) WithLLM(p FrameProcessor) *VoiceAgentBuilder {
	return b.WithProcessor(p)
}

func (b *VoiceAgentBuilder) WithTTS(p FrameProcessor) *VoiceAgentBuilder {
	return b.WithProcessor(p)
}

func (b *VoiceAgentBuilder) WithTurnManager(p FrameProcessor) *VoiceAgentBuilder {
	return b.WithProcessor(p)
}

func (b *VoiceAgentBuilder) WithContext(p FrameProcessor) *VoiceAgentBuilder {
	return b.WithProcessor(p)
}

func (b *VoiceAgentBuilder) WithRouter(p FrameProcessor) *VoiceAgentBuilder {
	return b.WithProcessor(p)
}

func (b *VoiceAgentBuilder) WithAcoustic(p FrameProcessor) *VoiceAgentBuilder {
	b.pre = append(b.pre, p)
	return b
}

func (b *VoiceAgentBuilder) WithPLC(p FrameProcessor) *VoiceAgentBuilder {
	b.pre = append(b.pre, p)
	return b
}

func (b *VoiceAgentBuilder) WithSerializer(p FrameProcessor) *VoiceAgentBuilder {
	b.post = append(b.post, p)
	return b
}

func (b *VoiceAgentBuilder) WithFiller(p FrameProcessor) *VoiceAgentBuilder {
	return b.WithProcessor(p)
}

func (b *VoiceAgentBuilder) Build(cfg Config) Orchestrator {
	return NewWithPipelineConfig(PipelineConfig{
		Config:     cfg,
		Processors: append(append(b.pre, b.core...), b.post...),
	})
}
