# Task Flows

These are the first four tasks most engineers complete when adopting Ranya.

## Task 1: Get a Call Working (Twilio + Providers)

1. Pick providers for Transport, STT, TTS, and LLM.
2. Set environment variables and start from `examples/hvac/config.yaml`.
3. Run the reference example.
4. Confirm a full call completes end-to-end.
5. If it fails, use the timeline artifacts to find the last stage.

<div class="r-quick-links" markdown>
Related:

- [Task 1: Get a Call Working](task-1-call.md)
- [Start Here](start-here.md)
- [Configuration](configuration.md)
- [Providers](providers.md)
- [Examples](examples.md)
- [Observability](observability.md)
</div>

## Task 2: Add Tools (Business Actions)

1. Define tool schemas with `llm.Tool`.
2. Implement `llm.ToolRegistry` in your app.
3. Enable confirmations for risky actions.
4. Set timeouts and retries.
5. Verify `tool_call` and `tool_result` frames appear.

<div class="r-quick-links" markdown>
Related:

- [Task 2: Add Tools](task-2-tools.md)
- [Tools and Confirmation](tools-confirmation.md)
- [Modules](modules.md)
- [Examples](examples.md)
</div>

## Task 3: Add Routing and Language

1. Choose `router.mode` (`off`, `bootstrap`, `full`).
2. Wire a `RouterStrategy` (LLM router or custom).
3. Add a `LanguageDetector` if you need multilingual routing.
4. Ensure STT final frames include `is_final=true`.

<div class="r-quick-links" markdown>
Related:

- [Task 3: Add Routing + Language](task-3-routing.md)
- [Routing and Language](routing.md)
- [Frames and Metadata](frames.md)
- [Configuration](configuration.md)
</div>

## Task 4: Enable Observability and Debugging

1. Set `observability.artifacts_dir` to a writable folder.
2. Use JSON logs for fast `trace_id` search.
3. Open the timeline JSONL and locate the last `frame_out`.
4. Use cost and latency events to validate performance.

<div class="r-quick-links" markdown>
Related:

- [Task 4: Enable Observability](task-4-observability.md)
- [Observability](observability.md)
- [Troubleshooting](troubleshooting.md)
</div>
