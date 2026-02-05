# ルーティングと言語

RoutingはLLM前にエージェントと言語を決めます。

## モードの使い分け
| Mode | 使う場面 |
| --- | --- |
| `off` | 単一エージェント。 |
| `bootstrap` | 最初の数ターンだけ。 |
| `full` | 途中で意図が変わる。 |

## ルーティングの流れ

- 最終STTフレームのみ（`is_final=true`）。
- `RouterStrategy` がagentとグローバルメタデータを返す。

## 言語検出

- 最終STTテキストで検出。
- `languages.code_switching=true` で毎ターン。

## 失敗しやすい点

- `is_final=true` が無い。
- `LanguageDetector` が未設定。

## 最小配線
```go
router := NewLLMRouterStrategy(llmAdapter, nil, LLMRouterConfig{})
opts := ranya.EngineOptions{
  Config:           cfg,
  Router:           router,
  LanguageDetector: myDetector,
  LanguagePrompts:  map[string]string{"id": "...", "en": "..."},
}
app := ranya.NewEngine(opts)
```

## 関連設定
| Key | 意味 |
| --- | --- |
| `router.mode` | `off`, `full`, `bootstrap`. |
| `router.max_turns` | bootstrapのターン数。 |
| `languages.code_switching` | 毎ターン検出。 |
| `languages.default` | 既定言語。 |
