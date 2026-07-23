# PostgreSQL Migration・Store Integration Test

## 1. 目的

PostgreSQLの実インスタンスを使い、次の問題をPull Request時に検出します。

- migrationのSQL構文エラー
- up/down migrationの組み合わせ不足
- 最新schemaまでの適用失敗
- down migration後にテーブルやenumが残る不整合
- 手書きStore SQLのプレースホルダー・型・制約不整合
- 外部キー・cascade・triggerなどDB境界の回帰

通常のunit testとはJobを分離し、PostgreSQL依存の失敗箇所を短時間で切り分けられる構成です。

## 2. CI構成

GitHub Actionsの`PostgreSQL / migration・Store integration` Jobで、`postgres:16` service containerを起動します。

テスト用接続先:

```text
postgres://vaultsend:vaultsend@localhost:5432/vaultsend_test?sslmode=disable
```

本番DBや本番Secretは使用しません。

## 3. Migration検証

対象スクリプト:

```text
scripts/verify-migrations.sh
```

実行順序:

1. `*.up.sql`と`*.down.sql`の件数を確認
2. 各up migrationに対応するdown migrationがあることを確認
3. up migrationをファイル名昇順で全件適用
4. 主要テーブルの存在を確認
5. down migrationをファイル名降順で全件適用
6. `public` schemaにユーザーテーブル・enumが残っていないことを確認
7. 後続のStore integration test向けにup migrationを再適用

検証後のDBは最新schemaの状態になります。

## 4. Store Integration Test

対象ファイル:

```text
internal/store/postgres_integration_test.go
```

`integration` build tagで通常の`go test ./...`から分離しています。

現在の確認内容:

- `Queries.CreateShipment`でshipmentを保存できる
- `Queries.GetShipment`で保存内容を取得できる
- `Queries.CreateFile`でfileを保存できる
- `files.shipment_id`の付け替えがDB triggerにより`23514`で拒否される
- 付け替え失敗後も元shipmentとの関連が維持される
- `DeleteShipmentCascade`でshipmentとfileを削除できる
- 削除済みshipment取得時に`store.ErrNotFound`へ正規化される

## 5. ローカル実行

### 前提

- PostgreSQL 16互換環境
- `psql`
- 最新migration適用前のテスト用DB

Docker Composeを使う場合:

```bash
docker compose up -d postgres
```

### Migration検証

```bash
make verify-migrations
```

接続先を変更する場合:

```bash
make verify-migrations DB_URL='postgres://user:password@localhost:5432/vaultsend_test?sslmode=disable'
```

### Store Integration Test

migration検証後に実行します。

```bash
make test-integration
```

または直接実行します。

```bash
DATABASE_URL='postgres://vaultsend:vaultsend@localhost:5432/vaultsend?sslmode=disable' \
  go test -tags=integration -count=1 -v ./internal/store
```

## 6. テストデータの扱い

- テスト専用shipment・fileを毎回作成します。
- shipment削除時のcascadeを確認します。
- `t.Cleanup`でも残存shipmentを削除します。
- 並列実行を前提に、storage keyへUUIDを含めます。
- 本番・共有開発DBではなく、破棄可能なテスト専用DBを使用してください。

## 7. 失敗時の確認順序

1. PostgreSQL service containerのhealth check
2. `PostgreSQL clientを確認`
3. `migrationのup・down・upを検証`
4. `Store integration testを実行`
5. 失敗Artifact `ci-postgres-failure-{run_id}-{run_attempt}`

Artifactには次を保存します。

```text
.ci-logs/postgres/migrations.log
.ci-logs/postgres/store-integration.log
```

保持期間は7日です。

## 8. Migration追加時のルール

- up/downを必ず同じPRで追加する
- ファイル名は既存のゼロ埋め連番を維持する
- down migrationは後続migrationから逆順に実行できる内容にする
- migrationで追加したtable・enum・trigger・function・constraintをdownで戻す
- 既存データを破壊する変更はexpand/contractを検討する
- integration testで検証すべき制約を追加した場合はテストも同じPRで更新する

## 9. 対象外

このJobでは次を扱いません。

- PostgreSQLの負荷・性能試験
- 複数PostgreSQLバージョンのmatrix test
- 実運用データを使ったmigration時間計測
- AWS・S3・SQS・SES integration test
- Stripe integration test
- APIを起動したE2E test

## 10. 次の改善候補

1. transaction・競合・同時更新を含むStore integration test拡充
2. migration前後のschema snapshot比較
3. PostgreSQL 17を含むmatrix test
4. API・PostgreSQLを組み合わせたHTTP integration test
5. test fixture builderの共通化
