# Routing and Language

Routing chooses the right agent (and language) before the LLM runs. It is handled by `pkg/processors/router.go`.

## When to Use Each Mode
| Mode | Use when |
| --- | --- |
| `off` | Single‑agent flows. |
| `bootstrap` | You only need routing for the first turns. |
| `full` | Routing can change anytime (multi‑intent calls). |

## Routing Flow

- Router runs only on final STT text frames (`source=stt` and `is_final=true`).
- `RouterStrategy` returns the agent and optional global metadata.
- The agent is stored per `stream_id` and injected into later frames.

## Language Detection

- Detection runs on final STT text.
- If `languages.code_switching=true`, detection can run on every final turn.
- If `languages.code_switching=false`, detection stops after the first language.

## Common Failure Points

- Routing never fires: STT final frames missing `is_final=true`.
- Language never sets: no `LanguageDetector` in `EngineOptions`.

## Minimal Wiring
```go
router := NewLLMRouterStrategy(llmAdapter, nil, LLMRouterConfig{})
opts := ranya.EngineOptions{
  Config:           cfg,
  Router:           router,
  LanguageDetector: myDetector,
  LanguagePrompts:  map[string]string{"id": "...", "en": "..."},
}
app := ranya.NewEngine(opts)
```

## Related Config
| Key | Meaning |
| --- | --- |
| `router.mode` | `off`, `full`, or `bootstrap`. |
| `router.max_turns` | Max routing turns for bootstrap mode. |
| `languages.code_switching` | Allow language detection on every turn. |
| `languages.default` | Default language for providers. |
