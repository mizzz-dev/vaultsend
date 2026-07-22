# GitHub Actions CI 設計・運用

## 目的

Pull Request と `main` へのpush時に、GoバックエンドとNext.js Webの最低限の品質ゲートを自動実行します。

CIで検出する対象は次のとおりです。

- Goコードのフォーマット漏れ
- Goの静的解析エラー
- Goテスト失敗
- API・mail worker・cleanup workerのコンパイル失敗
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

Goのバージョンは`go.mod`の`go`ディレクティブを参照します。

実行内容:

1. `go mod download`
2. `gofmt`適用漏れ確認
3. `go vet ./...`
4. `go test -count=1 ./...`
5. 以下の実行バイナリをbuild
   - `./cmd/api`
   - `./cmd/worker`
   - `./cmd/cleanup-worker`

### 現在の依存解決方針

現状の`go.sum`には一部モジュールの完全なchecksumが不足しているため、CIでは一時的に以下を設定しています。

```yaml
GOFLAGS: -mod=mod
```

これによりGitHub Actions上で不足checksumを解決してテストを実行します。

ただし、再現性を高めるため、後続タスクで開発環境から`go mod tidy`を実行し、更新した`go.mod`と`go.sum`をコミットする必要があります。完全化後はCIを次へ変更します。

```yaml
GOFLAGS: -mod=readonly
```

## Web Job

Node.jsは`web/package.json`の要件に合わせて`20.19.0`を使用します。

実行内容:

1. `npm install --no-audit --no-fund`
2. `npm run lint`
3. `npm run typecheck`
4. `npm run build`

Next.js build時のAPI接続先には、build時の設定値として以下を指定します。

```text
VAULTSEND_API_URL=http://localhost:8080
```

APIサーバーへの実通信は行いません。

### 現在の依存解決方針

現状は`web/package-lock.json`が存在しないため、CIでは`npm ci`を使用できません。

一時的に`npm install`を使用し、npmキャッシュは`web/package.json`のhashをkeyにしています。

後続タスクで`package-lock.json`を生成・コミットした後、CIを次へ変更します。

```bash
npm ci --no-audit --no-fund
```

キャッシュkeyも`web/package-lock.json`のhashを利用します。

## タイムアウト

Go、Webともに15分でタイムアウトします。

依存サービス待ちや無限ループによりRunnerを長時間占有しないための上限です。

## 失敗時の確認順序

### Go

1. `Go依存関係を取得`
2. `Goフォーマットを確認`
3. `Go静的解析を実行`
4. `Goテストを実行`
5. `Go実行バイナリをビルド`

ローカル確認例:

```bash
go mod download

gofmt -w $(find . -type f -name '*.go' -not -path './vendor/*')
go vet ./...
go test -count=1 ./...
go build ./cmd/api ./cmd/worker ./cmd/cleanup-worker
```

### Web

1. `Web依存関係をインストール`
2. `ESLintを実行`
3. `TypeScript型チェックを実行`
4. `Next.jsをビルド`

ローカル確認例:

```bash
cd web
npm install --no-audit --no-fund
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

Branch Protectionはリポジトリ設定変更であり、このPRのコード差分には含めません。

## 対象外

今回のCIには含めません。

- PostgreSQLを利用するintegration test
- LocalStackまたは実AWSを利用するS3・SQS・SES test
- Stripe webhook integration test
- Playwright E2E
- migrationのup/down実行検証
- Docker image build
- deployment
- CodeQLや依存脆弱性スキャン

## 次の改善候補

1. `go mod tidy`による`go.sum`完全化と`-mod=readonly`への変更
2. `package-lock.json`追加と`npm ci`への変更
3. PostgreSQL service containerを利用したrepository層integration test
4. Playwrightによる送信・受信フローE2E
5. CodeQLとDependabotの追加
6. Docker image buildとコンテナ起動確認
