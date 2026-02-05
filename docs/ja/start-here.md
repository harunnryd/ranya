# はじめに

最短で動くボイスエージェントを作る手順です。

詳細な手順は [タスクフロー](task-flows.md) を参照してください。

## 1. スタックを決める
各役割で1つ選びます。後で入れ替え可能です。

- **Transport**: Twilio（本番）または Mock（ローカル）。
- **STT**: Deepgram（本番）または Mock（ローカル）。
- **TTS**: ElevenLabs（本番）または Mock（ローカル）。
- **LLM**: OpenAI（本番）または Mock（ローカル）。

## 2. 参照サンプルを実行
HVACサンプルは routing、確認、リカバリ、サマリまで含む完全版です。

```bash
go run ./examples/hvac --config examples/hvac/config.yaml
```

## 3. 最小エンジン配線
```go
package main

import (
  "context"
  "log"
  "os"
  "os/signal"

  "github.com/harunnryd/ranya/pkg/ranya"
)

func main() {
  cfg, err := ranya.LoadConfig("config.yaml")
  if err != nil {
    log.Fatal(err)
  }

  engine := ranya.NewEngine(ranya.EngineOptions{Config: cfg})

  ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
  defer stop()

  if err := engine.Start(ctx); err != nil {
    log.Fatal(err)
  }

  <-ctx.Done()
  _ = engine.Stop()
}
```

## 4. 最小限の追加

- **ツール**: スキーマとハンドラを定義。  
  [ツールと確認](tools-confirmation.md)

- **ルーティング**: エージェントと言語を決定。  
  [ルーティングと言語](routing.md)

- **観測**: アーティファクトを有効化。  
  [可観測性](observability.md)

## 5. 本番条件で検証

- `turn.min_barge_in_ms` を調整。
- `pipeline.backpressure` を調整。
- `privacy.redact_pii=true` を基本に。

## 完了条件

- 通話が end‑to‑end で動く。
- タイムラインで原因特定できる。
- プロバイダーをコード変更なしで交換できる。

## 次に読む

- [設定](configuration.md)
- [アーキテクチャ](architecture.md)
- [モジュール](modules.md)
