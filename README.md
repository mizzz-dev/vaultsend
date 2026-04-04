# vaultsend

Secure Send MVP のバックエンド土台（Go + chi + pgx + sqlc）です。

## ディレクトリ構成（MVP土台）

```text
cmd/api
cmd/worker
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

# uploads 本実装向け（任意上書き）
export UPLOAD_URL_TTL_SEC=900
export RATE_LIMIT_RPS=100
export VERIFY_MAX_ATTEMPTS=5
export DOWNLOAD_RATE_LIMIT=10
export PRESIGNED_URL_TTL=60

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
  - token の生値・hash は返却しない
- `GET /v1/shipments`
  - ログインユーザーの shipment 一覧を返却（`limit`, `offset` ページネーション）
  - `download_count`, `max_download_count`, `file_count` を含む
- `DELETE /v1/shipments/{id}`
  - ログインユーザー本人の shipment のみ論理削除（`status=deleted`）
  - 関連する access token を revoke し、以後ダウンロード不可

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
- 未実装: メール再送API、バウンス処理、SNS連携。
- TODO: 現在の store は hand-rolled 実装です。次PRで sqlc generated code に置き換える予定です。
