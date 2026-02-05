# Turn Management

Turn management controls listening, thinking, speaking, and interruption behavior. Core logic lives in `pkg/processors/manager.go` and `pkg/turn`.

## The Only Knobs That Usually Matter

- `turn.min_barge_in_ms`: how quickly speech cancels playback.
- `turn.end_of_turn_timeout_ms`: force turn end when STT is slow.
- `turn.silence_reprompt`: when and how to reprompt.

## Tuning Recipes
| Goal | Suggested settings |
| --- | --- |
| Aggressive barge‑in | lower `turn.min_barge_in_ms`. |
| Polite agent (no interruptions) | use `turn.PoliteStrategy`. |
| STT doesn’t finalize | set `turn.end_of_turn_timeout_ms`. |
| Silence recovery | enable `turn.silence_reprompt`. |

## Example Config
```yaml
turn:
  barge_in_threshold_ms: 500
  min_barge_in_ms: 300
  end_of_turn_timeout_ms: 1200
  silence_reprompt:
    timeout_ms: 8000
    max_attempts: 2
    prompt_text: "Hello, are you still there?"
    prompt_by_language:
      id: "Halo, apakah Anda masih di line?"
```
