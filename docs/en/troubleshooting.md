# Troubleshooting

## First 10 Minutes

1. Find the `trace_id` in logs.
2. Open the timeline JSONL.
3. Identify the last `frame_out`.
4. Fix the stage that stopped emitting frames.

## Symptoms and Fixes
| Symptom | Likely cause | Fix |
| --- | --- | --- |
| Router never picks an agent | STT frames not final | Ensure `is_final=true` and `source=stt`. |
| Language is never detected | No language detector | Set `EngineOptions.LanguageDetector`. |
| Tools never execute | Tool registry missing | Ensure `EngineOptions.Tools` is set. |
| Tool confirmations repeat | Ambiguous user replies | Use clear yes/no or DTMF `1`/`2`. |
| No barge‑in | Threshold too high | Lower `turn.min_barge_in_ms`. |
| Silence reprompt never fires | Reprompt disabled | Set `turn.silence_reprompt.timeout_ms`. |
| End‑of‑turn slow | STT doesn’t finalize | Set `turn.end_of_turn_timeout_ms`. |
| Frames dropped | Backpressure drop | Use `pipeline.backpressure=wait` or increase buffers. |
