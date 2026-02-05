# Ringkasan Konsep

Ini mental model yang perlu Anda pegang untuk build dan debug Ranya.

## 1. Frame adalah Kontrak
Setiap stage membaca frame dan memancarkan frame. Debug jadi deterministik.

- `audio`, `text`, `control`, `system`, `image`.
- Jika processor tidak memancarkan frame, pipeline berhenti di situ.

## 2. Pipeline Satu Arah
Frame mengalir maju. Control frame punya prioritas.

- Latensi stabil di bawah load.
- Interruption ditangani via `flush` dan `cancel`.

## 3. Turn Manager Pegang State
Listening, thinking, speaking adalah state eksplisit.

- Jangan taruh logika turn di prompt.
- Tuning `turn.*` dulu.

## 4. Routing Sebelum LLM
Routing hanya pada **final STT text**.

- Jika routing gagal, cek `is_final=true`.
- `bootstrap` untuk routing hanya turn awal.

## 5. Tools di Luar LLM
Tool dieksekusi dispatcher untuk safety.

- Retry dan timeout enforced.
- Konfirmasi eksplisit.

## 6. Observability = Debugger
Timeline menunjukkan stage terakhir yang jalan.

## Di Mana Menambah Logic

- **Before LLM**: normalisasi, prompt injection.
- **Before TTS**: formatting, truncation.
- **Post‑processor**: logging, serialization.

## Pendalaman

- [Frame dan Metadata](frames.md)
- [Pipeline dan Backpressure](pipeline.md)
- [Manajemen Giliran](turn-management.md)
- [Routing dan Bahasa](routing.md)
- [Tools dan Konfirmasi](tools-confirmation.md)
- [Observabilitas](observability.md)
