# Memulai

Panduan cepat untuk agent yang jalan.

## Prasyarat

- Versi Go dari `go.mod` (saat ini `1.24.13`).
- API key vendor jika memakai provider nyata.

## Install
```bash
go get github.com/harunnryd/ranya
```

## Jalankan Contoh
```bash
go run ./examples/hvac --config examples/hvac/config.yaml
```

## Wiring Engine
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

## Lanjut

- [Alur Tugas](task-flows.md)
- [Tugas 1: Call Jalan](task-1-call.md)
- [Tugas 2: Tambah Tools](task-2-tools.md)
- [Tugas 3: Routing + Bahasa](task-3-routing.md)
