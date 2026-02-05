# Alur Tugas

Empat tugas pertama yang biasanya dilakukan saat memakai Ranya.

## Tugas 1: Call Jalan (Twilio + Provider)

1. Pilih provider untuk Transport, STT, TTS, dan LLM.
2. Set env var dan mulai dari `examples/hvac/config.yaml`.
3. Jalankan contoh HVAC.
4. Pastikan call end‑to‑end selesai.
5. Jika gagal, gunakan timeline artifacts untuk mencari stage terakhir.

<div class="r-quick-links" markdown>
Related:

- [Tugas 1: Call Jalan](task-1-call.md)
- [Mulai di Sini](start-here.md)
- [Konfigurasi](configuration.md)
- [Penyedia](providers.md)
- [Contoh](examples.md)
- [Observabilitas](observability.md)
</div>

## Tugas 2: Tambah Tools (Aksi Bisnis)

1. Definisikan schema tool dengan `llm.Tool`.
2. Implement `llm.ToolRegistry` di aplikasi.
3. Aktifkan konfirmasi untuk aksi berisiko.
4. Atur timeout dan retry.
5. Pastikan frame `tool_call` dan `tool_result` muncul.

<div class="r-quick-links" markdown>
Related:

- [Tugas 2: Tambah Tools](task-2-tools.md)
- [Tools dan Konfirmasi](tools-confirmation.md)
- [Modul](modules.md)
- [Contoh](examples.md)
</div>

## Tugas 3: Routing dan Bahasa

1. Pilih `router.mode` (`off`, `bootstrap`, `full`).
2. Wire `RouterStrategy` (LLM router atau custom).
3. Tambah `LanguageDetector` jika perlu multi‑bahasa.
4. Pastikan final STT memakai `is_final=true`.

<div class="r-quick-links" markdown>
Related:

- [Tugas 3: Routing + Bahasa](task-3-routing.md)
- [Routing dan Bahasa](routing.md)
- [Frame dan Metadata](frames.md)
- [Konfigurasi](configuration.md)
</div>

## Tugas 4: Aktifkan Observabilitas dan Debugging

1. Set `observability.artifacts_dir` ke folder yang bisa ditulis.
2. Gunakan log JSON agar cepat mencari `trace_id`.
3. Buka timeline JSONL dan cari `frame_out` terakhir.
4. Gunakan event biaya dan latensi untuk validasi performa.

<div class="r-quick-links" markdown>
Related:

- [Tugas 4: Observabilitas](task-4-observability.md)
- [Observabilitas](observability.md)
- [Pemecahan Masalah](troubleshooting.md)
</div>
