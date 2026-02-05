# Tools dan Konfirmasi

Tool dieksekusi **di luar** LLM untuk safety.

## Langkah Implementasi

1. Definisikan schema tool (`llm.Tool`).
2. Implement `llm.ToolRegistry` di aplikasi.
3. Aktifkan konfirmasi untuk aksi berisiko.
4. Set timeout dan retry.

## Default Safety yang Penting

- `tools.timeout_ms`
- `tools.retries`
- `tools.serialize_by_stream`
- `confirmation.mode`

## Contoh Tool
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

## Konfirmasi

- DTMF: `1` = yes, `2` = no.
- Keyword mendukung Inggris dan Indonesia.
- Aktifkan `confirmation.llm_fallback` hanya jika perlu klasifikasi ambigu.
