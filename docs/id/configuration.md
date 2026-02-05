# Konfigurasi

Konfigurasi dipakai untuk ganti vendor, tuning latensi, dan safety.

## Contoh Minimal
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

## Default yang Paling Berpengaruh
| Key | Default | Kenapa penting |
| --- | --- | --- |
| `pipeline.backpressure` | `drop` | Menjaga latensi stabil saat load. |
| `turn.min_barge_in_ms` | `300` | Seberapa cepat interruption membatalkan speech. |
| `tools.timeout_ms` | `6000` | Mencegah tool call macet. |
| `context.max_history` | `12` | Membatasi pertumbuhan token. |
| `privacy.redact_pii` | `true` | Melindungi artifact secara default. |

## Quick Decision Guide
| Jika butuh | Ubah |
| --- | --- |
| Latensi lebih rendah | kecilkan buffer, tetap `backpressure=drop`. |
| Tanpa loss | set `backpressure=wait` + naikkan kapasitas. |
| Barge‑in lebih cepat | turunkan `turn.min_barge_in_ms`. |
| Tools lebih aman | aktifkan konfirmasi + naikkan timeout. |

## Field Wajib

- `transports.provider`
- `vendors.stt.provider`
- `vendors.tts.provider`
- `vendors.llm.provider`

## Catatan

- Semua string mendukung `${ENV_NAME}`.
- Gunakan provider `mock` untuk lokal.
