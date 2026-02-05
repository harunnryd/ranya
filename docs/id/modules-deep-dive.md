# Bedah Modul (Peta Debug)

Gunakan halaman ini saat ada issue di production.

## Ikuti Frame

1. **Transport**: Audio frame masuk dengan `stream_id`?
2. **STT**: Final transcript `is_final=true`?
3. **Router/Context**: `agent` atau `global_*` muncul?
4. **LLM**: `thinking_start` atau output muncul?
5. **Tools**: `tool_call` dan `tool_result`?
6. **TTS**: Audio frame kembali ke transport?
7. **Turn**: `flush`/`cancel` saat barge‑in?

## Di Mana Lihat Kode

- Kontrak frame: `pkg/frames`
- Pipeline: `pkg/pipeline`
- Turn logic: `pkg/turn`
- LLM + tools: `pkg/processors/llm.go`, `pkg/ranya/dispatcher.go`
- Observability: `pkg/metrics`, `pkg/observers`
