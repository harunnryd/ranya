# Getting Started

This guide gets you to a working agent quickly.

## Prerequisites

- Go version from `go.mod` (currently `1.24.13`).
- Vendor API keys if you run real providers.

## Install
```bash
go get github.com/harunnryd/ranya
```

## Run the Example
```bash
go run ./examples/hvac --config examples/hvac/config.yaml
```

## Wire Your Own Engine
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

## Next

- [Task Flows](task-flows.md)
- [Task 1: Get a Call Working](task-1-call.md)
- [Task 2: Add Tools](task-2-tools.md)
- [Task 3: Add Routing + Language](task-3-routing.md)
