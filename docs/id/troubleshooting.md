# Pemecahan Masalah

## 10 Menit Pertama

1. Cari `trace_id` di log.
2. Buka timeline JSONL.
3. Temukan `frame_out` terakhir.
4. Perbaiki stage yang berhenti.

## Gejala dan Solusi
| Gejala | Penyebab | Solusi |
| --- | --- | --- |
| Router tidak memilih agent | STT tidak final | Pastikan `is_final=true` dan `source=stt`. |
| Bahasa tidak terdeteksi | Tidak ada language detector | Set `EngineOptions.LanguageDetector`. |
| Tools tidak jalan | Tool registry kosong | Set `EngineOptions.Tools`. |
| Konfirmasi berulang | Jawaban ambigu | Gunakan yes/no jelas atau DTMF `1`/`2`. |
| Barge‑in tidak jalan | Threshold tinggi | Turunkan `turn.min_barge_in_ms`. |
| Silence reprompt mati | Reprompt disabled | Set `turn.silence_reprompt.timeout_ms`. |
| End‑of‑turn lambat | STT tidak final | Set `turn.end_of_turn_timeout_ms`. |
| Frame drop | Backpressure drop | Pakai `pipeline.backpressure=wait` atau tambah buffer. |
