# Providers

Providers are pluggable adapters for STT, TTS, LLM, and Transport.

## Selection Checklist

- **Latency**: streaming response time.
- **Language coverage**: target locales.
- **Reliability**: retries + uptime.
- **Cost**: per minute / per token.

## STT
### Deepgram
Settings (from `examples/hvac/main.go`):

- `api_key`, `model`, `language`, `sample_rate`, `encoding`, `interim`, `vad_events`.

### Mock STT
Use for local testing without credentials.

## TTS
### ElevenLabs
Settings:

- `api_key`, `voice_id`, `model_id`, `output_format`, `sample_rate`.

### Mock TTS
Use for deterministic tests.

## LLM
### OpenAI
Settings:

- `api_key`, `model`, `base_url`, `use_circuit_breaker`, `circuit_threshold`, `circuit_cooldown_ms`.

### Mock LLM
Use for deterministic responses.

## Transport
### Twilio
Settings:

- `account_sid`, `auth_token`, `public_url`, `voice_path`, `ws_path`, `status_callback_path`.

### Mock Transport
In‑memory transport for tests.

## Add a Custom Provider

1. Implement the adapter interface in `pkg/adapters/stt`, `pkg/adapters/tts`, or `pkg/llm`.
2. Build a factory from `ranya.Config`.
3. Register it in `ProviderRegistry` before `NewEngine`.
