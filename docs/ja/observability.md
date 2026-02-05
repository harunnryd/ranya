# 可観測性

可観測性は「どのステージで止まったか」を特定するために使います。

## 得られるもの

- 通話ごとのタイムラインJSONL。
- コストサマリ。
- ステージ別レイテンシ。

## 推奨設定

- Dev: アーティファクト + 音声記録。
- Prod: アーティファクトのみ（音声はポリシー次第）。

## デバッグ手順

1. `trace_id` をログから取得。
2. タイムラインJSONLを開く。
3. 最後の `frame_out` を探す。
4. そのステージを修正。

## Key Config
| Key | 意味 |
| --- | --- |
| `observability.artifacts_dir` | 出力先。 |
| `observability.record_audio` | 音声ペイロード含む。 |
| `observability.retention_days` | 起動時の削除。 |
