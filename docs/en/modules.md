# Modules (Where to Change What)

Use this page to find the right place to edit code.

## Common Tasks
| Task | Where to change |
| --- | --- |
| Add a new STT/TTS/LLM provider | `pkg/providers` + adapter in `pkg/adapters/*` |
| Add a new tool | Your app implements `llm.ToolRegistry` |
| Add a processor before LLM | `EngineOptions.BeforeLLM` |
| Add a processor after LLM | `EngineOptions.BeforeTTS` or `PostProcessors` |
| Change routing logic | `pkg/processors/router.go` or custom `RouterStrategy` |
| Change barge‑in behavior | `pkg/turn` and `turn.*` config |
| Add observability sinks | `pkg/observers` |

## Code Map (Minimal)

- **Engine wiring**: `pkg/ranya`
- **Frames contract**: `pkg/frames`
- **Pipeline execution**: `pkg/pipeline`
- **Core processors**: `pkg/processors`
- **Providers**: `pkg/providers`
- **Transports**: `pkg/transports`
