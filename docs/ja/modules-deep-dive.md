# モジュール深掘り（デバッグマップ）

## Frameの流れを追う

1. **Transport**: `stream_id` 付きAudioFrameが来ているか。
2. **STT**: `is_final=true` のテキストがあるか。
3. **Router/Context**: `agent` や `global_*` があるか。
4. **LLM**: `thinking_start` または出力があるか。
5. **Tools**: `tool_call` と `tool_result` があるか。
6. **TTS**: AudioFrameが戻っているか。
7. **Turn**: `flush`/`cancel` が出ているか。

## 参照コード

- Frames: `pkg/frames`
- Pipeline: `pkg/pipeline`
- Turn: `pkg/turn`
- LLM/Tools: `pkg/processors/llm.go`, `pkg/ranya/dispatcher.go`
- Observability: `pkg/metrics`, `pkg/observers`
