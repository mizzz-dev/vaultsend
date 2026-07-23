# Playwright E2E 設計・運用

## 1. 目的

VaultSend Webの主要な送信・受信導線を、実ブラウザ操作とNext.jsのAPI rewriteを通して検証します。

unit testや型チェックだけでは検出しにくい、以下の回帰をPull Request時に検出することが目的です。

- 画面とAPI契約の不一致
- multipart uploadのPresigned URL・ETag処理の不整合
- 送信ウィザードの状態遷移不備
- Access Grant Cookieの転送不備
- 受信画面のパスワード検証・再読み込み復元不備
- ダウンロードURL取得からブラウザ保存開始までの不整合
- モバイル幅で主要操作が利用できない問題

## 2. 対象フロー

### 2-1. 送信フロー

```text
/send
  -> 認証状態確認
  -> ファイル選択
  -> SHA-256計算
  -> upload session作成
  -> multipart PUT
  -> upload complete
  -> URL共有条件入力
  -> shipment確定
  -> 共有URL表示
```

確認内容:

- `/v1/auth/me`の成功後に送信画面が表示される
- ファイルinputからファイルを追加できる
- SHA-256が64文字でupload APIへ渡る
- Presigned URLへ全パートをPUTする
- CORS経由で`ETag`を取得する
- 全パートをcomplete APIへ渡す
- URL共有shipmentを確定する
- 完了画面にShipment ID、最大DL回数、共有URLを表示する
- console error・page errorが発生しない

### 2-2. 受信フロー

```text
/r/e2e-token
  -> 受信情報取得
  -> 不正パスワードのエラー表示
  -> 正しいパスワードでGrant Cookie発行
  -> ファイル一覧表示
  -> ページ再読み込み
  -> Grant状態復元
  -> ダウンロードURL取得
  -> ブラウザのdownload開始
```

確認内容:

- パスワード保護画面が表示される
- 不正値で`invalid_password`を表示する
- 検証成功後にファイル一覧を表示する
- HttpOnly CookieがNext.js rewrite経由でブラウザへ保存される
- 再読み込み後も`verified=true`として復元される
- download URL APIがGrant Cookieを受け取る
- `Content-Disposition`に従って`contract.pdf`のダウンロードが開始される
- console error・page errorが発生しない

## 3. テスト構成

```text
web/
├── playwright.config.ts
└── e2e/
    ├── mock-api.mjs
    └── specs/
        ├── send-flow.spec.ts
        └── recipient-flow.spec.ts
```

### Playwright設定

- Browser: Chromium
- Desktop: `Desktop Chrome`
- Mobile: `Pixel 7`
- CI workers: 1
- CI retry: 1
- test timeout: 30秒
- web server timeout: 120秒
- failure時のみtrace・screenshot・videoを保持

## 4. Mock API方針

E2Eでは本番AWS・Stripe・PostgreSQL・メール送信へ接続しません。

`web/e2e/mock-api.mjs`が次を提供します。

- 認証済みユーザーの`/v1/auth/me`
- upload session作成
- mock S3 multipart PUT
- `ETag`付きCORSレスポンス
- upload complete
- shipment確定
- access inspect
- password verifyとHttpOnly Cookie
- download URL発行
- mock downloadレスポンス

Next.jsは通常どおり`/api/v1/*`を呼び出し、E2E起動時だけ次へrewriteします。

```text
VAULTSEND_API_URL=http://127.0.0.1:8081
```

これにより、画面側でE2E専用API分岐を追加せず、本番と同じAPIクライアント・rewrite経路を検証します。

## 5. ローカル実行

### 初回のみChromiumをインストール

```bash
cd web
npm ci
npx playwright install --with-deps chromium
```

### 全E2Eを実行

```bash
npm run e2e
```

リポジトリルートからは次を利用できます。

```bash
make web-e2e
```

### headed実行

```bash
cd web
npm run e2e:headed
```

### HTML report表示

```bash
cd web
npm run e2e:report
```

Playwrightの`webServer`がmock APIとNext.js dev serverを自動起動するため、通常は別ターミナルでサーバーを起動する必要はありません。

## 6. CI

GitHub ActionsのJob名:

```text
Playwright / 送信・受信E2E
```

Webのlint・typecheck・build Job成功後に実行します。

実行内容:

1. `npm ci --no-audit --no-fund`
2. `npx playwright install --with-deps chromium`
3. `npm run e2e`

失敗時は次をArtifactとして7日間保存します。

- E2Eコマンドログ
- HTML report
- trace
- screenshot
- video

Artifact名:

```text
ci-e2e-failure-{run_id}-{run_attempt}
```

## 7. セキュリティ・運用上の注意

- mock APIに本番Secretを設定しない
- fixtureへ実在ユーザー・実在メール・実access tokenを含めない
- 本番AWS・Stripe・DBへ接続しない
- E2E用Cookieは固定値であり、mock server以外では利用しない
- Presigned URL相当のmock URLをログ以外へ永続化しない
- CI Artifactは失敗時のみ作成し、7日で削除する

## 8. テスト追加ルール

- role・label・headingなど利用者が認識できるlocatorを優先する
- CSS classやDOM階層へ過度に依存しない
- 固定sleepを使わず、表示・response・download eventを待つ
- 画面だけでなくAPI responseやdownload eventも必要に応じて確認する
- console errorとpage errorを明示的に検証する
- DesktopとMobileの両方で成立する操作を基本とする
- mock APIのrequest validationを弱めすぎない

## 9. 対象外

現時点では次をE2E対象外とします。

- 実Go API・実PostgreSQLを起動したE2E
- 実S3 multipart upload
- SES通知メール到達
- Stripe Checkout
- organization管理画面
- 画面再読み込み後のupload session復元
- multipart abort
- Safari・Firefox
- 10GB実ファイルのアップロード

実Go API・PostgreSQLとの結合は、既存のGo unit test、PostgreSQL integration test、migration検証で補完します。

## 10. 次の改善候補

1. 実Go API・PostgreSQLを起動したHTTP E2E
2. 認証登録・ログイン・ログアウトE2E
3. 送信履歴一覧・詳細・再送・削除E2E
4. upload失敗と未完了パート再試行E2E
5. Access Grant期限切れ・再入力E2E
6. Playwright accessibility snapshotの追加
7. Firefoxプロジェクトの追加
