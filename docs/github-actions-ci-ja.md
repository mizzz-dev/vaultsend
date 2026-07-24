# GitHub Actions CI 設計・運用

## 目的

Pull Request と `main` へのpush時に、Goバックエンド、PostgreSQL、Next.js Web、Playwright E2Eの品質ゲートを自動実行します。

CIで検出する対象は次のとおりです。

- Go依存定義とchecksumの不整合
- Goコードのフォーマット漏れ
- Goの静的解析エラー
- Goテスト失敗
- API・mail worker・cleanup workerのコンパイル失敗
- migrationのSQL構文・up/down不整合
- PostgreSQL制約・trigger・cascadeの回帰
- 手書きStore SQLと実schemaの不整合
- npm lockfileと`package.json`の不整合
- ESLintエラー
- TypeScript型エラー
- Next.js本番ビルド失敗
- 送信ウィザードとAPI契約の不整合
- multipart PUT・CORS・ETag取得の回帰
- 受信リンクのパスワード検証・Grant Cookie復元の回帰
- ブラウザのdownload開始失敗
- Desktop・Mobile viewportでの主要導線回帰

## Workflow

対象ファイル:

```text
.github/workflows/ci.yml
```

Workflow名:

```text
CI
```

## 実行条件

- `main` を対象とするPull Request
- `main` へのpush
- GitHub画面からの手動実行（`workflow_dispatch`）

同一ブランチで新しい実行が開始された場合、古い実行はキャンセルします。

## 権限

Workflow全体のGitHub Token権限は以下だけです。

```yaml
permissions:
  contents: read
```

CIではAWS、Stripe、本番PostgreSQLなどのSecretを使用しません。

## Go Job

Job名:

```text
Go / format・vet・test・build
```

Goのバージョンは`go.mod`の`go`・`toolchain`ディレクティブを参照します。

実行内容:

1. `go mod download`
2. `go mod verify`
3. `go mod tidy`後に`go.mod`・`go.sum`の差分がないことを確認
4. `gofmt`適用漏れ確認
5. `go vet ./...`
6. `go test -count=1 ./...`
7. 以下の実行バイナリをbuild
   - `./cmd/api`
   - `./cmd/worker`
   - `./cmd/cleanup-worker`

### 依存解決方針

`go.mod`と`go.sum`をリポジトリへコミットし、CIでは次を設定しています。

```yaml
GOTOOLCHAIN: local
GOFLAGS: -mod=readonly
```

CI実行中に依存定義やchecksumを暗黙更新しません。ソースコードで利用するモジュールを追加・削除した場合は、開発環境で次を実行し、`go.mod`と`go.sum`を同じPRへ含めてください。

```bash
go mod tidy
go mod verify
```

`go mod tidy`実行後に差分が残るPRは、依存定義が未更新としてCIで失敗します。

## PostgreSQL Job

Job名:

```text
PostgreSQL / migration・Store integration
```

`postgres:16`のservice containerを起動し、破棄可能な`vaultsend_test`データベースを使用します。本番DBや本番認証情報は利用しません。

実行内容:

1. PostgreSQL clientの存在確認
2. 全migrationのup/downペア確認
3. up migrationを昇順で全件適用
4. 主要テーブルの存在確認
5. down migrationを降順で全件適用
6. `public` schemaにテーブル・enumが残っていないことを確認
7. up migrationを再適用
8. `integration` build tag付きStore testを実行

Migration検証:

```bash
bash scripts/verify-migrations.sh
```

Store integration test:

```bash
go test -tags=integration -count=1 -v ./internal/store
```

詳細は`docs/postgres-integration-tests-ja.md`を参照してください。

## Web Job

Job名:

```text
Web / lint・typecheck・build
```

Node.jsは`web/package.json`の要件に合わせて`20.19.0`を使用します。

実行内容:

1. `npm ci --no-audit --no-fund`
2. `npm run lint`
3. `npm run typecheck`
4. `npm run build`

Next.js build時のAPI接続先には、build時の設定値として以下を指定します。

```text
VAULTSEND_API_URL=http://localhost:8080
```

APIサーバーへの実通信は行いません。

### 依存解決方針

`web/package.json`と`web/package-lock.json`をリポジトリへコミットし、CIと通常のローカルセットアップでは`npm ci`を使用します。

`npm ci`はlockfileを更新せず、`package.json`と`package-lock.json`が一致しない場合に失敗します。依存パッケージを追加・更新する場合は次を使用し、両ファイルを同じPRへ含めてください。

```bash
cd web
npm install <package>
```

npmキャッシュkeyには`web/package-lock.json`のhashを使用します。lockfileが更新されると新しいキャッシュへ切り替わります。

## Playwright E2E Job

Job名:

```text
Playwright / 送信・受信E2E
```

Web Jobが成功した後に実行します。Webのlint・型・buildが失敗している状態でブラウザテストを開始しません。

実行内容:

1. `npm ci --no-audit --no-fund`
2. `npx playwright install --with-deps chromium`
3. `npm run e2e`

Playwrightは次の2 projectを順次実行します。

- `chromium-desktop`: Desktop Chrome相当
- `chromium-mobile`: Pixel 7相当

各projectで次の2シナリオを実行し、合計4ケースを確認します。

- `/send` のファイル選択・multipart upload・shipment確定
- `/r/e2e-token` のパスワード検証・Grant復元・download開始

本番AWS・Stripe・PostgreSQL・メールへ接続せず、`web/e2e/mock-api.mjs`を利用します。Next.jsは本番コードと同じ`/api/v1/*`を呼び出し、E2E起動時だけ次へrewriteします。

```text
VAULTSEND_API_URL=http://127.0.0.1:8081
```

詳細は`docs/playwright-e2e-ja.md`を参照してください。

## タイムアウト

- Go: 15分
- PostgreSQL: 15分
- Web: 15分
- Playwright E2E: 20分

依存サービス待ちや無限ループによりRunnerを長時間占有しないための上限です。

## 失敗ログ

失敗時だけ各コマンドの出力をArtifactへ保存します。

- Go: `ci-go-failure-{run_id}-{run_attempt}`
- PostgreSQL: `ci-postgres-failure-{run_id}-{run_attempt}`
- Web: `ci-web-failure-{run_id}-{run_attempt}`
- Playwright E2E: `ci-e2e-failure-{run_id}-{run_attempt}`
- 保持期間: 7日

E2E失敗Artifactには次を含めます。

- コマンドログ
- HTML report
- trace
- screenshot
- video

成功時はArtifactを作成しません。

## 失敗時の確認順序

### Go

1. `Go依存関係を検証`
2. `Goフォーマットを確認`
3. `Go静的解析を実行`
4. `Goテストを実行`
5. `Go実行バイナリをビルド`

ローカル確認例:

```bash
export GOTOOLCHAIN=local
export GOFLAGS=-mod=readonly

go mod download
go mod verify
go mod tidy
git diff --exit-code -- go.mod go.sum

gofmt -w $(find . -type f -name '*.go' -not -path './vendor/*')
go vet ./...
go test -count=1 ./...
go build ./cmd/api ./cmd/worker ./cmd/cleanup-worker
```

依存更新作業では一時的に`GOFLAGS`を解除して`go mod tidy`を実行し、生成結果をコミットしてください。

### PostgreSQL

1. PostgreSQL service containerのhealth check
2. `PostgreSQL clientを確認`
3. `migrationのup・down・upを検証`
4. `Store integration testを実行`
5. 失敗Artifactの`migrations.log`または`store-integration.log`

ローカル確認例:

```bash
docker compose up -d postgres
make verify-migrations
make test-integration
```

`verify-migrations`はdown後にupを再適用するため、破棄可能なテスト専用DBで実行してください。

### Web

1. `Web依存関係をインストール`
2. `ESLintを実行`
3. `TypeScript型チェックを実行`
4. `Next.jsをビルド`

ローカル確認例:

```bash
cd web
npm ci --no-audit --no-fund
npm run lint
npm run typecheck
VAULTSEND_API_URL=http://localhost:8080 npm run build
```

### Playwright E2E

1. `Web依存関係をインストール`
2. `Chromiumをインストール`
3. `Playwright E2Eを実行`
4. Artifactの`test.log`で失敗specとlocatorを確認
5. HTML report・trace・screenshot・videoで画面状態とnetworkを確認

ローカル確認例:

```bash
cd web
npm ci
npx playwright install --with-deps chromium
npm run e2e
```

画面を見ながら再現する場合:

```bash
npm run e2e:headed
```

固定sleepを追加して回避せず、表示状態・API response・download eventなど原因に対応する待機条件を追加してください。

## Branch Protection推奨設定

PRマージ前に次のcheckを必須化します。

- `Go / format・vet・test・build`
- `PostgreSQL / migration・Store integration`
- `Web / lint・typecheck・build`
- `Playwright / 送信・受信E2E`

併せて以下を推奨します。

- Pull Requestを必須化
- branchを最新状態にしてからマージ
- unresolved conversationがあるPRのマージ禁止
- force push禁止
- branch削除禁止

Branch Protectionはリポジトリ設定変更であり、コード差分には含めません。

## 対象外

現在のCIには含めません。

- 実Go API・実PostgreSQL・実S3を利用するブラウザE2E
- LocalStackまたは実AWSを利用するS3・SQS・SES test
- Stripe webhook integration test
- Firefox・WebKit E2E
- Docker image build
- deployment
- CodeQLや依存脆弱性スキャン

## 次の改善候補

1. 実Go API・PostgreSQLを起動したHTTP E2E
2. 認証登録・ログイン・ログアウトE2E
3. 送信履歴・再送・削除E2E
4. multipart再試行・Grant期限切れのE2E
5. Firefox・WebKit projectの追加
6. Playwright accessibility snapshotの追加
7. CodeQLとDependabotの追加
8. Docker image buildとコンテナ起動確認
9. staging・production deployment workflowの追加
