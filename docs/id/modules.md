# Modul (Di Mana Mengubah Apa)

Gunakan halaman ini untuk menemukan lokasi perubahan kode.

## Tugas Umum
| Tugas | Di mana |
| --- | --- |
| Tambah provider STT/TTS/LLM | `pkg/providers` + adapter di `pkg/adapters/*` |
| Tambah tool | Implement `llm.ToolRegistry` di app |
| Tambah processor sebelum LLM | `EngineOptions.BeforeLLM` |
| Tambah processor setelah LLM | `EngineOptions.BeforeTTS` atau `PostProcessors` |
| Ubah routing | `pkg/processors/router.go` / `RouterStrategy` |
| Ubah barge‑in | `pkg/turn` dan config `turn.*` |
| Tambah observability sink | `pkg/observers` |

## Peta Kode (Minimal)

- **Engine wiring**: `pkg/ranya`
- **Kontrak frame**: `pkg/frames`
- **Pipeline**: `pkg/pipeline`
- **Processor**: `pkg/processors`
- **Providers**: `pkg/providers`
- **Transport**: `pkg/transports`
