# Start Here

This is the shortest path to a working voice agent.

If you want step‑by‑step flows, see [Task Flows](task-flows.md).

## 1. Choose Your Stack
Pick one option per role. You can swap providers later.

- **Transport**: Twilio (production) or Mock (local tests).
- **STT**: Deepgram (production) or Mock (local tests).
- **TTS**: ElevenLabs (production) or Mock (local tests).
- **LLM**: OpenAI (production) or Mock (local tests).

## 2. Run the Reference Example
The HVAC example includes routing, confirmations, recovery, and summaries.

```bash
go run ./examples/hvac --config examples/hvac/config.yaml
```

## 3. Wire Your Minimal Engine
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

## 4. Add the Minimum You Need

- **Tools**: define tool schemas and handlers.  
  [Tools and Confirmation](tools-confirmation.md)

- **Routing**: set agent and language early.  
  [Routing and Language](routing.md)

- **Observability**: enable artifacts and trace IDs.  
  [Observability](observability.md)

## 5. Validate in Production Conditions

- Tune `turn.min_barge_in_ms` for interruption behavior.
- Tune `pipeline.backpressure` for latency vs completeness.
- Keep `privacy.redact_pii=true` unless you have an explicit policy.

## Done When

- You can complete a full call end‑to‑end.
- You can identify a failure using the timeline artifacts.
- You can swap one provider without code changes.

## Next Steps

- [Configuration](configuration.md)
- [Architecture](architecture.md)
- [Modules](modules.md)
