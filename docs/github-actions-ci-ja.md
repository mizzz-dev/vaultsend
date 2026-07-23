# GitHub Actions CI 設計・運用

## 目的

Pull Request と `main` へのpush時に、GoバックエンドとNext.js Webの最低限の品質ゲートを自動実行します。

CIで検出する対象は次のとおりです。

- Go依存定義とchecksumの不整合
- Goコードのフォーマット漏れ
- Goの静的解析エラー
- Goテスト失敗
- API・mail worker・cleanup workerのコンパイル失敗
- npm lockfileと`package.json`の不整合
- ESLintエラー
- TypeScript型エラー
- Next.js本番ビルド失敗

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

CIではAWS、Stripe、PostgreSQLなどの本番Secretを使用しません。

## Go Job

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

## Web Job

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

## タイムアウト

Go、Webともに15分でタイムアウトします。

依存サービス待ちや無限ループによりRunnerを長時間占有しないための上限です。

## 失敗ログ

Go・Webともに、失敗時だけ各コマンドの出力をArtifactへ保存します。

- Go: `ci-go-failure-{run_id}-{run_attempt}`
- Web: `ci-web-failure-{run_id}-{run_attempt}`
- 保持期間: 7日

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

## Branch Protection推奨設定

PRマージ前に次のcheckを必須化します。

- `Go / format・vet・test・build`
- `Web / lint・typecheck・build`

併せて以下を推奨します。

- Pull Requestを必須化
- branchを最新状態にしてからマージ
- unresolved conversationがあるPRのマージ禁止
- force push禁止
- branch削除禁止

Branch Protectionはリポジトリ設定変更であり、コード差分には含めません。

## 対象外

現在のCIには含めません。

- PostgreSQLを利用するintegration test
- LocalStackまたは実AWSを利用するS3・SQS・SES test
- Stripe webhook integration test
- Playwright E2E
- migrationのup/down実行検証
- Docker image build
- deployment
- CodeQLや依存脆弱性スキャン

## 次の改善候補

1. PostgreSQL service containerを利用したrepository層integration test
2. migrationのup/down実行検証
3. Playwrightによる送信・受信フローE2E
4. CodeQLとDependabotの追加
5. Docker image buildとコンテナ起動確認
6. staging・production deployment workflowの追加
