# Concepts Overview

This is the mental model you need to build and debug Ranya fast.

## 1. Frames Are the Contract
Every stage reads a frame and emits frames. This makes debugging deterministic.

- `audio`, `text`, `control`, `system`, `image` are the only kinds.
- If a processor doesn't emit a frame, the pipeline stalls there.

## 2. The Pipeline Is One‑Way
Frames flow forward through processors. Control frames get priority.

- Latency stays stable under load.
- Interruptions are handled by `flush` and `cancel` signals.

## 3. Turn Manager Owns State
Listening, thinking, speaking are explicit states.

- Do not encode turn logic into prompts.
- Tune `turn.*` before changing models.

## 4. Routing Happens Before LLM
Routing is only on **final STT text**.

- If routing fails, check `is_final=true` first.
- Use `bootstrap` to route only early turns.

## 5. Tools Run Outside the LLM
Tools are executed by a dispatcher for safety.

- Retries and timeouts are enforced.
- Confirmation is explicit.

## 6. Observability Is Your Debugger
Artifacts show the frame timeline, latency, and costs.

- Trace IDs let you replay a call.
- Use the last `frame_out` to find failure points.

## Where to Extend

- **Before LLM**: add normalization or prompt injection.
- **Before TTS**: add formatting, truncation, or translations.
- **Post‑processors**: add logging or serialization.

## Deep Dives

- Frame contract and metadata: [Frames and Metadata](frames.md)
- Pipeline behavior and backpressure: [Pipeline and Backpressure](pipeline.md)
- Turn state and barge‑in: [Turn Management](turn-management.md)
- Routing and language: [Routing and Language](routing.md)
- Tools and confirmation: [Tools and Confirmation](tools-confirmation.md)
- Observability and artifacts: [Observability](observability.md)
