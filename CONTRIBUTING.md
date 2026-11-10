# コントリビューションガイド

Party2 Re へのコントリビューションを歓迎します。Issue・PR は日本語・英語のどちらで書いても構いません。

> **English summary**: Read the documents listed below before you start, and always use the Issue and PR templates. Development rules assume AI coding agents and bind humans equally. Contributions are accepted under the MIT License until the project license is decided. Do not copy original Party2 source code or assets. Issues and PRs may be written in Japanese or English.

## はじめに読むドキュメント

作業を始める前に、人間も AI エージェントも次の順に読んでください。

1. [`AGENTS.md`](AGENTS.md) と、そこで作業内容に応じて指定される [`.agents/rules/`](.agents/rules/) のルール
2. [`docs/README.md`](docs/README.md) — どの情報がどのドキュメントにあるか
3. [`docs/development/development-environment.md`](docs/development/development-environment.md) — 開発環境の構築例

## テンプレートは必須です

- Issue は必ず [`.github/ISSUE_TEMPLATE/`](.github/ISSUE_TEMPLATE/) のテンプレートで作成してください。
- PR は必ず [`.github/PULL_REQUEST_TEMPLATE.md`](.github/PULL_REQUEST_TEMPLATE.md) を使い、チェック項目は実際に確認したものだけにチェックを入れてください。

Issue・PR の運用ルールは [開発ワークフロー](.agents/rules/01-development-workflow.md#2-issue-and-pr-workflow) が正本です。

## AI エージェントとの開発

開発ルールは AI コーディングエージェントの利用を前提に整備しており、人間のコントリビューターにも同じルールが適用されます。AI の支援を受けた PR も歓迎しますが、内容の責任は提出者が負います。

たとえば AI エージェントに「`AGENTS.md`・`docs/README.md`・`docs/development/development-environment.md` を読んで開発環境を構築して」「テンプレートに沿って Issue（PR）を作成して」と依頼できます。

## ライセンス

プロジェクトのライセンスはまだ確定していません（候補は [README](README.md#ライセンス) を参照）。確定するまでの間、コントリビューションは **MIT License** の下で提供されるものとします。MIT であれば、候補のどれが最終的なライセンスになっても取り込み済みのコントリビューションをそのまま利用できます。

## クリーンルーム

オリジナルの Party2 のソースコードやアセットをコピーしないでください。挙動やゲームデザインは参考にしますが、実装は新規に行います（[README](README.md)、[移行時の制約](.agents/rules/00-migration-constraints.md)）。
