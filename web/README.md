# VaultSend Web

VaultSend の Go API を利用する Next.js（App Router / TypeScript）フロントエンドです。

## 実装済み機能

- トップページ
- メールアドレス・パスワードによるログイン
- ユーザー登録
- セッションCookieを利用したログイン状態確認
- S3 multipart upload対応のファイル送信ウィザード
  - ドラッグ＆ドロップ／複数ファイル選択
  - 1ファイル10GB、最大20ファイル
  - チャンク単位のSHA-256計算
  - 3パート並列PUT
  - パート単位の最大3回再試行
  - 同一upload session内の完了済みパートを除外した再試行
  - URL共有／受信者限定共有
  - 有効期限、最大ダウンロード回数、任意パスワード
- 受信者向け `/r/{token}` ダウンロード画面
  - token・shipment・有効期限・ダウンロード上限の確認
  - パスワード検証
  - HttpOnly短命Access Grant Cookie
  - ファイル一覧・サイズ・受信情報表示
  - 短命Presigned download URL発行
  - 期限切れ、無効化、上限超過、再試行制限のエラー画面
- 送信履歴一覧
- 件名・ID検索、状態絞り込み、ページング
- shipment詳細表示
- 通知・受領・ダウンロード状況の表示
- 受信者限定shipmentの通知再送
- shipmentの論理削除
- ログアウト

## 動作要件

- Node.js 20.19以上
- Go API
- PostgreSQL
- multipart uploadを許可したS3 Bucket
- S3 Bucketからブラウザへ `ETag` を公開するCORS設定
- APIプロセスに32バイト以上の `ACCESS_GRANT_SECRET`

## 起動方法

### 1. Go APIを起動

リポジトリルートで、READMEに従ってPostgreSQL・APIを起動します。

```bash
export ACCESS_GRANT_SECRET='replace-with-at-least-32-random-bytes'
export ACCESS_GRANT_TTL_SEC=600
make migrate-up
make run
```

`APP_ENV=local|test` では開発用のGrant署名鍵を自動設定します。本番環境では必ずSecret Manager等でランダムな値を設定してください。

### 2. フロントエンドの環境変数を設定

```bash
cd web
cp .env.example .env.local
```

標準では、Next.jsの `/api/*` を `http://localhost:8080/*` へプロキシします。
APIの接続先を変更する場合は、`.env.local` の `VAULTSEND_API_URL` を変更してください。

### 3. S3 BucketのCORSを設定

送信ウィザードはPresigned URLへブラウザから直接PUTします。`AllowedOrigins` は実際のフロントエンドOriginへ限定してください。

```json
[
  {
    "AllowedHeaders": ["*"],
    "AllowedMethods": ["PUT"],
    "AllowedOrigins": ["http://localhost:3000"],
    "ExposeHeaders": ["ETag"],
    "MaxAgeSeconds": 300
  }
]
```

`ExposeHeaders` に `ETag` がない場合、S3へのPUT自体が成功してもブラウザがmultipart completeに必要なETagを取得できません。

### 4. 依存関係をインストールして起動

```bash
npm install
npm run dev
```

ブラウザで `http://localhost:3000` を開きます。

## 送信フロー

1. `/send` でファイルを選択します。
2. ブラウザがファイルをチャンク単位で読み込み、SHA-256を計算します。
3. `POST /v1/uploads` でupload sessionとパートごとのPresigned URLを取得します。
4. ブラウザからS3へ各パートを直接PUTします。
5. `POST /v1/uploads/{id}/complete` でmultipart uploadを確定します。
6. 全ファイル完了後、送信条件を入力します。
7. `POST /v1/shipments` でshipmentを確定します。

upload complete APIは、S3・DB更新後にレスポンスだけ失われた場合の再試行を考慮し、完了済みsessionでは既存の `file_id` と `shipment_id` を返します。

## 受信・ダウンロードフロー

1. メールまたは共有URLから `/r/{token}` を開きます。
2. `GET /v1/access/{token}` でshipmentとファイル情報を確認します。
3. パスワード付きshipmentでは `POST /v1/access/{token}/verify` を実行します。
4. 検証成功時、APIがtoken hashと期限に紐づく署名付きGrantをHttpOnly Cookieへ保存します。
5. `GET /v1/files/{id}/download-url` は、パスワード付きshipmentの場合にGrant Cookieを検証します。
6. APIが短命Presigned URLを返し、ブラウザがS3からファイルを取得します。

Access Grantは既定10分で失効し、access tokenまたはshipmentの期限を超えて発行されません。Cookie値はJavaScriptから読み取れず、別tokenへ再利用できません。

## 再試行仕様と制約

- PUT失敗時はパート単位で最大3回まで指数バックオフ付きで再試行します。
- 再試行時は同じupload sessionを利用し、完了済みパートは再送しません。
- Presigned URLの有効期限を過ぎたsessionは再利用しません。
- 現時点では画面再読み込み後のupload session復元とmultipart abort APIは未実装です。
- S3 complete成功後、DB更新前に障害が発生したケースの自動復旧は未実装です。運用上は対象multipart/objectとupload sessionの整合確認が必要です。
- ダウンロードURL発行時に利用回数を消費するため、発行後にブラウザ側で保存を中断した場合も回数へ含まれます。

## 開発用コマンド

```bash
npm run lint
npm run typecheck
npm run build
```

リポジトリルートからは以下も利用できます。

```bash
make web-install
make web-run
make web-lint
make web-typecheck
make web-build
```

## API接続方針

- ブラウザは `/api/v1/*` を呼び出します。
- Next.jsのrewriteで既存のGo APIへ転送します。
- APIが発行するHttpOnlyセッションCookieとAccess Grant Cookieを同一オリジン経由で利用します。
- 認証、組織権限、プラン制限、shipment操作可否、パスワード検証はGo APIの判定を最終結果として扱います。
- S3へのファイル本体アップロード・ダウンロードのみPresigned URLを利用します。
- access tokenは受信リンクとAPI queryに含まれますが、ログや永続ストレージへ保存しません。
- Stripe secret、AWS資格情報、Access Grant署名鍵などの秘密情報はブラウザへ渡しません。

## 意図的に未実装の機能

- organization管理・メンバー管理画面
- プラン・Checkout・請求書画面
- 画面再読み込み後のアップロード再開
- multipart upload abort API
- ダウンロードURL発行後・実ダウンロード完了前の失敗補償
- E2Eテスト

## ディレクトリ

```text
web/
├── app/
│   ├── auth/
│   ├── r/[token]/
│   ├── send/
│   ├── shipments/
│   ├── globals.css
│   ├── layout.tsx
│   └── page.tsx
├── components/
│   ├── auth-panel.tsx
│   ├── recipient-download-panel.tsx
│   ├── send-wizard.tsx
│   └── shipment-dashboard.tsx
├── lib/
│   ├── api.ts
│   ├── file-hash.ts
│   ├── multipart-upload.ts
│   └── types.ts
├── next.config.ts
├── package.json
└── tsconfig.json
```
