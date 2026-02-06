# Ranya Roadmap

This roadmap shows what we plan to build next so contributors can align early.

## Product Direction

Ranya focuses on production-grade realtime voice agents with:
- deterministic frame pipelines,
- safe tool execution,
- provider portability,
- and observability-first operations.

This roadmap is grounded in current code structure:
- Engine + orchestration: `/Users/harun/Desktop/learn-ai/ranya/pkg/ranya`, `/Users/harun/Desktop/learn-ai/ranya/pkg/pipeline`
- Providers/transports: `/Users/harun/Desktop/learn-ai/ranya/pkg/providers`, `/Users/harun/Desktop/learn-ai/ranya/pkg/transports`
- Processors + turn/tool management: `/Users/harun/Desktop/learn-ai/ranya/pkg/processors`, `/Users/harun/Desktop/learn-ai/ranya/pkg/turn`
- Observability primitives: `/Users/harun/Desktop/learn-ai/ranya/pkg/metrics`, `/Users/harun/Desktop/learn-ai/ranya/pkg/observers`

## Now (0-3 Months)

1. Security and deployment correctness
- Harden Twilio signature verification docs and proxy URL test coverage.
- Better secret/config preflight diagnostics before runtime.

2. Observability upgrades
- OpenTelemetry spans/metrics for pipeline stages.
- Faster debugging flow from artifacts to root cause.

3. Docs + onboarding quality
- More implementation-first tutorials for common task flows.
- Cleaner contribution entry points and issue mapping.

## Next (3-6 Months)

1. Reliability and test depth
- Integration tests for routing and language metadata propagation.
- More deterministic regression coverage for frame processors.

2. Developer experience
- `ranya doctor` style config and environment checks.
- Better local test/docs workflows (make targets + compose docs path).

3. Provider capability transparency
- Matrix showing STT/TTS/LLM/transport feature parity and tradeoffs.

## Later (6-12 Months)

1. Advanced runtime controls
- Policy-driven routing/tool constraints.
- Better failure isolation and graceful degradation patterns.

2. Ecosystem growth
- More production-ready examples by domain.
- Stronger extension points for custom processors and tools.

## Community Contribution Map

We track beginner-friendly and wider-scoped work with:
- `good first issue` for smaller, guided tasks.
- `help wanted` for broader roadmap items.

Current starter issues:
- [#2 good first issue: Add Makefile targets for local test and docs workflows](https://github.com/harunnryd/ranya/issues/2)
- [#3 good first issue: Add provider capability matrix page (EN/ID/JA)](https://github.com/harunnryd/ranya/issues/3)
- [#4 good first issue: Add Coolify docs deployment quickstart page (EN/ID/JA)](https://github.com/harunnryd/ranya/issues/4)

Current roadmap issues:
- [#5 help wanted: Harden Twilio signature verification docs and proxy URL tests](https://github.com/harunnryd/ranya/issues/5)
- [#6 help wanted: Add OpenTelemetry exporter for pipeline spans and metrics](https://github.com/harunnryd/ranya/issues/6)
- [#7 help wanted: Add integration tests for router and language metadata flow](https://github.com/harunnryd/ranya/issues/7)
- [#8 help wanted: Add preflight doctor command for config and env validation](https://github.com/harunnryd/ranya/issues/8)

Watch these labels:
- [good first issue](https://github.com/harunnryd/ranya/labels/good%20first%20issue)
- [help wanted](https://github.com/harunnryd/ranya/labels/help%20wanted)
