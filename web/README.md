# VaultSend Web

VaultSend の Go API を利用する Next.js（App Router / TypeScript）フロントエンドです。

## 今回の実装範囲

- トップページ
- メールアドレス・パスワードによるログイン
- ユーザー登録
- セッションCookieを利用したログイン状態確認
- 送信履歴一覧
- 件名・ID検索、状態絞り込み、ページング
- shipment詳細表示
- 通知・受領・ダウンロード状況の表示
- 受信者限定shipmentの通知再送
- shipmentの論理削除
- ログアウト

## 起動方法

### 1. Go APIを起動

リポジトリルートで、READMEに従ってPostgreSQL・APIを起動します。

```bash
make migrate-up
make run
```

### 2. フロントエンドの環境変数を設定

```bash
cd web
cp .env.example .env.local
```

標準では、Next.jsの `/api/*` を `http://localhost:8080/*` へプロキシします。
APIの接続先を変更する場合は、`.env.local` の `VAULTSEND_API_URL` を変更してください。

### 3. 依存関係をインストールして起動

```bash
npm install
npm run dev
```

ブラウザで `http://localhost:3000` を開きます。

## 開発用コマンド

```bash
npm run lint
npm run typecheck
npm run build
```

## API接続方針

- ブラウザは `/api/v1/*` を呼び出します。
- Next.jsのrewriteで既存のGo APIへ転送します。
- APIが発行するHttpOnlyセッションCookieを同一オリジン経由で利用します。
- 認証、組織権限、プラン制限、shipment操作可否はGo APIの判定を最終結果として扱います。
- トークン、Stripe secret、AWS資格情報などの秘密情報はブラウザへ渡しません。

## 意図的に未実装の機能

今回のPRはフロントエンド基盤と送信管理画面に範囲を限定しています。以下は次PR以降で実装します。

- S3 multipart uploadを利用したファイル送信ウィザード
- 受信者向け `/r/{token}` ダウンロード画面
- organization管理・メンバー管理画面
- プラン・Checkout・請求書画面
- E2Eテスト

## ディレクトリ

```text
web/
├── app/
│   ├── auth/
│   ├── shipments/
│   ├── globals.css
│   ├── layout.tsx
│   └── page.tsx
├── components/
├── lib/
├── next.config.ts
├── package.json
└── tsconfig.json
```
