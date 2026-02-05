# プロバイダー

プロバイダーはSTT/TTS/LLM/Transportのアダプタです。

## 選定チェック

- **レイテンシ**
- **言語対応**
- **信頼性**
- **コスト**

## STT
### Deepgram
Settings: `api_key`, `model`, `language`, `sample_rate`, `encoding`, `interim`, `vad_events`.

### Mock STT
ローカルテスト用。

## TTS
### ElevenLabs
Settings: `api_key`, `voice_id`, `model_id`, `output_format`, `sample_rate`.

### Mock TTS
テスト用。

## LLM
### OpenAI
Settings: `api_key`, `model`, `base_url`, `use_circuit_breaker`, `circuit_threshold`, `circuit_cooldown_ms`.

### Mock LLM
決定論的レスポンス。

## Transport
### Twilio
Settings: `account_sid`, `auth_token`, `public_url`, `voice_path`, `ws_path`, `status_callback_path`.

### Mock Transport
in‑memory transport。

## カスタム追加

1. `pkg/adapters/*` を実装。
2. `ranya.Config` からfactory作成。
3. `ProviderRegistry` に登録。
