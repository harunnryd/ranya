# Manajemen Giliran

Turn management mengatur listening, thinking, speaking, dan interruption.

## Knob yang Paling Berpengaruh

- `turn.min_barge_in_ms`
- `turn.end_of_turn_timeout_ms`
- `turn.silence_reprompt`

## Resep Tuning
| Target | Setting |
| --- | --- |
| Barge‑in agresif | turunkan `turn.min_barge_in_ms`. |
| Agent sopan | gunakan `turn.PoliteStrategy`. |
| STT lambat final | set `turn.end_of_turn_timeout_ms`. |
| Reprompt silence | aktifkan `turn.silence_reprompt`. |

## Contoh Konfigurasi
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
