# Examples

## HVAC Telephony Voice Agent
This is the reference implementation. Treat it as the blueprint.

### What to Copy

- `examples/hvac/config.yaml` as your base config.
- `examples/hvac/main.go` for engine wiring.
- `examples/hvac/tools.go` for tool registry.
- `examples/hvac/llm_router.go` for routing.

### How It Maps to the Top Tasks

- **Get a call working**: `main.go` + `config.yaml`.
- **Add tools**: `tools.go`.
- **Add routing**: `llm_router.go`.
- **Enable observability**: `config.yaml` (`observability.artifacts_dir`).

### Run
```bash
go run ./examples/hvac --config examples/hvac/config.yaml
```
