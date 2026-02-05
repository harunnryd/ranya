# モジュール（どこを変更するか）

## よくあるタスク
| タスク | 変更場所 |
| --- | --- |
| STT/TTS/LLM追加 | `pkg/providers` + `pkg/adapters/*` |
| ツール追加 | `llm.ToolRegistry` をアプリで実装 |
| LLM前にProcessor追加 | `EngineOptions.BeforeLLM` |
| LLM後にProcessor追加 | `EngineOptions.BeforeTTS` / `PostProcessors` |
| ルーティング変更 | `pkg/processors/router.go` / `RouterStrategy` |
| Barge‑in変更 | `pkg/turn` と `turn.*` |
| Observability追加 | `pkg/observers` |

## ミニコードマップ

- Engine: `pkg/ranya`
- Frames: `pkg/frames`
- Pipeline: `pkg/pipeline`
- Processors: `pkg/processors`
- Providers: `pkg/providers`
- Transports: `pkg/transports`
