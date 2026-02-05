package processors

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/harunnryd/ranya/pkg/errorsx"
	"github.com/harunnryd/ranya/pkg/frames"
	"github.com/harunnryd/ranya/pkg/llm"
	"github.com/harunnryd/ranya/pkg/metrics"
	"github.com/harunnryd/ranya/pkg/pipeline"
	"github.com/harunnryd/ranya/pkg/redact"
	"github.com/harunnryd/ranya/pkg/resilience"
)

type AgentConfig struct {
	Name    string
	System  string
	Adapter llm.LLMAdapter
}

type LLMProcessor struct {
	adapter            llm.LLMAdapter
	system             string
	tools              []llm.Tool
	toolIndex          map[string]llm.Tool
	messagesByScope    map[string][]map[string]any
	mu                 sync.Mutex
	ctx                context.Context
	obs                metrics.Observer
	agents             map[string]AgentConfig
	defaultAgent       string
	activeAgent        map[string]string
	lastInjected       map[string]string
	pendingTools       map[string]llm.ToolCall
	pendingConfirms    map[string]pendingToolConfirm
	lastLanguage       map[string]string
	lastLanguageByCall map[string]string
	lastCallSID        map[string]string
	maxHistory         int
	maxTokens          int
	confirmMode        string
	confirmLLMFallback bool
	confirmTimeout     time.Duration
}

const defaultLLMScope = "default"

type pendingToolConfirm struct {
	call    llm.ToolCall
	meta    map[string]string
	prompt  string
	created time.Time
}

func NewLLMProcessor(adapter llm.LLMAdapter, system string, tools []llm.Tool) *LLMProcessor {
	return &LLMProcessor{
		adapter:            adapter,
		system:             system,
		tools:              tools,
		toolIndex:          indexTools(tools),
		messagesByScope:    make(map[string][]map[string]any),
		ctx:                context.Background(),
		activeAgent:        make(map[string]string),
		lastInjected:       make(map[string]string),
		pendingTools:       make(map[string]llm.ToolCall),
		pendingConfirms:    make(map[string]pendingToolConfirm),
		lastLanguage:       make(map[string]string),
		lastLanguageByCall: make(map[string]string),
		lastCallSID:        make(map[string]string),
	}
}

func (p *LLMProcessor) Name() string { return "llm" }

func (p *LLMProcessor) SetObserver(obs metrics.Observer) {
	p.obs = obs
	if setter, ok := p.adapter.(interface{ SetObserver(metrics.Observer) }); ok {
		setter.SetObserver(obs)
	}
}

func (p *LLMProcessor) SetContext(ctx context.Context) {
	if ctx != nil {
		p.ctx = ctx
	}
}

func (p *LLMProcessor) SetAgents(agents map[string]AgentConfig, defaultAgent string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.agents = agents
	p.defaultAgent = defaultAgent
}

func (p *LLMProcessor) SetTools(tools []llm.Tool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tools = tools
	p.toolIndex = indexTools(tools)
}

func (p *LLMProcessor) SetMemoryLimits(maxHistory, maxTokens int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if maxHistory < 0 {
		maxHistory = 0
	}
	if maxTokens < 0 {
		maxTokens = 0
	}
	p.maxHistory = maxHistory
	p.maxTokens = maxTokens
}

func (p *LLMProcessor) SetConfirmationOptions(mode string, llmFallback bool, timeout time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.confirmMode = normalizeConfirmMode(mode)
	p.confirmLLMFallback = llmFallback
	p.confirmTimeout = timeout
}

func (p *LLMProcessor) Process(f frames.Frame) ([]frames.Frame, error) {
	if f.Kind() == frames.KindSystem {
		sf := f.(frames.SystemFrame)
		meta := sf.Meta()
		scope := p.scopeKey(meta, meta[frames.MetaStreamID])
		if msg := meta[frames.MetaSystemMessage]; msg != "" {
			p.appendSystem(scope, msg)
		}
		p.setLanguageFromMeta(meta)
		if sf.Name() == "call_end" {
			p.clearCall(meta)
		}
		if sf.Name() == "tool_result" {
			out, err := p.applyToolResult(sf)
			if err != nil {
				return []frames.Frame{f}, nil
			}
			return append(out, f), nil
		}
		if greet := meta[frames.MetaGreetingText]; greet != "" {
			streamID := meta[frames.MetaStreamID]
			meta[frames.MetaSource] = "llm"
			p.applyLanguageMeta(meta, streamID)
			p.appendAssistant(scope, greet)
			return []frames.Frame{frames.NewTextFrame(streamID, sf.PTS(), greet, meta)}, nil
		}
		return []frames.Frame{f}, nil
	}
	if f.Kind() != frames.KindText {
		return []frames.Frame{f}, nil
	}
	tf := f.(frames.TextFrame)
	meta := tf.Meta()
	streamID := meta[frames.MetaStreamID]
	p.setLanguageFromMeta(meta)
	p.setCallSIDFromMeta(meta)
	scope := p.scopeKey(meta, streamID)

	if out, ok := p.handlePendingConfirmation(streamID, tf); ok {
		return out, nil
	}

	safe := redact.Text(tf.Text())
	slog.Info("llm_input_received", "stream_id", streamID, "text", safe)

	agent := p.resolveAgent(meta, streamID)
	ctx := p.contextWithUserParts(tf.Text(), meta[frames.MetaImageURL], meta[frames.MetaImageBase64], meta[frames.MetaImageMIME], agent, scope)
	control := frames.NewControlFrame(streamID, time.Now().UnixNano(), frames.ControlStartInterruption, meta)
	var out []frames.Frame
	out = append(out, control)
	adapter := p.adapterFor(agent)

	slog.Info("llm_generating", "stream_id", streamID, "agent", agent)

	resp, err := adapter.Generate(p.ctx, ctx)
	if err != nil {
		reason := errorsx.ReasonLLMGenerate
		if resilience.IsRateLimit(err) {
			reason = errorsx.ReasonLLMRateLimit
		}
		err = errorsx.Wrap(err, reason)
		slog.Error("llm_generate_error", "stream_id", streamID, "reason_code", string(errorsx.Reason(err)), "error", err)
		p.popLastMessage(scope) // Rollback history to avoid stuck state
		fallback := frames.NewControlFrame(streamID, time.Now().UnixNano(), frames.ControlFallback, meta)
		return append(out, fallback), nil
	}
	if resp.HandoffAgent != "" {
		h := frames.NewControlFrame(streamID, time.Now().UnixNano(), frames.ControlHandoff, map[string]string{
			frames.MetaStreamID:     streamID,
			frames.MetaHandoffAgent: resp.HandoffAgent,
			frames.MetaAgent:        agent,
		})
		out = append(out, h)
	}
	if len(resp.ToolCalls) > 0 {
		out = append(out, p.handleToolCallsWithConfirmation(streamID, resp.ToolCalls, meta)...)
		return out, nil
	}
	ch, err := adapter.Stream(p.ctx, ctx)
	if err != nil {
		reason := errorsx.ReasonLLMStream
		if resilience.IsRateLimit(err) {
			reason = errorsx.ReasonLLMRateLimit
		}
		err = errorsx.Wrap(err, reason)
		slog.Error("llm_stream_error", "stream_id", streamID, "reason_code", string(errorsx.Reason(err)), "error", err)
		fallback := frames.NewControlFrame(streamID, time.Now().UnixNano(), frames.ControlFallback, meta)
		return append(out, fallback), nil
	}
	return append(out, p.streamToFrames(tf, ch)...), nil
}

func (p *LLMProcessor) toolsLocked() []llm.Tool {
	return p.tools
}

func (p *LLMProcessor) contextWithUserParts(text, imageURL, imageBase64, imageMIME, agent, scope string) llm.Context {
	if imageURL == "" && imageBase64 == "" {
		return p.contextWithUserWithAgent(text, agent, scope)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ensureAgentLocked(agent, scope)
	msgs := p.ensureMessagesLocked(scope)
	var parts []map[string]any
	if text != "" {
		parts = append(parts, map[string]any{"type": "text", "text": text})
	}
	if imageURL != "" {
		parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL}})
	} else if imageBase64 != "" {
		mime := imageMIME
		if mime == "" {
			mime = "image/png"
		}
		url := "data:" + mime + ";base64," + imageBase64
		parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}})
	}
	msgs = append(msgs, map[string]any{"role": "user", "content_parts": parts})
	msgs = p.pruneMessagesLocked(msgs)
	p.messagesByScope[scopeKeyOrDefault(scope)] = msgs
	return llm.Context{Messages: cloneMessages(msgs), Tools: p.toolsLocked()}
}

func (p *LLMProcessor) contextWithUserWithAgent(text, agent, scope string) llm.Context {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ensureAgentLocked(agent, scope)
	msgs := p.ensureMessagesLocked(scope)
	msgs = append(msgs, map[string]any{"role": "user", "content": text})
	msgs = p.pruneMessagesLocked(msgs)
	p.messagesByScope[scopeKeyOrDefault(scope)] = msgs
	return llm.Context{Messages: cloneMessages(msgs), Tools: p.toolsLocked()}
}

func (p *LLMProcessor) ensureAgentLocked(agent, scope string) {
	if agent == "" {
		return
	}
	scope = scopeKeyOrDefault(scope)
	if p.lastInjected[scope] == agent {
		return
	}
	p.ensureMessagesLocked(scope)
	cfg, ok := p.agents[agent]
	if !ok || cfg.System == "" {
		return
	}
	msgs := p.messagesByScope[scope]
	msgs = append(msgs, map[string]any{"role": "system", "content": cfg.System})
	p.messagesByScope[scope] = msgs
	p.lastInjected[scope] = agent
}

func (p *LLMProcessor) resolveAgent(meta map[string]string, streamID string) string {
	agent := meta[frames.MetaAgent]
	if agent == "" {
		p.mu.Lock()
		agent = p.activeAgent[streamID]
		p.mu.Unlock()
	}
	if agent == "" {
		agent = p.defaultAgent
	}
	if agent == "" {
		return agent
	}
	p.mu.Lock()
	if p.activeAgent[streamID] != agent {
		p.activeAgent[streamID] = agent
	}
	p.mu.Unlock()
	return agent
}

func (p *LLMProcessor) scopeKey(meta map[string]string, streamID string) string {
	if meta != nil {
		if callSID := strings.TrimSpace(meta[frames.MetaCallSID]); callSID != "" {
			return "call:" + callSID
		}
		if sid := strings.TrimSpace(meta[frames.MetaStreamID]); sid != "" {
			return "stream:" + sid
		}
	}
	if streamID != "" {
		return "stream:" + streamID
	}
	return defaultLLMScope
}

func scopeKeyOrDefault(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return defaultLLMScope
	}
	return scope
}

func (p *LLMProcessor) ensureMessagesLocked(scope string) []map[string]any {
	scope = scopeKeyOrDefault(scope)
	msgs, ok := p.messagesByScope[scope]
	if !ok {
		if p.system != "" {
			msgs = []map[string]any{{"role": "system", "content": p.system}}
		} else {
			msgs = []map[string]any{}
		}
		p.messagesByScope[scope] = msgs
	}
	return msgs
}

func (p *LLMProcessor) setLanguageFromMeta(meta map[string]string) {
	if meta == nil {
		return
	}
	streamID := meta[frames.MetaStreamID]
	if streamID == "" {
		return
	}
	lang := meta[frames.MetaLanguage]
	if lang == "" {
		lang = meta[frames.MetaGlobalLanguage]
	}
	if lang == "" {
		return
	}
	p.mu.Lock()
	p.lastLanguage[streamID] = lang
	if callSID := meta[frames.MetaCallSID]; callSID != "" {
		p.lastLanguageByCall[callSID] = lang
	}
	p.mu.Unlock()
}

func (p *LLMProcessor) setCallSIDFromMeta(meta map[string]string) {
	if meta == nil {
		return
	}
	streamID := meta[frames.MetaStreamID]
	callSID := meta[frames.MetaCallSID]
	if streamID == "" || callSID == "" {
		return
	}
	p.mu.Lock()
	p.lastCallSID[streamID] = callSID
	p.mu.Unlock()
}

func (p *LLMProcessor) languageForStream(streamID string) string {
	if streamID == "" {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastLanguage[streamID]
}

func (p *LLMProcessor) applyLanguageMeta(meta map[string]string, streamID string) {
	if meta == nil || streamID == "" {
		return
	}
	if meta[frames.MetaLanguage] != "" {
		return
	}
	if lang := p.languageForStream(streamID); lang != "" {
		meta[frames.MetaLanguage] = lang
		return
	}
	if callSID := meta[frames.MetaCallSID]; callSID != "" {
		p.mu.Lock()
		lang := p.lastLanguageByCall[callSID]
		p.mu.Unlock()
		if lang != "" {
			meta[frames.MetaLanguage] = lang
		}
	}
}

func (p *LLMProcessor) clearCall(meta map[string]string) {
	if meta == nil {
		return
	}
	streamID := meta[frames.MetaStreamID]
	callSID := meta[frames.MetaCallSID]
	p.mu.Lock()
	delete(p.activeAgent, streamID)
	delete(p.lastLanguage, streamID)
	delete(p.pendingConfirms, streamID)
	delete(p.lastCallSID, streamID)
	if streamID != "" {
		delete(p.messagesByScope, "stream:"+streamID)
		delete(p.lastInjected, "stream:"+streamID)
	}
	if callSID != "" {
		delete(p.messagesByScope, "call:"+callSID)
		delete(p.lastInjected, "call:"+callSID)
	}
	if callSID != "" {
		delete(p.lastLanguageByCall, callSID)
	}
	p.mu.Unlock()
}

func (p *LLMProcessor) adapterFor(agent string) llm.LLMAdapter {
	p.mu.Lock()
	defer p.mu.Unlock()
	if cfg, ok := p.agents[agent]; ok && cfg.Adapter != nil {
		return cfg.Adapter
	}
	return p.adapter
}

func (p *LLMProcessor) contextSnapshot(scope string) llm.Context {
	p.mu.Lock()
	defer p.mu.Unlock()
	msgs := p.ensureMessagesLocked(scope)
	return llm.Context{Messages: cloneMessages(msgs), Tools: p.toolsLocked()}
}

func indexTools(tools []llm.Tool) map[string]llm.Tool {
	out := make(map[string]llm.Tool)
	for _, t := range tools {
		if t.Name == "" {
			continue
		}
		out[t.Name] = t
	}
	return out
}

func (p *LLMProcessor) toolForName(name string) (llm.Tool, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	t, ok := p.toolIndex[name]
	return t, ok
}

func (p *LLMProcessor) handleToolCallsWithConfirmation(streamID string, calls []llm.ToolCall, meta map[string]string) []frames.Frame {
	for _, call := range calls {
		if t, ok := p.toolForName(call.Name); ok && t.RequiresConfirmation {
			p.storePendingConfirm(streamID, call, meta, t)
			return []frames.Frame{p.confirmPromptFrame(streamID, meta, t)}
		}
	}
	out := []frames.Frame{
		frames.NewSystemFrame(streamID, time.Now().UnixNano(), "thinking_start", meta),
	}
	out = append(out, p.emitToolCalls(streamID, calls, meta)...)
	out = append(out, frames.NewSystemFrame(streamID, time.Now().UnixNano(), "thinking_end", meta))
	return out
}

func (p *LLMProcessor) storePendingConfirm(streamID string, call llm.ToolCall, meta map[string]string, tool llm.Tool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if streamID == "" {
		return
	}
	if meta == nil {
		meta = map[string]string{}
	}
	clone := map[string]string{}
	for k, v := range meta {
		clone[k] = v
	}
	p.pendingConfirms[streamID] = pendingToolConfirm{
		call:    call,
		meta:    clone,
		prompt:  confirmPromptFromTool(tool, languageFromMeta(meta)),
		created: time.Now(),
	}
}

func confirmPromptFromTool(tool llm.Tool, lang string) string {
	if tool.ConfirmationPromptByLanguage != nil {
		if prompt := tool.ConfirmationPromptByLanguage[lang]; prompt != "" {
			return prompt
		}
	}
	if tool.ConfirmationPrompt != "" {
		return tool.ConfirmationPrompt
	}
	return defaultConfirmPrompt(lang)
}

func languageFromMeta(meta map[string]string) string {
	if meta == nil {
		return ""
	}
	lang := strings.ToLower(strings.TrimSpace(meta[frames.MetaLanguage]))
	if lang == "" {
		lang = strings.ToLower(strings.TrimSpace(meta[frames.MetaGlobalLanguage]))
	}
	return lang
}

func (p *LLMProcessor) confirmPromptFrame(streamID string, meta map[string]string, tool llm.Tool) frames.SystemFrame {
	prompt := confirmPromptFromTool(tool, languageFromMeta(meta))
	outMeta := map[string]string{
		frames.MetaStreamID:     streamID,
		frames.MetaGreetingText: prompt,
	}
	if meta != nil {
		if callSID := meta[frames.MetaCallSID]; callSID != "" {
			outMeta[frames.MetaCallSID] = callSID
		}
		if traceID := meta[frames.MetaTraceID]; traceID != "" {
			outMeta[frames.MetaTraceID] = traceID
		}
		if agent := meta[frames.MetaAgent]; agent != "" {
			outMeta[frames.MetaAgent] = agent
		}
	}
	p.applyLanguageMeta(outMeta, streamID)
	return frames.NewSystemFrame(streamID, time.Now().UnixNano(), "tool_confirm_prompt", outMeta)
}

func (p *LLMProcessor) handlePendingConfirmation(streamID string, tf frames.TextFrame) ([]frames.Frame, bool) {
	p.mu.Lock()
	pending, ok := p.pendingConfirms[streamID]
	mode := p.confirmMode
	llmFallback := p.confirmLLMFallback
	p.mu.Unlock()
	if !ok {
		return nil, false
	}
	text := tf.Text()
	mode = normalizeConfirmMode(mode)
	useKeywords := mode == "hybrid" || mode == "keywords"
	useLLM := mode == "hybrid" || mode == "llm"
	llmEnabled := useLLM && (mode == "llm" || llmFallback)

	if useKeywords {
		confirm, cancel := confirmationIntent(text)
		if confirm {
			p.mu.Lock()
			delete(p.pendingConfirms, streamID)
			p.mu.Unlock()
			meta := pending.meta
			if meta == nil {
				meta = map[string]string{}
			}
			meta[frames.MetaStreamID] = streamID
			p.applyLanguageMeta(meta, streamID)
			out := []frames.Frame{frames.NewSystemFrame(streamID, time.Now().UnixNano(), "thinking_start", meta)}
			out = append(out, p.emitToolCalls(streamID, []llm.ToolCall{pending.call}, meta)...)
			out = append(out, frames.NewSystemFrame(streamID, time.Now().UnixNano(), "thinking_end", meta))
			return out, true
		}
		if cancel {
			p.mu.Lock()
			delete(p.pendingConfirms, streamID)
			p.mu.Unlock()
			cancelPrompt := defaultCancelPrompt(languageFromMeta(pending.meta))
			cancelMeta := map[string]string{
				frames.MetaStreamID:     streamID,
				frames.MetaGreetingText: cancelPrompt,
			}
			if pending.meta != nil {
				if callSID := pending.meta[frames.MetaCallSID]; callSID != "" {
					cancelMeta[frames.MetaCallSID] = callSID
				}
				if traceID := pending.meta[frames.MetaTraceID]; traceID != "" {
					cancelMeta[frames.MetaTraceID] = traceID
				}
			}
			p.applyLanguageMeta(cancelMeta, streamID)
			return []frames.Frame{frames.NewSystemFrame(streamID, time.Now().UnixNano(), "tool_confirm_cancelled", cancelMeta)}, true
		}
	}

	if confirm, cancel := p.classifyConfirmationLLM(tf.Text(), pending.meta, llmEnabled); confirm || cancel {
		if confirm {
			p.mu.Lock()
			delete(p.pendingConfirms, streamID)
			p.mu.Unlock()
			meta := pending.meta
			if meta == nil {
				meta = map[string]string{}
			}
			meta[frames.MetaStreamID] = streamID
			p.applyLanguageMeta(meta, streamID)
			out := []frames.Frame{frames.NewSystemFrame(streamID, time.Now().UnixNano(), "thinking_start", meta)}
			out = append(out, p.emitToolCalls(streamID, []llm.ToolCall{pending.call}, meta)...)
			out = append(out, frames.NewSystemFrame(streamID, time.Now().UnixNano(), "thinking_end", meta))
			return out, true
		}
		if cancel {
			p.mu.Lock()
			delete(p.pendingConfirms, streamID)
			p.mu.Unlock()
			cancelPrompt := defaultCancelPrompt(languageFromMeta(pending.meta))
			cancelMeta := map[string]string{
				frames.MetaStreamID:     streamID,
				frames.MetaGreetingText: cancelPrompt,
			}
			if pending.meta != nil {
				if callSID := pending.meta[frames.MetaCallSID]; callSID != "" {
					cancelMeta[frames.MetaCallSID] = callSID
				}
				if traceID := pending.meta[frames.MetaTraceID]; traceID != "" {
					cancelMeta[frames.MetaTraceID] = traceID
				}
			}
			p.applyLanguageMeta(cancelMeta, streamID)
			return []frames.Frame{frames.NewSystemFrame(streamID, time.Now().UnixNano(), "tool_confirm_cancelled", cancelMeta)}, true
		}
	}

	// Ambiguous reply: repeat confirmation prompt.
	repeatPrompt := pending.prompt
	if repeatPrompt == "" {
		repeatPrompt = defaultConfirmPrompt(languageFromMeta(pending.meta))
	}
	repeatMeta := map[string]string{
		frames.MetaStreamID:     streamID,
		frames.MetaGreetingText: repeatPrompt,
	}
	if pending.meta != nil {
		if callSID := pending.meta[frames.MetaCallSID]; callSID != "" {
			repeatMeta[frames.MetaCallSID] = callSID
		}
		if traceID := pending.meta[frames.MetaTraceID]; traceID != "" {
			repeatMeta[frames.MetaTraceID] = traceID
		}
	}
	p.applyLanguageMeta(repeatMeta, streamID)
	return []frames.Frame{frames.NewSystemFrame(streamID, time.Now().UnixNano(), "tool_confirm_repeat", repeatMeta)}, true
}

func (p *LLMProcessor) emitToolCalls(streamID string, calls []llm.ToolCall, meta map[string]string) []frames.Frame {
	var out []frames.Frame
	p.mu.Lock()
	for _, call := range calls {
		p.pendingTools[call.ID] = call
		args, _ := json.Marshal(call.Arguments)
		outMeta := map[string]string{
			frames.MetaStreamID:   streamID,
			frames.MetaToolCallID: call.ID,
			frames.MetaToolName:   call.Name,
			frames.MetaToolArgs:   string(args),
		}
		if tool, ok := p.toolIndex[call.Name]; ok && tool.RequiresConfirmation {
			outMeta[frames.MetaToolRequiresConfirm] = "true"
			prompt := confirmPromptFromTool(tool, languageFromMeta(meta))
			if prompt != "" {
				outMeta[frames.MetaToolConfirmPrompt] = prompt
			}
		}
		if meta != nil {
			if callSID := meta[frames.MetaCallSID]; callSID != "" {
				outMeta[frames.MetaCallSID] = callSID
			}
			if traceID := meta[frames.MetaTraceID]; traceID != "" {
				outMeta[frames.MetaTraceID] = traceID
			}
			if lang := meta[frames.MetaLanguage]; lang != "" {
				outMeta[frames.MetaLanguage] = lang
			}
			if agent := meta[frames.MetaAgent]; agent != "" {
				outMeta[frames.MetaAgent] = agent
			}
		}
		out = append(out, frames.NewControlFrame(streamID, time.Now().UnixNano(), frames.ControlToolCall, outMeta))
	}
	p.mu.Unlock()
	return out
}

func (p *LLMProcessor) applyToolResult(sf frames.SystemFrame) ([]frames.Frame, error) {
	meta := sf.Meta()
	streamID := meta[frames.MetaStreamID]
	scope := p.scopeKey(meta, streamID)
	p.setLanguageFromMeta(meta)
	callID := meta[frames.MetaToolCallID]
	result := meta[frames.MetaToolResult]
	status := strings.ToLower(meta[frames.MetaToolStatus])
	if status != "" && status != "ok" {
		p.appendSystem(scope, toolFailureSystemMessage(languageFromMeta(meta)))
	}
	if callID == "" || result == "" {
		return nil, nil
	}
	p.mu.Lock()
	call, ok := p.pendingTools[callID]
	if ok {
		delete(p.pendingTools, callID)
	}
	toolName := call.Name
	if toolName == "" {
		toolName = meta[frames.MetaToolName]
	}
	if status == "" {
		status = "ok"
	}
	p.mu.Unlock()
	p.recordToolResult(streamID, meta[frames.MetaTraceID], toolName, status, meta[frames.MetaToolError])

	p.mu.Lock()
	msgs := p.ensureMessagesLocked(scope)
	msgs = append(msgs, map[string]any{
		"role": "assistant",
		"tool_calls": []map[string]any{
			{
				"id":   callID,
				"type": "function",
				"function": map[string]any{
					"name":      call.Name,
					"arguments": call.Arguments,
				},
			},
		},
	})
	msgs = append(msgs, map[string]any{
		"role":         "tool",
		"tool_call_id": callID,
		"content":      result,
	})
	msgs = p.pruneMessagesLocked(msgs)
	p.messagesByScope[scopeKeyOrDefault(scope)] = msgs
	p.mu.Unlock()
	adapter := p.adapterFor(p.resolveAgent(meta, streamID))
	ctx := p.contextSnapshot(scope)
	ch, err := adapter.Stream(p.ctx, ctx)
	if err != nil {
		reason := errorsx.ReasonLLMStream
		if resilience.IsRateLimit(err) {
			reason = errorsx.ReasonLLMRateLimit
		}
		err = errorsx.Wrap(err, reason)
		slog.Error("llm_stream_error", "stream_id", streamID, "reason_code", string(errorsx.Reason(err)), "error", err)
		fallback := frames.NewControlFrame(streamID, time.Now().UnixNano(), frames.ControlFallback, meta)
		return []frames.Frame{fallback}, nil
	}
	return p.streamToFrames(frames.NewTextFrame(streamID, sf.PTS(), "", meta), ch), nil
}

func confirmationIntent(text string) (bool, bool) {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return false, false
	}
	t = strings.TrimPrefix(t, "dtmf input:")
	t = strings.TrimSpace(t)
	tokens := splitTokens(t)
	for _, tok := range tokens {
		switch tok {
		case "1":
			return true, false
		case "2":
			return false, true
		}
	}
	yesWords := map[string]struct{}{
		"ya": {}, "iya": {}, "y": {}, "yes": {}, "yep": {}, "yup": {}, "sure": {}, "ok": {}, "oke": {}, "okay": {}, "okey": {}, "lanjut": {}, "setuju": {}, "boleh": {}, "confirm": {}, "siap": {}, "sip": {}, "baik": {}, "benar": {},
	}
	noWords := map[string]struct{}{
		"tidak": {}, "gak": {}, "nggak": {}, "ngga": {}, "enggak": {}, "ga": {}, "no": {}, "nope": {}, "cancel": {}, "batal": {}, "jangan": {}, "stop": {}, "jgn": {},
	}
	for _, tok := range tokens {
		if _, ok := yesWords[tok]; ok {
			return true, false
		}
		if _, ok := noWords[tok]; ok {
			return false, true
		}
	}
	return false, false
}

func splitTokens(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		if r >= 'a' && r <= 'z' {
			return false
		}
		if r >= '0' && r <= '9' {
			return false
		}
		return true
	})
}

func defaultConfirmPrompt(lang string) string {
	if isEnglish(lang) {
		return "Before I proceed, do you want me to continue?"
	}
	return "Sebelum saya lanjut, apakah boleh saya teruskan?"
}

func defaultCancelPrompt(lang string) string {
	if isEnglish(lang) {
		return "Okay, I won't proceed."
	}
	return "Baik, saya tidak akan melanjutkan."
}

func toolFailureSystemMessage(lang string) string {
	if isEnglish(lang) {
		return "The tool failed or timed out. Summarize briefly and suggest the next step."
	}
	return "Tool gagal atau timeout. Berikan ringkasan singkat dan sarankan langkah berikutnya."
}

func normalizeConfirmMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "llm", "llm_only", "full_llm":
		return "llm"
	case "keywords", "keyword", "rule", "rules", "rules_only":
		return "keywords"
	case "hybrid", "":
		return "hybrid"
	default:
		return "hybrid"
	}
}

func isEnglish(lang string) bool {
	lang = strings.ToLower(strings.TrimSpace(lang))
	return strings.HasPrefix(lang, "en")
}

func (p *LLMProcessor) classifyConfirmationLLM(text string, meta map[string]string, enabled bool) (bool, bool) {
	p.mu.Lock()
	timeout := p.confirmTimeout
	adapter := p.adapter
	p.mu.Unlock()
	if !enabled || adapter == nil {
		return false, false
	}
	if timeout <= 0 {
		timeout = 600 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(p.ctx, timeout)
	defer cancel()
	lang := languageFromMeta(meta)
	prompt := confirmationClassifierPrompt(lang)
	input := llm.Context{
		Messages: []map[string]any{
			{"role": "system", "content": prompt},
			{"role": "user", "content": text},
		},
	}
	resp, err := adapter.Generate(ctx, input)
	if err != nil {
		return false, false
	}
	return parseConfirmationDecision(resp.Text)
}

func confirmationClassifierPrompt(lang string) string {
	if isEnglish(lang) {
		return "Classify the user's reply to a confirmation request. Reply with only: yes, no, or other. Consider Indonesian/English, short replies, and DTMF (1=yes, 2=no). If unsure, answer other."
	}
	return "Klasifikasikan jawaban user terhadap permintaan konfirmasi. Jawab hanya: yes, no, atau other. Pertimbangkan bahasa Indonesia/Inggris, jawaban singkat, dan DTMF (1=yes, 2=no). Jika ragu, jawab other."
}

func parseConfirmationDecision(text string) (bool, bool) {
	tokens := splitTokens(strings.ToLower(text))
	for _, tok := range tokens {
		switch tok {
		case "yes", "ya", "iya", "y", "benar", "setuju":
			return true, false
		case "no", "tidak", "gak", "nggak", "ga", "cancel", "batal":
			return false, true
		}
	}
	return false, false
}

func (p *LLMProcessor) pruneMessagesLocked(messages []map[string]any) []map[string]any {
	if p.maxHistory <= 0 && p.maxTokens <= 0 {
		return messages
	}
	if len(messages) == 0 {
		return messages
	}
	if p.maxHistory > 0 {
		messages = pruneByHistory(messages, p.maxHistory)
	}
	if p.maxTokens > 0 {
		messages = pruneByTokens(messages, p.maxTokens)
	}
	return messages
}

func pruneByHistory(messages []map[string]any, maxHistory int) []map[string]any {
	if maxHistory <= 0 {
		return messages
	}
	nonSystem := nonSystemIndices(messages)
	if len(nonSystem) <= maxHistory {
		return messages
	}
	toDrop := len(nonSystem) - maxHistory
	drop := make(map[int]struct{}, toDrop)
	for i := 0; i < toDrop; i++ {
		drop[nonSystem[i]] = struct{}{}
	}
	filtered := make([]map[string]any, 0, len(messages)-toDrop)
	for idx, msg := range messages {
		if _, ok := drop[idx]; ok {
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
}

func pruneByTokens(messages []map[string]any, maxTokens int) []map[string]any {
	if maxTokens <= 0 {
		return messages
	}
	for {
		total := estimateMessagesTokens(messages)
		if total <= maxTokens {
			return messages
		}
		nonSystem := nonSystemIndices(messages)
		if len(nonSystem) == 0 {
			return messages
		}
		dropIdx := nonSystem[0]
		filtered := make([]map[string]any, 0, len(messages)-1)
		for i, msg := range messages {
			if i == dropIdx {
				continue
			}
			filtered = append(filtered, msg)
		}
		messages = filtered
	}
}

func nonSystemIndices(messages []map[string]any) []int {
	out := make([]int, 0, len(messages))
	for i, msg := range messages {
		if role, _ := msg["role"].(string); strings.ToLower(role) != "system" {
			out = append(out, i)
		}
	}
	return out
}

func estimateMessagesTokens(messages []map[string]any) int {
	total := 0
	for _, msg := range messages {
		total += estimateMessageTokens(msg)
	}
	return total
}

func estimateMessageTokens(msg map[string]any) int {
	if msg == nil {
		return 0
	}
	var parts []string
	if v, ok := msg["content"].(string); ok {
		parts = append(parts, v)
	}
	if rawParts, ok := msg["content_parts"].([]map[string]any); ok {
		for _, part := range rawParts {
			if text, ok := part["text"].(string); ok {
				parts = append(parts, text)
			}
		}
	} else if rawParts, ok := msg["content_parts"].([]any); ok {
		for _, part := range rawParts {
			if pmap, ok := part.(map[string]any); ok {
				if text, ok := pmap["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
	}
	if len(parts) == 0 {
		return 0
	}
	joined := strings.Join(parts, " ")
	return len(splitTokens(joined))
}

func (p *LLMProcessor) streamToFrames(src frames.TextFrame, ch <-chan string) []frames.Frame {
	var out []frames.Frame
	var full strings.Builder
	var chunk strings.Builder
	first := true
	streamID := src.Meta()[frames.MetaStreamID]
	scope := p.scopeKey(src.Meta(), streamID)
	const minChunkLen = 120
	emitChunk := func(text string, flush bool) {
		meta := src.Meta()
		meta[frames.MetaSource] = "llm"
		p.applyLanguageMeta(meta, streamID)
		if flush {
			meta[frames.MetaTTSFlush] = "true"
		}
		out = append(out, frames.NewTextFrame(streamID, time.Now().UnixNano(), text, meta))
	}
	for tok := range ch {
		full.WriteString(tok)
		chunk.WriteString(tok)
		if first {
			first = false
			p.record("llm_first_token", streamID, src.Meta()[frames.MetaTraceID])
		}
		if chunk.Len() >= minChunkLen {
			emitChunk(chunk.String(), false)
			chunk.Reset()
		}
	}
	if chunk.Len() > 0 {
		emitChunk(chunk.String(), true)
	} else {
		emitChunk("", true)
	}
	p.appendAssistant(scope, full.String())
	p.recordWithFields("llm_output_text", streamID, src.Meta()[frames.MetaTraceID], map[string]any{"text": redact.Text(full.String())})
	p.record("llm_done", streamID, src.Meta()[frames.MetaTraceID])
	return out
}

func (p *LLMProcessor) appendAssistant(scope, text string) {
	if text == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	msgs := p.ensureMessagesLocked(scope)
	msgs = append(msgs, map[string]any{"role": "assistant", "content": text})
	msgs = p.pruneMessagesLocked(msgs)
	p.messagesByScope[scopeKeyOrDefault(scope)] = msgs
}

func (p *LLMProcessor) appendSystem(scope, text string) {
	if text == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	msgs := p.ensureMessagesLocked(scope)
	msgs = append(msgs, map[string]any{"role": "system", "content": text})
	msgs = p.pruneMessagesLocked(msgs)
	p.messagesByScope[scopeKeyOrDefault(scope)] = msgs
}

func (p *LLMProcessor) popLastMessage(scope string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	msgs := p.ensureMessagesLocked(scope)
	if len(msgs) == 0 {
		return
	}
	msgs = msgs[:len(msgs)-1]
	p.messagesByScope[scopeKeyOrDefault(scope)] = msgs
}

func cloneMessages(in []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, m := range in {
		c := make(map[string]any, len(m))
		for k, v := range m {
			c[k] = v
		}
		out = append(out, c)
	}
	return out
}

var _ pipeline.FrameProcessor = (*LLMProcessor)(nil)

func (p *LLMProcessor) record(name, streamID, traceID string) {
	if p.obs == nil {
		return
	}
	tags := map[string]string{frames.MetaStreamID: streamID, "component": "llm"}
	if traceID != "" {
		tags[frames.MetaTraceID] = traceID
	}
	if callSID := p.callSIDForStream(streamID); callSID != "" {
		tags[frames.MetaCallSID] = callSID
	}
	if p.adapter != nil {
		tags["provider"] = p.adapter.Name()
	}
	p.obs.RecordEvent(metrics.MetricsEvent{
		Name: name,
		Time: time.Now(),
		Tags: tags,
	})
}

func (p *LLMProcessor) recordWithFields(name, streamID, traceID string, fields map[string]any) {
	if p.obs == nil {
		return
	}
	tags := map[string]string{frames.MetaStreamID: streamID, "component": "llm"}
	if traceID != "" {
		tags[frames.MetaTraceID] = traceID
	}
	if callSID := p.callSIDForStream(streamID); callSID != "" {
		tags[frames.MetaCallSID] = callSID
	}
	if p.adapter != nil {
		tags["provider"] = p.adapter.Name()
	}
	p.obs.RecordEvent(metrics.MetricsEvent{
		Name:   name,
		Time:   time.Now(),
		Tags:   tags,
		Fields: fields,
	})
}

func (p *LLMProcessor) recordToolResult(streamID, traceID, toolName, status, errMsg string) {
	fields := map[string]any{
		"tool":   toolName,
		"status": status,
	}
	if errMsg != "" {
		fields["error"] = errMsg
	}
	p.recordWithFields("tool_result", streamID, traceID, fields)
}

func (p *LLMProcessor) callSIDForStream(streamID string) string {
	if streamID == "" {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastCallSID[streamID]
}
