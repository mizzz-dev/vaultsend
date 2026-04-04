# vaultsend

Secure Send MVP のバックエンド土台（Go + chi + pgx + sqlc）です。

## ディレクトリ構成（MVP土台）

```text
cmd/api
cmd/worker
cmd/cleanup-worker
internal/config
internal/http
internal/http/handler
internal/domain
internal/mail
internal/queue
internal/service
internal/storage
internal/store
internal/worker
db/migrations
db/query
```

## ローカル起動手順

### 1) PostgreSQL を起動

```bash
docker compose up -d postgres
```

### 2) 環境変数を設定

```bash
export APP_ENV=local
export PORT=8080
export DATABASE_URL='postgres://vaultsend:vaultsend@localhost:5432/vaultsend?sslmode=disable'
export AWS_REGION='ap-northeast-1'
export S3_BUCKET='vaultsend-local'
export SQS_QUEUE_URL='https://sqs.ap-northeast-1.amazonaws.com/123456789012/vaultsend-local'
export SES_FROM_EMAIL='noreply@example.com'
export FRONTEND_URL='http://localhost:3000'
export STRIPE_SECRET_KEY='sk_test_xxx'
export STRIPE_WEBHOOK_SECRET='whsec_xxx'
export STRIPE_PRICE_ID_PRO='price_xxx'

# uploads 本実装向け（任意上書き）
export UPLOAD_URL_TTL_SEC=900
export RATE_LIMIT_RPS=100
export VERIFY_MAX_ATTEMPTS=5
export DOWNLOAD_RATE_LIMIT=10
export PRESIGNED_URL_TTL=60
export CLEANUP_INTERVAL_SEC=180
export CLEANUP_BATCH_SIZE=100
export DELETION_GRACE_PERIOD_HOURS=24

# auth / session（MVP最小実装）
export SESSION_TTL_HOURS=168
export COOKIE_DOMAIN=''
export COOKIE_SECURE=false
export COOKIE_SAMESITE='lax'
```

### 3) マイグレーション適用

```bash
make migrate-up
```

### 4) API サーバ起動

```bash
make run
```

### 5) メール worker 起動（別ターミナル）

```bash
make run-worker
```

### 6) cleanup worker 起動（別ターミナル）

```bash
make run-cleanup-worker
```

## メール送信フロー（SES + SQS）

1. `POST /v1/shipments` で `share_mode=recipient_restricted` の shipment を確定。
2. shipment確定トランザクション完了後、受信者ごとに SQS へ mail notification を enqueue。
3. `cmd/worker` が SQS を long-poll し、メッセージを decode。
4. worker が token 付きダウンロードURLをテンプレート展開。
5. SES で HTML / Text のマルチパートメールを送信。
6. 送信成功時のみ SQS メッセージを delete（失敗時は SQS retry に委譲）。

> TODO: enqueue 失敗時補償は outbox 導入で改善予定。

## API 動作確認（ローカル）

### uploads

```bash
curl -sS -X POST http://localhost:8080/v1/uploads \
  -H 'Content-Type: application/json' \
  -d '{
    "file_name":"sample.bin",
    "file_size":10485760,
    "content_type":"application/octet-stream",
    "checksum_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  }'

curl -sS -X POST http://localhost:8080/v1/uploads/{upload_session_id}/complete \
  -H 'Content-Type: application/json' \
  -d '{
    "parts":[{"part_number":1,"etag":"\"etag-part-1\""}]
  }'
```

### shipments

```bash
# URL共有型（url_shared）
curl -sS -X POST http://localhost:8080/v1/shipments \
  -H 'Content-Type: application/json' \
  -d '{
    "shipment_id":"{uploadsで作成されたshipment_id}",
    "file_ids":["{file_id}"],
    "subject":"4月請求書",
    "message":"添付をご確認ください",
    "share_mode":"url_shared",
    "max_download_count":10,
    "expires_at":"2026-04-11T00:00:00Z",
    "password":"passw0rd123"
  }'

# 受信者限定共有（recipient_restricted）
curl -sS -X POST http://localhost:8080/v1/shipments \
  -H 'Content-Type: application/json' \
  -d '{
    "shipment_id":"{uploadsで作成されたshipment_id}",
    "file_ids":["{file_id}"],
    "subject":"契約書",
    "message":"確認をお願いします",
    "share_mode":"recipient_restricted",
    "recipients":[{"email":"a@example.com"},{"email":"b@example.com"}]
  }'

# ログイン必須: 送信履歴一覧
curl -sS "http://localhost:8080/v1/shipments?limit=20&offset=0" \
  -b /tmp/vs-cookie.txt

# ログイン必須: shipment 詳細
curl -sS http://localhost:8080/v1/shipments/{shipment_id} \
  -b /tmp/vs-cookie.txt

# ログイン必須: shipment 論理削除
curl -sS -X DELETE http://localhost:8080/v1/shipments/{shipment_id} \
  -b /tmp/vs-cookie.txt
```


### auth (register/login/logout/me)

```bash
# register
curl -i -sS -X POST http://localhost:8080/v1/auth/register   -H 'Content-Type: application/json'   -d '{
    "email":"user@example.com",
    "password":"password123",
    "display_name":"Taro"
  }'

# login
curl -i -sS -X POST http://localhost:8080/v1/auth/login   -H 'Content-Type: application/json'   -c /tmp/vs-cookie.txt   -d '{
    "email":"user@example.com",
    "password":"password123"
  }'

# me
curl -sS http://localhost:8080/v1/auth/me -b /tmp/vs-cookie.txt

# logout
curl -i -sS -X POST http://localhost:8080/v1/auth/logout -b /tmp/vs-cookie.txt
```

### access verify / download

```bash
# 1) 受信リンク情報取得（token有効性確認 + password要求有無）
curl -sS http://localhost:8080/v1/access/{access_token}

# 2) password付きshipmentの検証（password不要なら省略可）
curl -sS -X POST http://localhost:8080/v1/access/{access_token}/verify \
  -H 'Content-Type: application/json' \
  -d '{"password":"passw0rd123"}'

# 3) ファイルの短命ダウンロードURL発行（TTL は PRESIGNED_URL_TTL で設定）
curl -sS "http://localhost:8080/v1/files/{file_id}/download-url?access_token={access_token}"
```


## 認証フロー（MVP最小実装）

- `POST /v1/auth/register`
  - email + password でユーザー作成し、同時に session cookie を発行します。
- `POST /v1/auth/login`
  - email + password を検証し、session cookie を再発行します。
- `POST /v1/auth/logout`
  - session を `revoked_at` 更新して失効し、cookieを削除します。
- `GET /v1/auth/me`
  - cookie の session token を検証し、ログイン中ユーザー情報を返します。

## 匿名利用とログイン利用の挙動差

- 匿名利用（cookieなし）
  - `POST /v1/uploads` / `POST /v1/shipments` は従来どおり利用可能です。
  - `owner_user_id` は `NULL` のまま扱います。
- ログイン利用（cookieあり）
  - `POST /v1/uploads` と `POST /v1/shipments` で `owner_user_id` をサーバー側で自動反映します。
  - shipment作成時に、対象ファイルの `owner_user_id` がログインユーザーと一致しない場合は409で拒否します。
  - `GET /v1/shipments` / `GET /v1/shipments/{id}` / `DELETE /v1/shipments/{id}` はログイン必須です。
  - shipment一覧/詳細/削除では `owner_user_id` を必ず検証し、他ユーザー資産には403を返します。

## レート制限 / セキュリティ仕様（MVP仮置き）

- API全体: `100 req / 分 / IP`（`RATE_LIMIT_RPS`）
- `POST /v1/access/{token}/verify`: より厳しめのレート制限 + brute-force対策
  - token単位の失敗回数をカウント
  - `VERIFY_MAX_ATTEMPTS` 超過で 10 分ロック
- `GET /v1/files/{id}/download-url`: token + IP 組み合わせで短時間連続発行を制限（`DOWNLOAD_RATE_LIMIT` / 分）
- セキュリティヘッダ:
  - `X-Content-Type-Options: nosniff`
  - `X-Frame-Options: DENY`
  - `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'; base-uri 'none'`
- suspicious accessログ:
  - `rate_limit_hit`
  - `verify_failure`, `verify_locked`
  - `download_abuse_block`

## ローカル確認方法（仮置き）

- LocalStack 等で SQS / SES のエミュレーション先を構成して疎通確認してください。
- 最低確認:
  - recipient_restricted shipment作成で API が 2xx を返す
  - worker ログに `send email` エラーが出ない
  - 対象 recipient の受信箱に通知メールが届く

## 開発用コマンド

```bash
make test
make lint
make sqlc-generate
make migrate-down
```

## Billing（サブスクリプション）仕様

- プラン
  - `free`: 最大ファイルサイズ 1GB / 保存期間 3日 / 月間 shipment 50件まで
  - `pro`: 最大ファイルサイズ 10GB / 保存期間 7日 / shipment 制限なし（MVP）
- Checkout
  - `POST /v1/billing/checkout`（ログイン必須）で Stripe Checkout Session URL を発行します。
- Webhook
  - `POST /v1/billing/webhook` で `customer.subscription.created|updated|deleted` を受け取り、
    `subscriptions` テーブルへ反映します。

- プラン情報API
  - `GET /v1/billing/plan`（ログイン必須）
  - レスポンス:
    - `plan`: `free|pro`
    - `limits.max_file_size` / `limits.max_storage_days` / `limits.monthly_shipment_limit`
    - `usage.current_month_shipments` / `usage.current_storage_bytes`
    - `remaining.remaining_shipments`
- 制限エラー（plan_limit系）共通フォーマット
  - `error`: `plan_limit_exceeded`
  - `code`: `FILE_SIZE_LIMIT | STORAGE_DAYS_LIMIT | MONTHLY_SHIPMENT_LIMIT`
  - `message`: ユーザー向け説明
  - `upgrade_required`: `true`
  - `upgrade_url`: `/settings/billing`
  - `recommended_plan`: `pro`
- 制限適用
  - upload 作成時にプランの `max_file_size` を検証
  - shipment 確定時に保存期間と月間作成数を検証

### Stripe webhook ローカル設定例

```bash
stripe listen --forward-to localhost:8080/v1/billing/webhook
```

## shipments仕様（今回PR時点）

- `POST /v1/shipments`
  - `file_ids` を必須化し、アップロード済みファイル（`upload_status=completed`）のみ shipment 化
  - `share_mode=url_shared | recipient_restricted` をサポート
  - recipient_restricted の場合のみ recipients を作成（メール正規化 + 重複除外）
  - access token を DB 保持型で作成（生トークンは `url_shared` のみレスポンス返却）
  - パスワード指定時は bcrypt で `password_hash` を保存（平文保存禁止）
  - 送信確定時に shipment status を `sent` へ遷移
  - recipient_restricted は送信確定後に SQS enqueue（token生値をイベントに積む）
- `GET /v1/shipments/{id}`
  - ログインユーザー本人の shipment のみ取得可能
  - shipment, files, recipients を返却
  - download_count / recipient_downloads / last_download_at を返却
  - notification_summary を返却
    - `total_notifications`, `queued_count`, `sent_count`, `failed_count`, `last_notification_at`
  - recipient_summaries を返却
    - `recipient_status`（DB上の recipients.status）
    - `has_downloaded`（download_events success が1件以上かどうか）
    - `notification_count`, `last_notification_status`, `last_notification_type`, `last_notified_at`
    - `download_count`, `first_download_at`, `last_download_at`
  - token の生値・hash は返却しない
- `GET /v1/shipments`
  - ログインユーザーの shipment 一覧を返却（`limit`, `offset` ページネーション）
  - `download_count`, `max_download_count`, `file_count` を含む
- `POST /v1/shipments/{id}/resend`
  - ログインユーザー本人の `recipient_restricted` shipment の通知メールを再送
  - `recipient_ids` 未指定時は全 recipient を再送対象にする
  - `recipient_ids` 指定時は shipment に属する recipient のみ許可
  - 再送不可条件: owner不一致 / `url_shared` / deleted / revoked / expired / 不正status
  - レスポンス: `shipment_id`, `resent_recipient_count`, `skipped_recipient_count`, `skipped_reasons`, `queued_at`
- `GET /v1/shipments/{id}/notifications`
  - ログインユーザー本人の shipment のみ取得可能
  - shipment に紐づく notification_events を新しい順で返却
  - `limit` / `offset` でページング
  - レスポンス項目:
    - `notification_event_id`, `recipient_id`, `recipient_email`
    - `event_type`, `status`, `error_message`
    - `created_at`, `queued_at`, `sent_at`, `failed_at`
- `GET /v1/shipments/{id}/recipients`
  - ログインユーザー本人の shipment のみ取得可能
  - recipient ごとの通知・受領集計を返却（`items` 配列）
- `DELETE /v1/shipments/{id}`
  - ログインユーザー本人の shipment のみ論理削除（`status=deleted`）
  - 関連する access token を revoke し、以後ダウンロード不可

## cleanup worker（期限切れ/削除データの自動クリーンアップ）

- 目的:
  - 期限切れ shipment の `status` を自動で `expired` に更新
  - 論理削除済み shipment を猶予期間経過後に物理削除
  - 紐づく S3 オブジェクトを削除し、DB レコードも cascade で削除
- 対象条件:
  1. 期限切れ: `expires_at < now` かつ `status NOT IN ('deleted','expired','revoked')`
  2. 物理削除対象: `status = 'deleted'` かつ `deleted_at < now - grace_period`
- 物理削除対象テーブル:
  - `shipments`, `files`, `recipients`, `access_tokens`, `download_events`, `notification_events`
- 安全対策:
  - `CLEANUP_BATCH_SIZE` で 1 回あたりの最大処理件数を制限
  - shipment 単位でエラーをログ出力し、他 shipment の処理を継続
  - S3 delete は短いリトライ（デフォルト3回）
- 仮置き値:
  - `CLEANUP_INTERVAL_SEC=180`（3分）
  - `CLEANUP_BATCH_SIZE=100`
  - `DELETION_GRACE_PERIOD_HOURS=24`

### 再送API request / response 例

```bash
curl -sS -X POST http://localhost:8080/v1/shipments/{shipment_id}/resend \
  -H 'Content-Type: application/json' \
  -b /tmp/vs-cookie.txt \
  -d '{
    "recipient_ids": ["{recipient_id_1}"]
  }'
```

```json
{
  "shipment_id": "d5a79053-a2fa-4b57-aeb0-83af5dc25728",
  "resent_recipient_count": 1,
  "skipped_recipient_count": 0,
  "skipped_reasons": [],
  "queued_at": "2026-04-04T08:10:00Z"
}
```

### notification_events（今回追加）

- 送信通知の enqueue 履歴を `notification_events` に記録します。
- `event_type`: `initial_send` / `resend`
- `status`: `queued` / `sent` / `failed`
- API側で enqueue 前に `queued` を記録し、worker側で送信結果に応じて `sent_at` / `failed_at` を更新します。
- 仮置き: 失敗時の再試行制御は SQS retry に委譲（独自の再送回数制御は未実装）。

### shipment detail response 例（通知/受領可視化）

```json
{
  "id": "d5a79053-a2fa-4b57-aeb0-83af5dc25728",
  "status": "sent",
  "share_mode": "recipient_restricted",
  "subject": "4月請求書",
  "download_count": 1,
  "notification_summary": {
    "total_notifications": 3,
    "queued_count": 1,
    "sent_count": 1,
    "failed_count": 1,
    "last_notification_at": "2026-04-04T08:10:00Z"
  },
  "recipient_summaries": [
    {
      "recipient_id": "8fb4d31a-5d14-4874-bfbf-5ca5f1d9bdbf",
      "email": "a@example.com",
      "recipient_status": "pending",
      "notification_count": 2,
      "last_notification_status": "sent",
      "last_notification_type": "resend",
      "last_notified_at": "2026-04-04T08:10:00Z",
      "has_downloaded": true,
      "download_count": 1,
      "first_download_at": "2026-04-04T08:30:00Z",
      "last_download_at": "2026-04-04T08:30:00Z"
    }
  ]
}
```

### notifications API response 例

```json
{
  "items": [
    {
      "notification_event_id": 120,
      "recipient_id": "8fb4d31a-5d14-4874-bfbf-5ca5f1d9bdbf",
      "recipient_email": "a@example.com",
      "event_type": "resend",
      "status": "failed",
      "error_message": "ses temporary failure",
      "created_at": "2026-04-04T08:00:00Z",
      "queued_at": "2026-04-04T08:00:00Z",
      "sent_at": null,
      "failed_at": "2026-04-04T08:00:03Z"
    }
  ],
  "limit": 20,
  "offset": 0,
  "total": 1
}
```

### 送信履歴APIレスポンス例（抜粋）

```json
{
  "items": [
    {
      "id": "8fb4d31a-5d14-4874-bfbf-5ca5f1d9bdbf",
      "subject": "4月請求書",
      "share_mode": "url_shared",
      "status": "sent",
      "created_at": "2026-04-04T03:00:00Z",
      "expires_at": "2026-04-11T03:00:00Z",
      "download_count": 2,
      "max_download_count": 10,
      "file_count": 3
    }
  ],
  "limit": 20,
  "offset": 0,
  "total": 1
}
```

## 補足（仮置き / 未実装）

- 仮置き: `POST /v1/uploads` は shipment 未指定時に匿名 draft shipment を自動作成します。
- 仮置き: `share_mode=public_link` は互換入力として受け付け、内部では `url_shared` に正規化します。
- 仮置き: レート制限は in-memory 実装です（将来 Redis 置換予定）。
- 仮置き: 認証は session token + cookie を採用（JWT / OAuth / Magic Link は未実装）。
- 仮置き: ダウンロード回数制御は shipment 単位（`download_events` の success件数）です。
- 仮置き: `download_events.ip_hash` にはIP平文ではなくSHA-256 hashを保存します。
- 仮置き: recipient の最新通知状態は「最新(created_at desc, id desc)の notification_event」で判定します。
- 仮置き: recipient.status の自動更新（download成功時の downloaded 遷移）は未対応です。
- TODO: 将来PRで download成功時に recipients.status 更新を導入し、状態整合性を自動化する。
- 未実装: バウンス処理、SNS連携。
- TODO: 現在の store は hand-rolled 実装です。次PRで sqlc generated code に置き換える予定です。
