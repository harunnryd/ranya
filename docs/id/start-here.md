# Mulai di Sini

Ini jalur paling cepat untuk voice agent yang jalan.

Kalau butuh langkah detail, lihat [Alur Tugas](task-flows.md).

## 1. Pilih Stack
Pilih satu opsi per peran. Anda bisa menukarnya nanti.

- **Transport**: Twilio (produksi) atau Mock (uji lokal).
- **STT**: Deepgram (produksi) atau Mock (uji lokal).
- **TTS**: ElevenLabs (produksi) atau Mock (uji lokal).
- **LLM**: OpenAI (produksi) atau Mock (uji lokal).

## 2. Jalankan Contoh Referensi
Contoh HVAC sudah mencakup routing, konfirmasi, recovery, dan summary.

```bash
go run ./examples/hvac --config examples/hvac/config.yaml
```

## 3. Wiring Engine Minimal
```go
package main

import (
  "context"
  "log"
  "os"
  "os/signal"

  "github.com/harunnryd/ranya/pkg/ranya"
)

func main() {
  cfg, err := ranya.LoadConfig("config.yaml")
  if err != nil {
    log.Fatal(err)
  }

  engine := ranya.NewEngine(ranya.EngineOptions{Config: cfg})

  ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
  defer stop()

  if err := engine.Start(ctx); err != nil {
    log.Fatal(err)
  }

  <-ctx.Done()
  _ = engine.Stop()
}
```

## 4. Tambahkan Minimum yang Dibutuhkan

- **Tools**: definisikan schema + handler.  
  [Tools dan Konfirmasi](tools-confirmation.md)

- **Routing**: pilih agent dan bahasa.  
  [Routing dan Bahasa](routing.md)

- **Observability**: nyalakan artifacts.  
  [Observabilitas](observability.md)

## 5. Validasi di Kondisi Produksi

- Tuning `turn.min_barge_in_ms` untuk interruption.
- Tuning `pipeline.backpressure` untuk latensi vs kelengkapan.
- Biarkan `privacy.redact_pii=true` kecuali ada kebijakan khusus.

## Selesai Jika

- Call end‑to‑end berjalan.
- Bisa menemukan error lewat timeline.
- Bisa ganti provider tanpa ubah kode.

## Lanjut

- [Konfigurasi](configuration.md)
- [Arsitektur](architecture.md)
- [Modul](modules.md)
