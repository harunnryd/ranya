# トラブルシューティング

## 最初の10分

1. ログで `trace_id` を探す。
2. タイムラインJSONLを開く。
3. 最後の `frame_out` を確認。
4. そのステージを修正。

## 症状と対処
| 症状 | 原因 | 対処 |
| --- | --- | --- |
| Routerが動かない | STTがfinalでない | `is_final=true` と `source=stt` を確認。 |
| 言語が検出されない | LanguageDetector未設定 | `EngineOptions.LanguageDetector` を設定。 |
| ツールが実行されない | Tool registry未設定 | `EngineOptions.Tools` を設定。 |
| 確認が繰り返される | 返答が曖昧 | 明確な yes/no or DTMF `1`/`2`。 |
| Barge‑inが効かない | 閾値が高い | `turn.min_barge_in_ms` を下げる。 |
| Silence repromptが動かない | 無効設定 | `turn.silence_reprompt.timeout_ms` を設定。 |
| End‑of‑turnが遅い | STT finalが遅い | `turn.end_of_turn_timeout_ms` を設定。 |
| Frameが落ちる | Backpressure drop | `pipeline.backpressure=wait` またはバッファ増。 |
