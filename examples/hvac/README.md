# HVAC Telephony Voice Agent (Ranya Example)

This example demonstrates a production-style telephony voice agent built on Ranya. It is designed to be LLM-driven, multi‑language (ID/EN), and ready for real call flows with barge‑in, confirmations, recovery prompts, and call-end summaries.

## What It Shows
- Provider-agnostic core with adapters for Twilio (transport), Deepgram (STT), ElevenLabs (TTS), OpenAI (LLM)
- LLM-only routing with **bootstrap mode** to extract global context early and then reduce extra LLM calls
- Tool confirmations (DTMF + LLM), recovery prompts, short-turn response limiter
- Call end reason mapping and call summary events

## Quick Start
1. Set environment variables:
   - `TWILIO_ACCOUNT_SID`
   - `TWILIO_AUTH_TOKEN`
   - `TWILIO_PUBLIC_URL` (public HTTPS domain, e.g. ngrok)
   - `DEEPGRAM_API_KEY`
   - `ELEVENLABS_API_KEY`
   - `OPENAI_API_KEY`
2. Run:
```bash
go run ./examples/hvac --config examples/hvac/config.yaml
```

## Key Config Knobs
- `router.mode`: `off | full | bootstrap`
- `router.max_turns`: number of early turns to run the LLM router
- `confirmation.mode`: `llm | hybrid | keywords`
- `turn.end_of_turn_timeout_ms`: end‑of‑turn timeout tuning
- `observability.artifacts_dir`: write timeline + cost traces

## Production Notes
- Configure `transports.settings.status_callback_path` to receive call end events and map reasons.
- Set `observability.record_audio` only when required and with proper consent.
- Use `languages.overrides` to switch voices per language.

