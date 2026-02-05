# Observabilitas

Gunakan observability untuk menemukan stage yang gagal.

## Yang Didapat

- Timeline JSONL per call.
- Cost summary per call.
- Latensi per stage.

## Setup Rekomendasi

- Dev: artifacts + record audio.
- Prod: artifacts aktif, audio off kecuali ada kebijakan.

## Alur Debug

1. Temukan `trace_id` di log.
2. Buka timeline JSONL.
3. Cari `frame_out` terakhir.
4. Perbaiki stage yang berhenti mengirim frame.

## Key Config
| Key | Makna |
| --- | --- |
| `observability.artifacts_dir` | Lokasi file timeline + cost. |
| `observability.record_audio` | Sertakan payload audio. |
| `observability.retention_days` | Hapus artifact lama saat startup. |
