<div class="r-hero" markdown>
<div class="r-hero__content" markdown>

<div class="r-eyebrow">Framework Telephony Streaming</div>

# Ranya

<p class="r-hero__subtitle">Voice agent enterprise‑grade untuk panggilan nyata: pipeline deterministik, eksekusi tool aman, dan observabilitas kelas satu.</p>

<div class="r-hero__cta" markdown>
[Mulai di Sini](start-here.md){.md-button .md-button--primary}
[Alur Tugas](task-flows.md){.md-button}
</div>
</div>

<div class="r-hero__art" markdown>
<img src="assets/logo-hikari.svg" alt="Ranya Hikari logo" />
</div>
</div>

<div class="r-social" markdown>
[Repositori GitHub](https://github.com/harunnryd/ranya)
[Star](https://github.com/harunnryd/ranya/stargazers)
[Fork](https://github.com/harunnryd/ranya/network/members)
</div>

## Yang Bisa Anda Bangun
<div class="grid cards" markdown>

- **Inbound support line**  
  Barge‑in aman, routing cepat, latensi stabil.

- **Outbound call flow**  
  State jelas dan interruption terkontrol.

- **Voice agent bertool**  
  Aksi aman dengan konfirmasi, retry, idempotensi.

- **Deploy multi‑bahasa**  
  Deteksi bahasa dan routing provider.
</div>

## Tugas Utama
<div class="grid cards" markdown>

- **Call Harus Jalan**  
  Jalankan referensi end‑to‑end.  
  [Tugas 1: Call Jalan](task-1-call.md)

- **Tambah Tools**  
  Schema + konfirmasi.  
  [Tugas 2: Tambah Tools](task-2-tools.md)

- **Routing + Bahasa**  
  Routing agent dan deteksi bahasa.  
  [Tugas 3: Routing + Bahasa](task-3-routing.md)

- **Observabilitas**  
  Artifak, trace ID, dan timeline debug.  
  [Tugas 4: Observabilitas](task-4-observability.md)
</div>

## Keputusan Penting

- **Latensi vs kelengkapan**: `pipeline.backpressure` dan kapasitas queue.  
  [Pipeline dan Backpressure](pipeline.md)

- **Perilaku barge‑in**: `turn.min_barge_in_ms` dan reprompt.  
  [Manajemen Giliran](turn-management.md)

- **Safety tools**: konfirmasi dan timeout.  
  [Tools dan Konfirmasi](tools-confirmation.md)

- **Strategi routing**: `router.mode` dan deteksi bahasa.  
  [Routing dan Bahasa](routing.md)

- **Debug cepat**: artifacts dan trace ID.  
  [Observabilitas](observability.md)

## Alur Data (High‑Level)
```mermaid
flowchart LR
  Caller((Caller))
  Transport --> STT --> Turn --> Context --> Router --> LLM --> ToolDispatcher --> LLM --> TTS --> Transport
  Caller --> Transport
  Transport --> Caller
  Observers -.-> STT
  Observers -.-> LLM
  Observers -.-> TTS
```
