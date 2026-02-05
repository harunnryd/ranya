# Tools and Confirmation

Tools are executed **outside** the LLM for safety. Definitions live in `pkg/llm`, execution in `pkg/ranya/dispatcher.go`.

## Implementation Steps

1. Define tool schemas (`llm.Tool`).
2. Implement `llm.ToolRegistry` in your app.
3. Enable confirmation if the action is risky.
4. Set timeouts and retry counts.

## Safety Defaults That Matter

- `tools.timeout_ms`: prevents stuck tool calls.
- `tools.retries`: transient failure recovery.
- `tools.serialize_by_stream`: avoids out‑of‑order tool execution.
- `confirmation.mode`: keyword vs LLM confirmation.

## Example Tool
```go
llm.Tool{
  Name: "schedule_visit",
  Description: "Schedule a technician visit.",
  RequiresConfirmation: true,
  ConfirmationPromptByLanguage: map[string]string{
    "id": "Sebelum saya jadwalkan kunjungan, apakah Anda ingin saya lanjutkan?",
    "en": "Before I schedule the visit, do you want me to proceed?",
  },
  Schema: map[string]any{
    "type": "object",
    "properties": map[string]any{
      "location": map[string]any{"type": "string"},
      "preferred_time": map[string]any{"type": "string"},
    },
    "required": []string{"location", "preferred_time"},
  },
}
```

## Confirmation Behavior

- DTMF: `1` = yes, `2` = no.
- Keyword match supports English and Indonesian.
- Enable `confirmation.llm_fallback` only if you need ambiguous classification.
