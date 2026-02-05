# Observability

Use observability to find the exact stage where a call fails.

## What You Get

- Timeline JSONL per call.
- Cost summary per call.
- Per‑stage latency metrics.

## Recommended Setup

- Dev: enable artifacts + audio recording.
- Prod: enable artifacts, keep audio off unless policy allows.

## Debug Workflow

1. Find the `trace_id` in logs.
2. Open the timeline JSONL.
3. Locate the last `frame_out` event.
4. Fix the stage that stopped emitting frames.

## Key Config
| Key | Meaning |
| --- | --- |
| `observability.artifacts_dir` | Where timeline + cost files go. |
| `observability.record_audio` | Include base64 audio payloads. |
| `observability.retention_days` | Delete old artifacts at startup. |
