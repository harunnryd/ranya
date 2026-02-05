# はじめに

素早く起動するための手順です。

## 前提条件

- `go.mod` に記載のGoバージョン（現在 `1.24.13`）。
- 本番プロバイダーを使う場合は各APIキー。

## インストール
```bash
go get github.com/harunnryd/ranya
```

## サンプル実行
```bash
go run ./examples/hvac --config examples/hvac/config.yaml
```

## エンジン配線
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

## 次のステップ

- [タスクフロー](task-flows.md)
- [タスク1: 通話を動かす](task-1-call.md)
- [タスク2: ツール追加](task-2-tools.md)
- [タスク3: ルーティング + 言語](task-3-routing.md)
