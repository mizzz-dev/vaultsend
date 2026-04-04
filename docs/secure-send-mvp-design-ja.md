# SecureSend（仮称）MVP 具体設計（実装着手版）

この文書は「読める設計」から「そのまま実装できる設計」へ昇格させるための確定版です。未確定事項はすべて「仮置き」で明示し、MVPスコープを固定します。

---

## 1. 要件の不足・曖昧点の洗い出し

以下は現行設計で実装時に詰まる曖昧点です。全項目に対して曖昧点を明示します。

### 1-1. 匿名送信 vs ログイン送信
- 曖昧点:
  - 匿名送信で「送信後の削除・再送」を誰がどう認証して実行するか未定義。
  - 匿名送信の履歴閲覧期間（即時のみ / 24時間 / 期限まで）が未定義。
  - ログイン送信と匿名送信で機能差（受信者数・容量上限）をつけるか未定義。

### 1-2. 受信者限定アクセス認証方式
- 曖昧点:
  - メールリンクのみで許可するか、追加で認証コード（OTP）を必須にするか未定義。
  - 同一リンクの転送時に、受信者本人性をどこまで担保するか未定義（メール一致確認のみ/追加コード）。

### 1-3. トークン設計（ワンタイム / 再利用 / 有効期限）
- 曖昧点:
  - アクセストークンをワンタイムにするか再利用可能にするか未定義。
  - ダウンロードURL（Presigned URL）発行時にダウンロード回数を消費するか、実ダウンロード完了時に消費するか未定義。
  - 匿名管理トークンの失効条件（手動失効のみ/初回使用後失効/期限失効）未定義。

### 1-4. ファイルサイズ制限
- 曖昧点:
  - 1ファイル上限・1配送（shipment）合計上限が確定していない。
  - ブラウザ側制約（モバイル・低メモリ）を考慮した上限値が未確定。

### 1-5. 保持期間と削除ポリシー
- 曖昧点:
  - 期限切れ後、実ファイル削除を即時にするか遅延バッチにするか未定義。
  - 論理削除データ（監査/イベント）の保持期間がテーブル単位で確定していない。

### 1-6. 誤送信・削除・再送仕様
- 曖昧点:
  - 送信後のファイル差し替え可否が未定（MVPでやるか未定）。
  - 誤送信時の「即時失効（revoke）」と「論理削除」の優先仕様が未定義。

### 1-7. ダウンロード制御（回数・期限）
- 曖昧点:
  - 回数制限をshipment単位かrecipient単位か未定。
  - 複数ファイルダウンロード時の回数消費ルール（ファイル単位/セッション単位）未定。

### 1-8. 不正利用対策（最低限）
- 曖昧点:
  - 匿名送信のレート制限閾値が未確定。
  - CAPTCHA導入条件（常時/閾値超過時のみ）未定。
  - 禁止ファイル拡張子・MIMEのブロックポリシーが未確定。

---

## 2. 前提 assumptions（仮置き）と確認事項

以降は実装着手のための仮置き確定値です。MVP中はこの値で固定します。

### 2-1. 仮置き（確定値として実装）
- 最大ファイルサイズ: **10GB / file**
- ファイル数上限: **20 files / shipment**
- 受信者数上限: **20 recipients / shipment**
- DL回数制限（初期値）: **10回 / shipment**
- 有効期限（初期値）: **7日**（設定範囲: 1日〜14日）
- 削除タイミング:
  - 期限到達時に`expired`化（即時）
  - オブジェクト本体は期限到達後 **24時間以内のバッチ削除**
  - 論理削除メタデータは90日保持
- 匿名送信の扱い: **許可**（ただし厳格レート制限 + 管理トークン必須）
- トークン方式（推奨案）:
  - 受信リンクトークンは**再利用可能（期限内）**
  - 実ファイルDLは都度`/download-url`で短命URL（60秒）発行
  - 匿名管理トークンは長寿命（shipment期限 + 7日）

### 2-2. 推奨案と代替案

#### A. 匿名送信
- 推奨案: 許可（ただし匿名は「送信一覧永続履歴なし」、管理トークンで操作）
- 代替案: 匿名送信禁止（ログイン必須）

#### B. 受信者認証
- 推奨案: メールリンク + 任意OTP（高リスク時のみ必須化）
- 代替案: メールリンクのみ（実装容易だが転送耐性が低い）

#### C. トークン
- 推奨案: 長寿命アクセス・短寿命Presigned URLの2段階
- 代替案: ワンタイムトークンのみ（セキュアだが再アクセスUX悪化）

#### D. DL回数消費
- 推奨案: Presigned URL発行成功時に1消費（実装シンプル）
- 代替案: ダウンロード完了Webhookで消費（精度高いが複雑）

### 2-3. リリース前に最終確認が必要な項目
1. 企業顧客向け保持期間（監査ログ180日で足りるか）
2. 匿名送信のCaptcha閾値
3. 1ファイル10GB上限が営業要件と一致するか
4. 受信者本人確認の厳格化条件（OTP強制ルール）

---

## 3. 推奨アーキテクチャ（最終確定版）

### 3-1. 構成
- フロント: **Next.js（App Router, TypeScript）**
- API: **Go（chi + sqlc + pgx）**
- DB: **PostgreSQL 16**
- ストレージ: **S3互換（AWS S3前提）**
- アップロード: **Multipart + Presigned URL**
- メール: **SES API**
- 非同期処理: **SQS + worker（Go）**
- 監視: **OpenTelemetry + CloudWatch + Sentry**

### 3-2. なぜこの構成か
- Next.js: 送信者UI/受信者UIを同一基盤で高速開発できる。
- Go API: I/O中心ワークロード（署名URL、DB更新、メールキュー投入）で低レイテンシ・高安定。
- PostgreSQL: 状態遷移・整合性（トランザクション、ユニーク制約）を確実に管理。
- S3直アップロード: APIサーバ帯域を圧迫せず大容量に対応。
- SQS分離: メール失敗や削除遅延をAPI本線から切り離し可用性を上げる。

### 3-3. Presigned URL運用ルール
- Upload URL TTL: **15分**
- Download URL TTL: **60秒**
- ダウンロードURLは毎回認可判定後に発行。
- URL自体はログに平文保存しない（監査には`request_id`のみ記録）。

### 3-4. バッチ処理
- `expire_shipments`（5分おき）: `sent/accessed`で期限切れを`expired`へ。
- `purge_objects`（15分おき）: `expired`/`deleted`で削除予定到達分のS3削除。
- `retry_emails`（1分おき）: 失敗メール再送（最大3回、指数バックオフ）。

### 3-5. 監視
- SLI:
  - upload complete率
  - download URL発行成功率
  - email送信成功率
  - API p95 latency
- アラート:
  - 5xx > 1%（5分）
  - purge backlog > 1000件
  - email failure > 5%（15分）

---

## 4. データモデル（ER図レベル）

型はPostgreSQL前提。IDは`uuid`（gen_random_uuid）で統一。

### 4-1. shipments
- カラム
  - `id uuid pk`
  - `owner_type text not null check (owner_type in ('anonymous','user'))`
  - `owner_user_id uuid null`
  - `status text not null check (status in ('draft','uploading','ready','sent','accessed','expired','deleted','revoked'))`
  - `share_mode text not null check (share_mode in ('public_link','recipient_restricted'))`
  - `title varchar(200) not null`
  - `message text null`
  - `max_downloads int not null default 10 check (max_downloads between 1 and 100)`
  - `current_downloads int not null default 0`
  - `expires_at timestamptz not null`
  - `sent_at timestamptz null`
  - `revoked_at timestamptz null`
  - `deleted_at timestamptz null`
  - `created_at timestamptz not null default now()`
  - `updated_at timestamptz not null default now()`
- インデックス
  - `(owner_user_id, created_at desc)`
  - `(status, expires_at)`
  - partial: `(deleted_at) where deleted_at is null`

### 4-2. files
- カラム
  - `id uuid pk`
  - `shipment_id uuid not null fk shipments(id)`
  - `original_name varchar(255) not null`
  - `size_bytes bigint not null check (size_bytes > 0)`
  - `mime_type varchar(120) not null`
  - `storage_bucket varchar(63) not null`
  - `storage_key varchar(1024) not null unique`
  - `checksum_sha256 char(64) not null`
  - `upload_status text not null check (upload_status in ('initiated','parts_uploaded','completed','failed'))`
  - `created_at timestamptz not null default now()`
- インデックス
  - `(shipment_id)`
  - `(upload_status)`

### 4-3. recipients
- カラム
  - `id uuid pk`
  - `shipment_id uuid not null fk shipments(id)`
  - `email varchar(320) not null`
  - `email_normalized varchar(320) not null`
  - `status text not null check (status in ('pending','notified','verified','downloaded','blocked'))`
  - `verify_code_hash varchar(255) null`
  - `created_at timestamptz not null default now()`
  - `updated_at timestamptz not null default now()`
- 制約・インデックス
  - unique `(shipment_id, email_normalized)`
  - `(shipment_id, status)`

### 4-4. access_tokens
- カラム
  - `id uuid pk`
  - `shipment_id uuid not null fk shipments(id)`
  - `recipient_id uuid null fk recipients(id)`
  - `token_type text not null check (token_type in ('download_access','manage','otp_verify'))`
  - `token_hash char(64) not null`
  - `expires_at timestamptz not null`
  - `max_uses int not null default 1`
  - `used_count int not null default 0`
  - `revoked_at timestamptz null`
  - `created_at timestamptz not null default now()`
- 制約・インデックス
  - unique `(token_hash)`
  - `(shipment_id, token_type, expires_at)`
  - `(recipient_id, token_type)`

### 4-5. download_events
- カラム
  - `id bigserial pk`
  - `shipment_id uuid not null fk shipments(id)`
  - `file_id uuid not null fk files(id)`
  - `recipient_id uuid null fk recipients(id)`
  - `result text not null check (result in ('success','expired','over_limit','invalid_token','forbidden'))`
  - `ip_hash char(64) not null`
  - `user_agent text null`
  - `created_at timestamptz not null default now()`
- インデックス
  - `(shipment_id, created_at desc)`
  - `(recipient_id, created_at desc)`

### 4-6. audit_logs
- カラム
  - `id bigserial pk`
  - `actor_type text not null check (actor_type in ('user','anonymous','system','admin'))`
  - `actor_id varchar(64) null`
  - `action varchar(64) not null`
  - `resource_type varchar(64) not null`
  - `resource_id varchar(64) not null`
  - `metadata jsonb not null default '{}'::jsonb`
  - `created_at timestamptz not null default now()`
- インデックス
  - `(resource_type, resource_id, created_at desc)`
  - `(actor_type, actor_id, created_at desc)`

### 4-7. upload_sessions
- カラム
  - `id uuid pk`
  - `shipment_id uuid null fk shipments(id)`
  - `file_id uuid null fk files(id)`
  - `storage_bucket varchar(63) not null`
  - `storage_key varchar(1024) not null`
  - `multipart_upload_id varchar(255) not null`
  - `part_size_bytes int not null`
  - `status text not null check (status in ('initiated','uploading','completed','aborted'))`
  - `expires_at timestamptz not null`
  - `created_at timestamptz not null default now()`
- インデックス
  - `(status, expires_at)`
  - unique `(multipart_upload_id)`

### 4-8. リレーション
- shipments 1 - N files
- shipments 1 - N recipients
- shipments 1 - N access_tokens
- shipments 1 - N download_events
- shipments 1 - N audit_logs（resource_id参照）
- files 1 - 1 upload_sessions（MVPは1ファイル1セッション）

---

## 5. 状態遷移設計（shipment）

### 5-1. 状態定義
- `draft`: shipment器のみ作成
- `uploading`: upload session進行中
- `ready`: 全ファイルアップロード完了、送信確定前
- `sent`: 送信通知済み・共有可能
- `accessed`: 1件以上ダウンロード発生
- `expired`: 期限切れ
- `deleted`: 送信者/管理者削除
- `revoked`: 誤送信等で即時無効化

### 5-2. 状態遷移図（テキスト）
- `draft -> uploading -> ready -> sent -> accessed`
- `sent -> expired`
- `accessed -> expired`
- `draft|uploading|ready|sent|accessed -> deleted`
- `sent|accessed -> revoked`
- `revoked -> deleted`（バッチで整理）

### 5-3. APIと状態遷移の対応
- `POST /v1/uploads` : `draft -> uploading`（shipment未作成時は内部でdraft作成可）
- `POST /v1/uploads/{id}/complete` : 対象ファイル完了、全完了で`uploading -> ready`
- `POST /v1/shipments` : `ready -> sent`
- `GET /v1/files/{id}/download-url` : 初回成功時`sent -> accessed`
- バッチ `expire_shipments` : `sent|accessed -> expired`
- `DELETE /v1/shipments/{id}` : `* -> deleted`
- `POST /v1/shipments/{id}/resend` : 状態維持（`sent|accessed`）

---

## 6. API設計（OpenAPIレベル）

### 共通
- Base: `/v1`
- 認証:
  - 送信者API: `Authorization: Bearer <user_jwt | manage_token>`
  - 受信者API: パスパラメータ`{token}` + 必要時OTP
- エラー形式:
```json
{ "code": "string", "message": "string", "request_id": "string" }
```

### 6-1. `POST /v1/uploads`
- 目的: multipart upload session開始
- 認証: 必須（ログイン or 匿名manage tokenなしの場合はanonymous session cookie）
- request
```json
{
  "shipment_id": "uuid|null",
  "file_name": "report.pdf",
  "size_bytes": 1048576,
  "mime_type": "application/pdf",
  "checksum_sha256": "hex",
  "part_count": 10
}
```
- response 201
```json
{
  "upload_session_id": "uuid",
  "shipment_id": "uuid",
  "file_id": "uuid",
  "part_size_bytes": 8388608,
  "presigned_parts": [{"part_number":1,"url":"https://..."}],
  "expires_at": "2026-04-11T00:00:00Z"
}
```
- エラー
  - `400 invalid_file_size`
  - `409 upload_session_exists`
  - `413 payload_too_large`

### 6-2. `POST /v1/uploads/{id}/complete`
- 目的: multipart complete
- 認証: 必須
- request
```json
{ "parts": [{"part_number":1,"etag":"\"abc\""}] }
```
- response 200
```json
{ "file_id":"uuid", "upload_status":"completed", "shipment_status":"ready" }
```
- エラー
  - `400 invalid_parts`
  - `409 upload_not_in_progress`

### 6-3. `POST /v1/shipments`
- 目的: 送信確定 + 受信者通知キュー投入
- 認証: 必須
- request
```json
{
  "shipment_id":"uuid",
  "title":"請求書",
  "message":"4月分です",
  "share_mode":"recipient_restricted",
  "recipient_emails":["a@example.com"],
  "expires_in_days":7,
  "max_downloads":10
}
```
- response 201
```json
{
  "id":"uuid",
  "status":"sent",
  "access_url":"https://app.example.com/r/xxxx",
  "expires_at":"2026-04-11T00:00:00Z"
}
```
- エラー
  - `400 invalid_recipients`
  - `409 shipment_not_ready`

### 6-4. `GET /v1/shipments`
- 目的: 送信一覧
- 認証: 必須（ログインユーザーのみ。匿名は不可）
- query: `status`, `cursor`, `limit`
- response 200
```json
{ "items":[{"id":"uuid","status":"sent","created_at":"..."}], "next_cursor":"..." }
```
- エラー: `401 unauthorized`

### 6-5. `GET /v1/shipments/{id}`
- 目的: 送信詳細
- 認証: 必須（所有者 or manage token）
- response 200
```json
{
  "id":"uuid",
  "status":"accessed",
  "files":[{"id":"uuid","name":"a.pdf"}],
  "recipients":[{"email":"a@example.com","status":"downloaded"}]
}
```
- エラー: `403 forbidden`, `404 not_found`

### 6-6. `POST /v1/shipments/{id}/resend`
- 目的: 通知再送
- 認証: 必須（所有者 or manage token）
- request
```json
{ "recipient_emails":["a@example.com"] }
```
- response 202
```json
{ "queued": true }
```
- エラー: `409 shipment_not_sendable`（expired/deleted/revoked）

### 6-7. `DELETE /v1/shipments/{id}`
- 目的: 即時無効 + 削除予約
- 認証: 必須（所有者 or manage token）
- response 200
```json
{ "id":"uuid", "status":"deleted" }
```
- エラー: `404 not_found`

### 6-8. `GET /v1/access/{token}`
- 目的: 受信リンク事前検証
- 認証: 不要（token自体が資格）
- response 200
```json
{
  "shipment_id":"uuid",
  "requires_otp":false,
  "expires_at":"2026-04-11T00:00:00Z",
  "file_summaries":[{"id":"uuid","name":"a.pdf","size_bytes":123}]
}
```
- エラー: `410 token_expired`, `404 invalid_token`

### 6-9. `POST /v1/access/{token}/verify`
- 目的: OTPまたは受信者一致検証
- 認証: 不要
- request
```json
{ "email":"a@example.com", "otp":"123456" }
```
- response 200
```json
{ "verified": true, "access_grant":"jwt_or_session" }
```
- エラー: `401 verification_failed`, `429 too_many_attempts`

### 6-10. `GET /v1/files/{id}/download-url`
- 目的: ダウンロードURL発行
- 認証: 必須（access_grantまたは所有者）
- response 200
```json
{ "url":"https://...", "expires_in_sec":60, "remaining_downloads":9 }
```
- エラー
  - `403 not_allowed`
  - `410 shipment_expired`
  - `409 download_limit_exceeded`

---

## 7. UI/UXフロー

### 7-1. 匿名送信フロー
1. `/send/anonymous` でファイル投入
2. アップロード進捗表示（再試行あり）
3. 送信設定（受信者・期限・回数）
4. 送信完了で「共有URL + 管理URL」を表示
5. 管理URLから再送/削除

### 7-2. ログイン送信フロー
1. `/login` でMagic Link認証
2. `/send` でファイル投入
3. 送信設定して確定
4. `/shipments` で履歴閲覧
5. `/shipments/{id}` で再送/削除/進捗確認

### 7-3. 受信者ダウンロードフロー
1. メールリンクから `/r/{token}`
2. 必要ならメール入力・OTP検証
3. ファイル一覧表示
4. ダウンロード押下で`download-url`取得
5. 期限切れ/回数超過時は専用エラー画面

### 7-4. 画面一覧と遷移
- `Top`
- `AnonymousSend`
- `Login`
- `Send`
- `ShipmentComplete`
- `ShipmentList`
- `ShipmentDetail`
- `RecipientAccess`
- `RecipientVerify`
- `RecipientDownload`
- `ErrorExpired`
- `ErrorForbidden`

---

## 8. 実装タスク分解（Issueレベル）

### Epic A: 基盤・スキーマ
- Issue A1: DB初期スキーマ migration作成（shipments/files/recipients/...）
- Issue A2: sqlc query定義（CRUD + 状態遷移）
- Issue A3: 監査ログミドルウェア実装

### Epic B: アップロード
- Issue B1: `POST /v1/uploads`（multipart initiate + presign）
- Issue B2: `POST /v1/uploads/{id}/complete`
- Issue B3: フロントのmultipart並列アップロード実装
- Issue B4: 失敗part再送と中断復帰

### Epic C: shipment送信
- Issue C1: `POST /v1/shipments`（ready検証 + sent遷移）
- Issue C2: recipients登録と重複排除
- Issue C3: `GET /v1/shipments` + cursor pagination
- Issue C4: `GET /v1/shipments/{id}`詳細
- Issue C5: `POST /v1/shipments/{id}/resend`
- Issue C6: `DELETE /v1/shipments/{id}`

### Epic D: 受信者アクセス
- Issue D1: `GET /v1/access/{token}`
- Issue D2: `POST /v1/access/{token}/verify`
- Issue D3: `GET /v1/files/{id}/download-url`
- Issue D4: download_events記録

### Epic E: メール・非同期
- Issue E1: SES送信クライアント実装
- Issue E2: SQS producer（送信/再送）
- Issue E3: worker実装（再試行3回）

### Epic F: セキュリティ・運用
- Issue F1: レート制限（IP + token）
- Issue F2: CAPTCHA連携（匿名送信）
- Issue F3: 期限切れ・削除バッチ
- Issue F4: 監視メトリクス/アラート設定

---

## 9. 技術的リスクと回避策

1. 大容量アップロード失敗
- 対策: multipart + partリトライ + 中断再開。partサイズ8MB固定。

2. ストレージコスト増大
- 対策: 期限削除バッチ + S3 lifecycle + orphan検出ジョブ（日次）。

3. トークン漏洩
- 対策: tokenはDBにhash保存、URL TTL短縮、revoke API即時反映。

4. メール未到達
- 対策: SPF/DKIM/DMARC整備、再送キュー、送信失敗ダッシュボード。

5. 不正ダウンロード
- 対策: 受信者検証、短命URL、IP/UA異常検知、閾値超えでtoken失効。

6. レート制御不足
- 対策: NGINX/API双方で多段制限（例: 匿名送信 10 req/min/IP）。

---

## 10. MVPスコープ確定

### 10-1. やる機能
- 匿名送信/ログイン送信
- multipartアップロード
- 受信者限定リンク
- 期限・回数制御
- 削除・再送
- 監査ログ・ダウンロードイベント

### 10-2. やらない機能（MVP外）
- ファイル差し替え（同一リンク維持）
- 組織/RBAC
- SSO（SAML/OIDC）
- AVフルスキャン（MVPは拡張子・MIMEチェックのみ）
- 課金

### 10-3. 削る理由
- 差し替えは監査整合性を崩しやすく設計負債が大きい。
- 組織/SSO/課金はドメイン設計拡張が必要でMVP速度を落とす。
- AVフルスキャンはコストと処理遅延が大きく、初期価値に直結しない。

### 10-4. 将来拡張
- recipientごとのDL回数制御
- ワンタイムトークン強制モード
- DLP/ウイルススキャン
- テナント別容量課金

---

## 11. サンプルコード（最小）

### 11-1. Go: presigned URL発行
```go
func (s *Service) CreateDownloadURL(ctx context.Context, fileID uuid.UUID, actor Actor) (string, error) {
    file, sh, err := s.repo.GetFileAndShipment(ctx, fileID)
    if err != nil { return "", err }

    if sh.Status == "expired" || sh.Status == "deleted" || sh.Status == "revoked" {
        return "", ErrShipmentUnavailable
    }
    if sh.CurrentDownloads >= sh.MaxDownloads {
        return "", ErrDownloadLimit
    }

    if err := s.repo.ConsumeDownload(ctx, sh.ID); err != nil {
        return "", err
    }
    return s.s3.PresignGetObject(ctx, file.StorageBucket, file.StorageKey, 60*time.Second)
}
```

### 11-2. Go: shipment作成
```go
func (s *Service) CreateShipment(ctx context.Context, req CreateShipmentReq, actor Actor) (*Shipment, error) {
    sh, err := s.repo.GetShipmentForUpdate(ctx, req.ShipmentID)
    if err != nil { return nil, err }
    if sh.Status != "ready" { return nil, ErrShipmentNotReady }

    sh.Title = req.Title
    sh.Message = req.Message
    sh.ShareMode = req.ShareMode
    sh.ExpiresAt = time.Now().AddDate(0, 0, req.ExpiresInDays)
    sh.MaxDownloads = req.MaxDownloads
    sh.Status = "sent"
    sh.SentAt = ptr(time.Now().UTC())

    if err := s.repo.UpdateShipment(ctx, sh); err != nil { return nil, err }
    _ = s.queue.EnqueueEmail(ctx, sh.ID)
    return sh, nil
}
```

### 11-3. TypeScript: アップロード処理
```ts
export async function uploadFile(file: File, shipmentId?: string) {
  const init = await fetch('/v1/uploads', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      shipment_id: shipmentId ?? null,
      file_name: file.name,
      size_bytes: file.size,
      mime_type: file.type,
      checksum_sha256: 'todo',
      part_count: Math.ceil(file.size / (8 * 1024 * 1024)),
    }),
  }).then(r => r.json());

  await Promise.all(init.presigned_parts.map(async (p: any, i: number) => {
    const start = i * init.part_size_bytes;
    const end = Math.min(file.size, start + init.part_size_bytes);
    await fetch(p.url, { method: 'PUT', body: file.slice(start, end) });
  }));

  return fetch(`/v1/uploads/${init.upload_session_id}/complete`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ parts: [] }),
  });
}
```

### 11-4. TypeScript: ダウンロード処理
```ts
export async function downloadFile(fileId: string, accessGrant: string) {
  const res = await fetch(`/v1/files/${fileId}/download-url`, {
    headers: { Authorization: `Bearer ${accessGrant}` },
  });
  if (!res.ok) throw new Error('download_url_failed');

  const { url } = await res.json();
  window.location.href = url;
}
```

---

## 実装開始チェックリスト（固定）
- [ ] OpenAPI定義ファイル作成（本書6章準拠）
- [ ] migration v1作成（本書4章準拠）
- [ ] 状態遷移テスト（本書5章準拠）
- [ ] レート制限初期値設定（本書9章準拠）
- [ ] 削除バッチのドライラン確認

