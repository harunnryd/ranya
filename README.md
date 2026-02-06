<p align="center">
  <img src="docs/en/assets/logo-hikari.svg" alt="Ranya logo" width="144" />
</p>

# Ranya
[![CI](https://github.com/harunnryd/ranya/actions/workflows/ci.yml/badge.svg?branch=master)](https://github.com/harunnryd/ranya/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/harunnryd/ranya.svg)](https://pkg.go.dev/github.com/harunnryd/ranya)

Ranya is a frame-driven telephony voice-agent framework in Go for production call flows.

## Why Ranya
- Deterministic streaming pipeline that behaves predictably under real call pressure.
- Provider-agnostic architecture for STT, TTS, LLM, and transport.
- Built-in turn management for interruption, barge-in, and silence recovery.
- Tool execution with confirmations, retries, and idempotency-friendly flow.
- Observability-first design with timelines, latency, and cost artifacts.
- Privacy defaults (`privacy.redact_pii=true`) for safer operation.

## Start Fast (HVAC Reference)
### Prerequisites
- Go `1.24.13` (from `go.mod`).
- Public HTTPS endpoint for Twilio webhook (for example, ngrok/cloudflared).
- Provider credentials in environment variables.

### Required Environment Variables
- `TWILIO_ACCOUNT_SID`
- `TWILIO_AUTH_TOKEN`
- `TWILIO_PUBLIC_URL`
- `DEEPGRAM_API_KEY`
- `ELEVENLABS_API_KEY`
- `ELEVENLABS_VOICE_ID`
- `ELEVENLABS_VOICE_ID_EN`
- `OPENAI_API_KEY`

### Run
```bash
go run ./examples/hvac --config examples/hvac/config.yaml
```

### Verify
- Twilio sends inbound requests to `/voice`.
- Media stream upgrades to `/ws`.
- You see STT final frames, LLM text frames, and TTS audio frames.

## Minimal Engine Skeleton
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

## First Tasks for Engineers
1. Get a call working end-to-end.
   [Task 1: Get a Call Working](docs/en/task-1-call.md)
2. Add business tools with confirmations.
   [Task 2: Add Tools](docs/en/task-2-tools.md)
3. Add routing and multilingual behavior.
   [Task 3: Add Routing + Language](docs/en/task-3-routing.md)
4. Enable observability and fast debugging loops.
   [Task 4: Enable Observability](docs/en/task-4-observability.md)

## Providers
| Layer | Production | Local/Testing |
|---|---|---|
| Transport | Twilio | Mock |
| STT | Deepgram | Mock |
| TTS | ElevenLabs | Mock |
| LLM | OpenAI | Mock |

## Defaults That Matter
- `router.mode: "bootstrap"` for early-call routing with controlled cost.
- `confirmation.mode: "llm"` for safer tool execution.
- `pipeline.backpressure: "drop"` for latency protection.
- `observability.artifacts_dir` for timeline-driven debugging.
- `privacy.redact_pii: true` for safer logs/artifacts.

## Documentation
- English: [Home](docs/en/index.md), [Start Here](docs/en/start-here.md), [Task Flows](docs/en/task-flows.md)
- Bahasa Indonesia: [Beranda](docs/id/index.md), [Mulai di Sini](docs/id/start-here.md), [Alur Tugas](docs/id/task-flows.md)
- Japanese: [ホーム](docs/ja/index.md), [はじめに](docs/ja/start-here.md), [タスクフロー](docs/ja/task-flows.md)

## Roadmap
- [Roadmap](ROADMAP.md)
- [good first issue](https://github.com/harunnryd/ranya/labels/good%20first%20issue)
- [help wanted](https://github.com/harunnryd/ranya/labels/help%20wanted)

Open contribution queue:
- [#2 Add Makefile targets for local test and docs workflows](https://github.com/harunnryd/ranya/issues/2)
- [#3 Add provider capability matrix page (EN/ID/JA)](https://github.com/harunnryd/ranya/issues/3)
- [#4 Add Coolify docs deployment quickstart page (EN/ID/JA)](https://github.com/harunnryd/ranya/issues/4)
- [#5 Harden Twilio signature verification docs and proxy URL tests](https://github.com/harunnryd/ranya/issues/5)
- [#6 Add OpenTelemetry exporter for pipeline spans and metrics](https://github.com/harunnryd/ranya/issues/6)
- [#7 Add integration tests for router and language metadata flow](https://github.com/harunnryd/ranya/issues/7)
- [#8 Add preflight doctor command for config and env validation](https://github.com/harunnryd/ranya/issues/8)

## Deploy Docs with Docker Compose
Use `docker-compose.docs.yml` as the single source for docs runtime and local preview.

### Coolify / Production
Point Coolify to `docker-compose.docs.yml` and deploy service `docs`.

What it does:
- Builds all locales with MkDocs + Material (`mkdocs.yml`, `mkdocs.id.yml`, `mkdocs.ja.yml`).
- Serves static docs from Nginx.
- Routes:
  - `/en/` English
  - `/id/` Indonesian
  - `/ja/` Japanese
  - `/` redirects to `/en/`

Local smoke test:
```bash
DOCS_PORT=8080 docker compose -f docker-compose.docs.yml up --build -d docs
```
Open `http://localhost:8080`.

Note:
- In Coolify, you can leave `DOCS_PORT` unset to auto-pick a free host port (default behavior).
- If you need a fixed host port, set `DOCS_PORT` to an unused value.

### Local Live Preview (Hot Reload)
```bash
docker compose -f docker-compose.docs.yml --profile dev up docs-dev
```

Switch locale config:
```bash
MKDOCS_CONFIG=mkdocs.id.yml docker compose -f docker-compose.docs.yml --profile dev up docs-dev
MKDOCS_CONFIG=mkdocs.ja.yml docker compose -f docker-compose.docs.yml --profile dev up docs-dev
```

### Legacy Path (Nixpacks Static Build)
If you prefer static-site buildpack flow:
- Install Command: `pip install mkdocs-material`
- Build Command: `bash scripts/build_docs_coolify.sh`
- Publish Directory: `.coolify/docs`

## Repository Layout
- `pkg/` core framework modules.
- `examples/` reference implementations.
- `docs/` product documentation (EN/ID/JA).
- `scripts/` project helper scripts.

## Contributing
Read [CONTRIBUTING.md](CONTRIBUTING.md).

## PR Title Format
Use Conventional Commit style in PR titles.

Valid examples:
- `feat: add custom router strategy`
- `fix: handle twilio websocket reconnect`
- `docs: clarify task 1 setup`
- `refactor: simplify pipeline builder`
- `chore: update dependency versions`
- `feat!: remove legacy config fields`

Notes:
- Allowed types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`.
- Breaking changes: use `!` after type, for example `feat!: ...`.

## Security
Read [SECURITY.md](SECURITY.md).

## License
Apache-2.0. See [LICENSE](LICENSE).
