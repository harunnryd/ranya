# Why Ranya

Use this page to decide quickly if Ranya is the right framework for your project.

## Use Ranya If You Need

- Real‑time telephony with **barge‑in** and interruption handling.
- A **deterministic pipeline** you can debug with frame traces.
- **Tool execution safety** (confirmation, retries, idempotency).
- **Vendor portability** across STT/TTS/LLM/Transport.

## Rethink If

- You only need a simple chatbot without streaming.
- You do not need tool safety or observability.
- You will never swap providers.

## Tradeoffs (Good to Know)

- Determinism favors predictable latency over loose, dynamic flows.
- The pipeline model is strict: processors emit frames, not side‑effects.
- You will configure more upfront to gain safer production behavior.

## Fastest Proof‑of‑Value

- Run the HVAC example.  
  [Examples](examples.md)

- Follow the minimal wiring path.  
  [Start Here](start-here.md)
