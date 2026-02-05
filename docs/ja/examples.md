# サンプル

## HVAC Telephony Voice Agent
参照実装です。これをベースにしてください。

### コピーするもの

- `examples/hvac/config.yaml`
- `examples/hvac/main.go`
- `examples/hvac/tools.go`
- `examples/hvac/llm_router.go`

### 主要タスクとの対応

- **通話を動かす**: `main.go` + `config.yaml`。
- **ツール追加**: `tools.go`。
- **ルーティング追加**: `llm_router.go`。
- **可観測性**: `config.yaml` (`observability.artifacts_dir`)。

### 実行
```bash
go run ./examples/hvac --config examples/hvac/config.yaml
```
