# Party2 Re (仮称)

＠パーティーII を新しい実装として再構築し、OSSとして継続的に発展させるためのプロジェクトです。

このプロジェクトは、既存の＠パーティーIIをそのまま移植・リファクタリングするものではありません。既存ゲームの挙動やゲームデザインを参考にしながら、**実装は新規に構築します**。

## このプロジェクトについて

Party2の大きな特徴の一つは、基本的なゲームシステムの上に、さまざまな機能を追加してゲームを拡張していけることにあります。

新しいParty2では、この特徴をソフトウェア設計にも反映します。

- 新しいゲーム機能を追加しやすいこと
- 既存機能への影響を最小限にできること
- 機能ごとの責務と境界が明確であること
- 将来的にコンポーネント単位で実装技術を変更できること
- 小さな機能を積み重ねながら長期的に開発できること

を重要な設計目標とします。

## アーキテクチャ

初期実装は **Go** で行います。

ただし、プロジェクト全体をGoという言語に固定することは目的としていません。

ゲームを複数のコンポーネントに分け、それぞれについて責務とインターフェースを明確にすることで、将来的に必要となった場合には特定のコンポーネントだけ別の言語へ置き換えられる構造を目指します。

初期段階では複雑なマイクロサービス構成にはせず、**モジュラーモノリス**として開発します。

概念的には、

```text
Core
  │
  ├── Shared Components
  │
  └── Feature Modules
```

という構成を基本とします。

Feature Moduleは新しいゲーム機能を追加するための主要な単位です。

詳しい設計方針については [`docs/architecture/`](docs/architecture/) を参照してください。

## 主なドメイン

現在検討している主なドメインには、以下があります。

- Player / Character
- Progression
- Job / Skill
- Item / Inventory / Equipment
- Currency
- Battle
- Adventure / Quest
- Scheduled Action
- Guild
- Domain Event
- その他のFeature Modules

これらは開発を進めながら詳細化します。

## 開発方針

開発は大きな機能を一度に実装するのではなく、小さな単位で進めます。

```text
設計
  ↓
小さな機能を実装
  ↓
テスト
  ↓
Architecture Review
  ↓
次の機能へ
```

特に新しい機能を追加するときは、

> 同じ種類の機能をもう一つ追加するとしたら、この設計で簡単に追加できるか？

という観点を重視します。

## ライセンス

ソフトウェアのライセンスは現時点では確定していません。

候補として以下を検討しています。

- MIT License
- Apache License 2.0
- GNU Affero General Public License v3.0 (AGPLv3)

最終的なライセンスは、実装途中で利用するライブラリなどのライセンスも確認したうえで決定します。

画像などのクリエイティブアセットについては、Creative Commons系ライセンスを候補とします。

依存ソフトウェアやアセットを追加する際には、ライセンスと出所を確認します。

利用している外部ソフトウェアとライセンスは
[`THIRD_PARTY_LICENSES.md`](THIRD_PARTY_LICENSES.md)で管理します。
READMEには方針と参照先のみを記載し、詳細な依存関係や配布時の注意は
同ファイルに記録します。画像・フォントなどのクリエイティブアセットは
[`docs/assets/`](docs/assets/)で別途管理します。

## ドキュメント

プロジェクトの設計・開発方針は以下にまとめています。

- [`AGENTS.md`](AGENTS.md) — AIエージェントを含む開発者向けの基本方針
- [`ROADMAP.md`](ROADMAP.md) — 開発フェーズと今後の計画
- [`STATUS.md`](STATUS.md) — 現在の状態と決定事項
- [`docs/migration/feature-inventory.md`](docs/migration/feature-inventory.md) — Version 1.0の機能・画像棚卸し
- [`docs/design/game-overview.md`](docs/design/game-overview.md) — ゲームの概要・設計上の理解
- [`docs/architecture/overview.md`](docs/architecture/overview.md) — アーキテクチャ概要
- [`docs/architecture/components.md`](docs/architecture/components.md) — コンポーネント定義
- [`docs/architecture/feature-modules.md`](docs/architecture/feature-modules.md) — Feature Moduleの設計
- [`docs/architecture/interfaces.md`](docs/architecture/interfaces.md) — コンポーネント間のインターフェースと契約
- [`docs/development/development-environment.md`](docs/development/development-environment.md) — コンテナ化された開発環境
- [`docs/development/testing.md`](docs/development/testing.md) — テスト・整形・静的解析の実行方法

## 開発フェーズ

現在は **Version 1.0へ向けた再構築・リファクタリングフェーズ** にあります。

ここでいうリファクタリングは、単に既存コードを整理する作業ではありません。既存Party2を実装上の制約から切り離し、新しいアーキテクチャと実装でVersion 1.0を構築するための移行プロセスです。

このフェーズでのみ必要となる作業と、Version 1.0以降も継続する開発方針は明確に分けて扱います。

### Version 1.0までの一時的な方針

以下は、現在の再構築・リファクタリングフェーズを完了するために必要な対応です。

- 既存Party2の挙動・ルール・コンテンツの調査
- 既存実装からのクリーンな再構築
- 既存コードを使用しない新規実装
- 既存画像などのアセットの再制作
- 既存実装との挙動差異の確認
- Version 1.0に必要な基本機能の再構築
- 旧実装に依存した移行・互換性対応

これらはVersion 1.0の完成に近づくにつれて役割を失うものです。

### Version 1.0以降も継続する方針

以下はリファクタリング完了後もプロジェクトに残る基本方針です。

- Feature拡張を重視した設計
- 小さく明確なコンポーネント境界
- Coreを小さく保つ
- Feature Moduleによる機能追加
- コンポーネントの実装言語を固定しない設計
- TDD
- Issue / PRを中心としたチケットドリブン開発
- Architecture Review
- 依存ソフトウェアとライセンスの管理
- 小さくテスト可能な単位での継続的な開発

詳細は [`STATUS.md`](STATUS.md)、[`ROADMAP.md`](ROADMAP.md)、[`AGENTS.md`](AGENTS.md) を参照してください。

## Origin

このプロジェクトは、Merino氏によって作られた＠パーティーIIを起源とするゲームの再構築プロジェクトです。
