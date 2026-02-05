# Module Deep Dive (Debug Map)

Use this page when you are debugging a production issue.

## Follow the Frame

1. **Transport**: Did audio frames arrive with `stream_id`?
2. **STT**: Did final transcripts emit `is_final=true`?
3. **Router/Context**: Did `agent` or `global_*` appear?
4. **LLM**: Did `thinking_start` or output frames appear?
5. **Tools**: Did `tool_call` and `tool_result` appear?
6. **TTS**: Did audio frames go back to transport?
7. **Turn**: Did `flush` and `cancel` fire on barge‑in?

## Where to Look in Code

- Frame contract: `pkg/frames`
- Pipeline: `pkg/pipeline`
- Turn logic: `pkg/turn`
- LLM + tools: `pkg/processors/llm.go`, `pkg/ranya/dispatcher.go`
- Observability: `pkg/metrics`, `pkg/observers`
