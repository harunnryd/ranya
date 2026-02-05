# 概念の概要

ビルドとデバッグのための最小メンタルモデルです。

## 1. Frameは契約
各ステージはFrameを読み、Frameを出力します。

- `audio`, `text`, `control`, `system`, `image`。
- 出力が止まるとそのステージが原因。

## 2. パイプラインは一方向
Frameは前に流れ、制御Frameが優先されます。

## 3. Turn Managerが状態を持つ
Listening/Thinking/Speaking を明示的に管理。

## 4. ルーティングはLLM前
最終STTテキストのみで動きます。

## 5. ツールはLLM外
安全性（タイムアウト、リトライ、確認）を担保。

## 6. 可観測性はデバッガ
タイムラインで停止地点を特定。

## 拡張ポイント

- **Before LLM**: 正規化、プロンプト注入。
- **Before TTS**: 整形、短縮。
- **Post‑processor**: ログ、分析。

## 深掘り

- [Frames and Metadata](frames.md)
- [Pipeline and Backpressure](pipeline.md)
- [Turn Management](turn-management.md)
- [Routing and Language](routing.md)
- [Tools and Confirmation](tools-confirmation.md)
- [Observability](observability.md)
