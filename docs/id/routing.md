# Routing dan Bahasa

Routing memilih agent (dan bahasa) sebelum LLM berjalan.

## Kapan Pakai Mode Ini
| Mode | Gunakan saat |
| --- | --- |
| `off` | Flow single agent. |
| `bootstrap` | Routing hanya di turn awal. |
| `full` | Intent bisa berubah kapan saja. |

## Alur Routing

- Router hanya jalan pada final STT text (`source=stt` dan `is_final=true`).
- `RouterStrategy` mengembalikan agent + metadata global.
- Agent disimpan per `stream_id` dan disuntikkan ke frame berikutnya.

## Deteksi Bahasa

- Jalan pada final STT text.
- `languages.code_switching=true` untuk deteksi setiap turn.

## Failure Umum

- Routing tidak jalan: STT tidak `is_final=true`.
- Bahasa tidak terdeteksi: tidak ada `LanguageDetector`.

## Wiring Minimal
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

## Konfigurasi Terkait
| Key | Makna |
| --- | --- |
| `router.mode` | `off`, `full`, `bootstrap`. |
| `router.max_turns` | Max turn untuk bootstrap. |
| `languages.code_switching` | Deteksi setiap turn. |
| `languages.default` | Bahasa default. |
