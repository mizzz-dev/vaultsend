# vaultsend

Secure Send MVP のバックエンド土台（Go + chi + pgx + sqlc）です。

## ディレクトリ構成（MVP土台）

```text
cmd/api
internal/config
internal/http
internal/http/handler
internal/domain
internal/service
internal/storage
internal/store
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

# uploads 本実装向け（任意上書き）
export UPLOAD_URL_TTL_SEC=900
```

### 3) マイグレーション適用

```bash
make migrate-up
```

### 4) API サーバ起動

```bash
make run
```

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
    "share_mode":"recipient_restricted",
    "recipients":[{"email":"a@example.com"},{"email":"b@example.com"}]
  }'

curl -sS http://localhost:8080/v1/shipments/{shipment_id}
```

### access verify / download

```bash
# 1) 受信リンク情報取得（token有効性確認 + password要求有無）
curl -sS http://localhost:8080/v1/access/{access_token}

# 2) password付きshipmentの検証（password不要なら省略可）
curl -sS -X POST http://localhost:8080/v1/access/{access_token}/verify \
  -H 'Content-Type: application/json' \
  -d '{"password":"passw0rd123"}'

# 3) ファイルの短命ダウンロードURL発行（TTL 60秒）
curl -sS "http://localhost:8080/v1/files/{file_id}/download-url?access_token={access_token}"
```

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
- `GET /v1/shipments/{id}`
  - shipment, files, recipients を返却
  - token の生値・hash は返却しない

## 補足（仮置き / 未実装）

- 仮置き: `POST /v1/uploads` は shipment 未指定時に匿名 draft shipment を自動作成します。
- 仮置き: `POST /v1/shipments` の `access_url` は `https://app.example.com/r/{token}` 固定です。
- 仮置き: `share_mode=public_link` は互換入力として受け付け、内部では `url_shared` に正規化します。
- 仮置き: brute-force対策（rate limit / captcha）は TODO です。
- 仮置き: ダウンロード回数制御は shipment 単位（`download_events` の success件数）です。
- 仮置き: `download_events.ip_hash` にはIP平文ではなくSHA-256 hashを保存します。
- 未実装: SES/SQS worker、認証本実装、virus scan。
- TODO: 現在の store は hand-rolled 実装です。次PRで sqlc generated code に置き換える予定です。
