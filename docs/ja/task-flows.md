# タスクフロー

Ranya導入時に最初にやることを4つに絞りました。

## タスク1: 通話を動かす（Twilio + Provider）

1. Transport/STT/TTS/LLM のプロバイダーを選ぶ。
2. `examples/hvac/config.yaml` をベースに環境変数を設定。
3. HVACサンプルを実行。
4. 通話が end‑to‑end で完了することを確認。
5. 失敗時はタイムラインで最後のステージを特定。

<div class="r-quick-links" markdown>
Related:

- [タスク1: 通話を動かす](task-1-call.md)
- [はじめに](start-here.md)
- [設定](configuration.md)
- [プロバイダー](providers.md)
- [サンプル](examples.md)
- [可観測性](observability.md)
</div>

## タスク2: ツール追加（業務アクション）

1. `llm.Tool` でスキーマを定義。
2. `llm.ToolRegistry` をアプリで実装。
3. リスクがあるなら確認を有効化。
4. タイムアウトとリトライを設定。
5. `tool_call` と `tool_result` のフレームを確認。

<div class="r-quick-links" markdown>
Related:

- [タスク2: ツール追加](task-2-tools.md)
- [ツールと確認](tools-confirmation.md)
- [モジュール](modules.md)
- [サンプル](examples.md)
</div>

## タスク3: ルーティングと言語

1. `router.mode` を選ぶ（`off`/`bootstrap`/`full`）。
2. `RouterStrategy` を設定（LLM router or custom）。
3. 多言語なら `LanguageDetector` を追加。
4. STT final に `is_final=true` を確認。

<div class="r-quick-links" markdown>
Related:

- [タスク3: ルーティング + 言語](task-3-routing.md)
- [ルーティングと言語](routing.md)
- [フレームとメタデータ](frames.md)
- [設定](configuration.md)
</div>

## タスク4: 可観測性とデバッグを有効化

1. `observability.artifacts_dir` を書き込み可能なフォルダに設定。
2. JSONログで `trace_id` を素早く検索。
3. JSONLタイムラインで最後の `frame_out` を確認。
4. コストとレイテンシのイベントで性能を検証。

<div class="r-quick-links" markdown>
Related:

- [タスク4: 可観測性](task-4-observability.md)
- [可観測性](observability.md)
- [トラブルシューティング](troubleshooting.md)
</div>
