# Configuration

Use configuration to swap vendors, tune latency, and enforce safety.

## Minimal Config
```yaml
transports:
  provider: twilio
  settings:
    account_sid: "${TWILIO_ACCOUNT_SID}"
    auth_token: "${TWILIO_AUTH_TOKEN}"
    public_url: "${TWILIO_PUBLIC_URL}"

vendors:
  stt:
    provider: deepgram
    settings:
      api_key: "${DEEPGRAM_API_KEY}"
      model: "nova-2"
  tts:
    provider: elevenlabs
    settings:
      api_key: "${ELEVENLABS_API_KEY}"
      voice_id: "${ELEVENLABS_VOICE_ID}"
  llm:
    provider: openai
    settings:
      api_key: "${OPENAI_API_KEY}"
      model: "gpt-4o-mini"
```

## Defaults That Matter
| Key | Default | Why it matters |
| --- | --- | --- |
| `pipeline.backpressure` | `drop` | Keeps latency stable under load. |
| `turn.min_barge_in_ms` | `300` | How quickly interruptions cancel speech. |
| `tools.timeout_ms` | `6000` | Prevents stuck tool calls. |
| `context.max_history` | `12` | Controls token growth. |
| `privacy.redact_pii` | `true` | Protects artifacts by default. |

## Quick Decision Guide
| If you need | Change |
| --- | --- |
| Lower latency | Reduce buffers, keep `backpressure=drop`. |
| No frame loss | Set `backpressure=wait` + increase capacity. |
| Stronger barge‑in | Lower `turn.min_barge_in_ms`. |
| Safer tools | Enable confirmations and raise timeouts. |

## Required Fields

- `transports.provider`
- `vendors.stt.provider`
- `vendors.tts.provider`
- `vendors.llm.provider`

## Notes

- All string values support `${ENV_NAME}` expansion.
- Use mock providers for local testing.
