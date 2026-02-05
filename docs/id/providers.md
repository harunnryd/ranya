# Provider

Provider adalah adapter pluggable untuk STT, TTS, LLM, dan Transport.

## Checklist Pilih Provider

- **Latensi**
- **Cakupan bahasa**
- **Reliabilitas**
- **Biaya**

## STT
### Deepgram
Settings: `api_key`, `model`, `language`, `sample_rate`, `encoding`, `interim`, `vad_events`.

### Mock STT
Untuk uji lokal tanpa kredensial.

## TTS
### ElevenLabs
Settings: `api_key`, `voice_id`, `model_id`, `output_format`, `sample_rate`.

### Mock TTS
Untuk test deterministik.

## LLM
### OpenAI
Settings: `api_key`, `model`, `base_url`, `use_circuit_breaker`, `circuit_threshold`, `circuit_cooldown_ms`.

### Mock LLM
Untuk response deterministik.

## Transport
### Twilio
Settings: `account_sid`, `auth_token`, `public_url`, `voice_path`, `ws_path`, `status_callback_path`.

### Mock Transport
In‑memory transport untuk test.

## Tambah Provider Baru

1. Implement adapter di `pkg/adapters/*`.
2. Buat factory dari `ranya.Config`.
3. Register di `ProviderRegistry`.
