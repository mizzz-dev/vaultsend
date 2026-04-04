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

### 5) uploads API 動作確認（ローカル）

```bash
# 1. multipart 開始 + 全パートpresigned URL発行
curl -sS -X POST http://localhost:8080/v1/uploads \
  -H 'Content-Type: application/json' \
  -d '{
    "file_name":"sample.bin",
    "file_size":10485760,
    "content_type":"application/octet-stream",
    "checksum_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  }'

# 2. クライアント側で各URLにPUTした後、complete を呼ぶ
curl -sS -X POST http://localhost:8080/v1/uploads/{upload_session_id}/complete \
  -H 'Content-Type: application/json' \
  -d '{
    "parts":[{"part_number":1,"etag":"\"etag-part-1\""}]
  }'
```

## アップロード仕様（今回PR時点）

- `POST /v1/uploads`
  - upload_session を DB に保存
  - S3 multipart upload を開始
  - 必要パート分の presigned URL を **1回で返却**
  - バリデーション（`file_name` 必須, `file_size > 0`, 10GB上限, content_type簡易チェック）
- `POST /v1/uploads/{id}/complete`
  - upload_session の status 検証
  - S3 complete multipart upload 実行
  - files レコード作成 + upload_session.completed 反映（トランザクション）
  - 二重完了は 409 を返却

## 開発用コマンド

```bash
make test
make lint
make sqlc-generate
make migrate-down
```

## 補足（仮置き / 未実装）

- 仮置き: shipment 未作成状態でも uploads を先行させるため、`POST /v1/uploads` で匿名 draft shipment を自動作成します。
- 仮置き: presigned URL 一括返却の上限として、パート数を1000に制限しています（巨大レスポンス回避）。
- 未実装: shipment 作成本実装、recipient、access token、download-url、SES/SQS worker 連携、virus scan、認証本実装。
- TODO: 現在の store は hand-rolled 実装です。次PRで sqlc generated code に置き換える予定です。
