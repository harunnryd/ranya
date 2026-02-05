# Contoh

## HVAC Telephony Voice Agent
Ini referensi utama. Anggap sebagai blueprint.

### Yang Perlu Disalin

- `examples/hvac/config.yaml` sebagai base config.
- `examples/hvac/main.go` untuk wiring engine.
- `examples/hvac/tools.go` untuk tool registry.
- `examples/hvac/llm_router.go` untuk routing.

### Mapping ke Tugas Utama

- **Call jalan**: `main.go` + `config.yaml`.
- **Tambah tools**: `tools.go`.
- **Tambah routing**: `llm_router.go`.
- **Observabilitas**: `config.yaml` (`observability.artifacts_dir`).

### Jalankan
```bash
go run ./examples/hvac --config examples/hvac/config.yaml
```
