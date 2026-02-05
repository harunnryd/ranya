# ターン管理

ターン管理は listening / thinking / speaking / 割り込み を制御します。

## 重要なノブ

- `turn.min_barge_in_ms`
- `turn.end_of_turn_timeout_ms`
- `turn.silence_reprompt`

## チューニング例
| 目的 | 設定 |
| --- | --- |
| Barge‑in強化 | `turn.min_barge_in_ms` を下げる。 |
| 丁寧な応答 | `turn.PoliteStrategy` を使う。 |
| STTが遅い | `turn.end_of_turn_timeout_ms` を設定。 |
| 沈黙対策 | `turn.silence_reprompt` を有効化。 |

## 設定例
```yaml
turn:
  barge_in_threshold_ms: 500
  min_barge_in_ms: 300
  end_of_turn_timeout_ms: 1200
  silence_reprompt:
    timeout_ms: 8000
    max_attempts: 2
    prompt_text: "Hello, are you still there?"
    prompt_by_language:
      id: "Halo, apakah Anda masih di line?"
```
