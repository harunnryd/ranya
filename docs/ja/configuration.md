# 設定

設定でベンダー切替、レイテンシ調整、安全性制御を行います。

## 最小構成例
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

## 影響が大きいデフォルト
| Key | Default | 理由 |
| --- | --- | --- |
| `pipeline.backpressure` | `drop` | 低レイテンシ。 |
| `turn.min_barge_in_ms` | `300` | 割り込み速度。 |
| `tools.timeout_ms` | `6000` | ツール停止防止。 |
| `context.max_history` | `12` | トークン増加抑制。 |
| `privacy.redact_pii` | `true` | 既定で保護。 |

## クイック判断
| 目的 | 変更 |
| --- | --- |
| 低レイテンシ | `backpressure=drop` + バッファ小。 |
| Lossless | `backpressure=wait` + バッファ大。 |
| Barge‑in強化 | `turn.min_barge_in_ms` を下げる。 |
| ツール安全性 | 確認を有効化 + timeout増。 |

## 必須フィールド

- `transports.provider`
- `vendors.stt.provider`
- `vendors.tts.provider`
- `vendors.llm.provider`

## メモ

- 文字列は `${ENV_NAME}` を展開。
- ローカルは mock provider を推奨。
