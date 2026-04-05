# AGENTS.md（vaultsend リポジトリ運用ガイド）

この文書は、vaultsend リポジトリで作業する AI エージェント向けの実務ルールです。実装時は README と `docs/secure-send-mvp-design-ja.md` を一次情報として必ず参照し、ここに記載の方針と矛盾する場合は設計書・既存実装の整合性を優先して差分を最小化してください。

---

## 1. このプロジェクトの概要

- vaultsend は、Secure Send（期限付き・回数制限付きファイル共有）の MVP バックエンドです。
- 現在の主実装は Go API / Worker / Cleanup Worker と DB スキーマ・クエリ群で、メール通知（SES）・キュー（SQS）・オブジェクトストレージ（S3）・課金（Stripe）・組織/RBAC を含みます。
- 主要ドメイン概念:
  - `shipments`（送信単位）
  - `files`, `upload_sessions`（アップロード管理）
  - `access_tokens`（受信アクセス/管理トークン。ハッシュ保存）
  - `notification_events`（通知の監査）
  - `users`, `sessions`（認証）
  - `organizations`, `organization_members`（組織・権限）
  - `subscriptions`（ユーザー/組織課金）
- API 本線は `cmd/api`、非同期通知は `cmd/worker`、データライフサイクル管理は `cmd/cleanup-worker` が担当します。
- 設計の一次ソース:
  1. `docs/secure-send-mvp-design-ja.md`
  2. `README.md`
  3. 実装コード（`internal/`, `db/`）

## 2. 技術スタックと主要ディレクトリ

- バックエンド: Go
  - HTTP: chi (`internal/http`)
  - サービス層: `internal/service`
  - 永続化: PostgreSQL + pgx + sqlc 方針 (`internal/store`, `db/query`, `sqlc.yaml`)
- ストレージ/通知:
  - S3: `internal/storage`
  - SQS: `internal/queue`
  - SES: `internal/mail`
- バッチ/ワーカー:
  - 通知送信: `internal/worker/mail_worker.go`
  - 期限切れ・削除: `internal/worker/cleanup_worker.go`
- エントリポイント:
  - API: `cmd/api/main.go`
  - Worker: `cmd/worker/main.go`
  - Cleanup Worker: `cmd/cleanup-worker/main.go`
- DB:
  - Migration: `db/migrations`
  - Query 定義: `db/query`
- 設定:
  - 環境変数ロード: `internal/config/config.go`
  - 開発コマンド: `Makefile`

> 補足: 設計書では Next.js を想定していますが、現時点で本リポジトリにフロントエンド実装本体は同梱されていません。

## 3. 作業前に確認すべきこと

1. `README.md` と `docs/secure-send-mvp-design-ja.md` を読み、変更対象の仕様を再確認する。
2. 変更箇所の既存責務を確認する（handler / service / store / middleware / worker）。
3. 既存の同等機能を検索し、重複実装を避ける（特に `internal/service` と `internal/store`）。
4. 変更対象の周辺テスト（service, handler, middleware, worker）を先に把握する。
5. 仕様が曖昧な場合は設計書（`docs/secure-send-mvp-design-ja.md`）優先で判断し、必要なら README/Docs を同時更新する。

## 4. 実装ルール（プロジェクト固有）

- **差分最小**: 今回の目的に必要な差分だけを入れる。無関係な rename・format-only 変更は禁止。
- **責務分離**:
  - handler: 入出力（JSON decode/validate, HTTP status 変換）のみに留める。
  - service: ドメインロジック・権限判定・業務バリデーション。
  - store: DB クエリ/トランザクション境界。
  - middleware: 横断関心（認証、rate limit、ヘッダ、request_id）。
  - worker: 非同期処理と再試行制御。
- **DB アクセス方針**:
  - 直接 SQL は `internal/store` に閉じ込め、service/handler から SQL を書かない。
  - `store.ErrNotFound` / `store.ErrConflict` を service で API エラーへ正規化する。
- **migration 追加時**:
  - `db/migrations` に up/down を必ず対で追加する。
  - index/constraint の意図をコメントまたは PR 本文で説明する。
  - 既存データへ破壊的変更を入れる場合はロールバック手順を明示する。
- **config/env 追加時**:
  - `internal/config/config.go` の Load とバリデーションを更新。
  - `README.md` の環境変数一覧・起動手順を更新。
- **worker と API の分離**:
  - メール送信・大量削除など遅延許容処理は API 内同期実行しない。
  - API は enqueue まで、実処理は worker で実施。
- **エラー形式の統一**:
  - HTTP レスポンスは既存の `render.Error` / `render.ServiceError` 形式に合わせる。
- **セキュリティ**:
  - パスワードは bcrypt ハッシュ保存のみ（平文保持禁止）。
  - access token / manage token は生値保存禁止（ハッシュ化して保存）。
  - 認可は常にサーバ側で検証（owner / org role / token validity）。
- **ownership/RBAC**:
  - `owner_user_id` と `organization_id` の判定を壊さない。
  - shipment 操作可否は OrgService の role ルール（member/admin/owner）と整合させる。
- **billing 優先順位**:
  - 組織コンテキストがある場合は org subscription を優先。
  - ない場合は user subscription を参照。
- **副作用処理**:
  - notification_events / cleanup は監査性が重要。状態遷移と再試行可能性（冪等性）を意識して更新する。

## 5. Go 実装規約

- package は既存構成に合わせ、層をまたぐ循環参照を作らない。
- 命名はドメイン語彙優先（shipment, recipient, access token, subscription）。
- コメントは日本語で可。既存コード同様、理由が必要な箇所に限定して簡潔に書く。
- エラーハンドリング:
  - `fmt.Errorf("...: %w", err)` で文脈を付与。
  - 想定業務エラーは `service.APIError` に変換して handler へ返す。
- `context.Context` は request 起点で受け渡し、`context.Background()` を業務処理に持ち込まない（main 初期化やテストを除く）。
- interface は「利用側（service/worker）」で最小定義する（現状の `ShipmentStore`, `BillingStore` 方式を踏襲）。
- テスト方針:
  - 変更した層のテストを優先（service / handler / middleware / worker）。
  - 正常系だけでなく認可・制限超過・競合系を追加。
- モック方針:
  - 既存テスト同様、最小スタブをテストファイル内に定義し過剰抽象化しない。
- 基本運用コマンド:
  - `gofmt -w <files>`
  - `go test ./...`
  - 必要時 `go mod tidy`
- 依存取得制約がある環境で失敗した場合:
  - 失敗コマンド、失敗理由（ネットワーク・権限等）、代替で実施した確認範囲を PR の Testing に明記する。

## 6. フロントエンド規約（該当ディレクトリ追加時）

> 現時点ではフロントエンド実装は本リポジトリ外/未同梱。`frontend` や `web` が追加された場合に適用。

- TypeScript strict を前提に、API 契約（request/response, error code）に追従する。
- 制限判定（plan, auth, org role, download 可否）は UI だけで完結させず、必ずサーバ判定結果を最終ソースにする。
- billing/auth/org/shipment の状態は API レスポンスを正として扱い、クライアント側で独自ルールを増やしすぎない。
- トークンや webhook secret 等の秘密情報をブラウザへ露出しない。

## 7. DB / migration / query のルール

- migration 命名は既存連番（`0000xx_*.up.sql` / `.down.sql`）を踏襲。
- up/down は必ず対称にし、down で戻せる範囲を保証する。
- index/constraint は「どのクエリ・どの整合性のためか」を明確にして追加する。
- query 追加時は `db/query/*.sql` に集約し、`sqlc generate` 前提で命名する。
- 現状は手書き store 実装も併用中のため、sqlc 生成物との責務重複を作らない（移行段階であることを尊重）。
- 本番影響の大きい変更（enum追加、column型変更、大量更新）は段階 migration（expand/contract）を検討する。

## 8. API 変更時のルール

- API 変更時は `README.md` と必要な docs を同時更新する。
- 既存クライアント互換性を意識し、互換を壊す場合は明示する。
- 認証必須/任意をルート単位で再確認し、`RequireAuth` 適用漏れを防ぐ。
- エラー形式は `code/message/request_id` を維持し、plan 制限時は既存の `plan_limit_exceeded` 系形式に合わせる。
- 監査/通知に関わる変更（notification_events, download_events, webhook）は副作用の増減をレビューで明記する。

## 9. worker / 非同期処理のルール

- API 本線で SES 直送信しない。SQS enqueue までを API の責務にする。
- queue payload は後方互換を意識して項目を追加し、既存 worker が decode 不能にならないよう配慮する。
- worker 実装は冪等性を意識する（再配信・重複実行前提）。
- failure/retry 方針:
  - 一時失敗は queue retry に委譲。
  - decode 不可能な poison message は削除してループ停止を回避。
- notification_events と実送信結果（sent/failed）の整合を維持する。
- cleanup は S3 削除失敗時に DB 削除を先行しない（現実装の順序を維持）。

## 10. セキュリティ・運用上の注意

- パスワード平文保存・ログ出力は禁止。
- token 生値保存禁止（`sha256` 等でハッシュ化して保存）。
- Presigned URL の TTL は短命維持（upload: 15分, download: 60秒の既定を尊重）。
- verify/download のレート制限を緩める変更は、根拠と abuse 対策を必ず添える。
- 削除処理は `deleted` 論理状態と cleanup worker の物理削除の二段階を維持。
- Stripe webhook は署名検証を必須にし、失敗時は `invalid_webhook` を返す現行方針を維持。
- owner/org/role チェックは「通す理由」を明確にし、曖昧な OR 条件を導入しない。
- ログに出してはいけない情報:
  - password
  - access/manage token 生値
  - セッション token
  - webhook secret

## 11. テストと検証

- 変更時の最低限:
  1. 変更パッケージの `go test`
  2. 可能なら `go test ./...`
  3. 必要に応じて `go vet ./...`
- テスト観点:
  - unit test: 入力バリデーション、状態遷移、制限値
  - handler test: HTTP status, error format, auth requirement
  - service test: 認可・課金制限・owner/org 判定
  - worker test: retry, poison message, event status 更新
- README にある手動確認手順へ影響する変更は README を更新する。
- `go test ./...` が環境依存で失敗した場合は、PR の Testing に失敗理由と未確認範囲を必ず記載する。

## 12. ドキュメント更新ルール

- 仕様変更時は README / docs をコードと同一 PR で更新する。
- API・環境変数・migration 追加時は README か該当 docs に必ず追記する。
- 未確定仕様は「仮置き」と明記し、確定事項と混在させない。

## 13. PR / コミットルール

- **コミットメッセージは必ず日本語**。
- **PR タイトルは必ず日本語**。
- **PR 本文は必ず日本語**。
- PR 本文には最低限以下を含める:
  - Motivation（なぜ変更が必要か）
  - Description（何をどう変えたか）
  - Testing（実行コマンドと結果）
- テスト未実行・環境制約がある場合は、理由を正直に記載する。
- 何を「今回やっていないか」を必要に応じて明記する。
- レビュー容易性を優先し、PR は機能単位で分割する。
- 大きな設計変更は、実装先行ではなく docs 更新先行（または同時）で進める。

## 14. やってはいけないこと

- 目的外の無関係なリネーム・レイアウト変更。
- 大量整形だけの差分を機能変更 PR に混ぜる。
- `docs/secure-send-mvp-design-ja.md` / 既存実装を読まずに別流儀を持ち込む。
- handler へビジネスロジックを集約する。
- 認可チェック（owner/org/role/token）を省略する。
- password/token/webhook secret の雑な取り扱い（平文保存・ログ出力）。
- UI 側だけで制限を守ろうとし、サーバ側検証を省略する。
- 仕様変更時に README/docs を更新しない。
- 日本語必須のコミット・PR を英語で作成する。
