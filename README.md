# vaultsend

Secure Send MVP のバックエンド土台（Go + chi + pgx + sqlc）です。

## ディレクトリ構成（MVP土台）

```text
cmd/api
internal/config
internal/http
internal/http/handler
internal/http/middleware
internal/domain
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
```

### 3) マイグレーション適用

```bash
make migrate-up
```

### 4) API サーバ起動

```bash
make run
```

### 5) ヘルスチェック確認

```bash
curl -i http://localhost:8080/healthz
```

## 開発用コマンド

```bash
make test
make lint
make sqlc-generate
make migrate-down
```

## 補足

- `POST /v1/uploads`, `POST /v1/uploads/{id}/complete`, `POST /v1/shipments`, `GET /v1/shipments/{id}` は雛形実装です。
- S3 multipart / SES / SQS の本実装は次PRで実施します（現状は TODO と仮置き値を返却）。
