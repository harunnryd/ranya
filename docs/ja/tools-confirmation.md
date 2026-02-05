# ツールと確認

ツールはLLMの外で安全に実行されます。

## 実装手順

1. `llm.Tool` でスキーマ定義。
2. `llm.ToolRegistry` を実装。
3. リスクが高いなら確認を有効化。
4. タイムアウトとリトライを設定。

## 重要なデフォルト

- `tools.timeout_ms`
- `tools.retries`
- `tools.serialize_by_stream`
- `confirmation.mode`

## 例
```go
llm.Tool{
  Name: "schedule_visit",
  Description: "Schedule a technician visit.",
  RequiresConfirmation: true,
  ConfirmationPromptByLanguage: map[string]string{
    "id": "Sebelum saya jadwalkan kunjungan, apakah Anda ingin saya lanjutkan?",
    "en": "Before I schedule the visit, do you want me to proceed?",
  },
  Schema: map[string]any{
    "type": "object",
    "properties": map[string]any{
      "location": map[string]any{"type": "string"},
      "preferred_time": map[string]any{"type": "string"},
    },
    "required": []string{"location", "preferred_time"},
  },
}
```

## 確認

- DTMF: `1` = yes, `2` = no。
- キーワードは英語/インドネシア語対応。
- 曖昧な場合のみ `confirmation.llm_fallback` を有効化。
