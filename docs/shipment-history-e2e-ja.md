# 送信履歴 Playwright E2E 設計・運用

## 目的

送信履歴画面の一覧表示、検索、状態絞り込み、詳細表示、通知再送、論理削除を実ブラウザ操作で検証します。

既存の送信・受信E2Eがファイル送信と受信リンクを対象としているのに対し、本テストは送信後の運用導線を対象にします。

## 対象

対象ファイル:

```text
web/e2e/specs/shipment-history.spec.ts
```

対象画面:

```text
/shipments
```

## 検証シナリオ

### 一覧・検索・絞り込み・詳細

1. 認証済みユーザーとして送信履歴を表示
2. 送信状況サマリーと3件のfixtureを確認
3. 初期選択された送信の詳細を確認
4. shipment IDで検索
5. 検索結果から別の送信を選択
6. 状態を`accessed`へ絞り込み
7. 条件不一致時のempty stateを確認
8. page errorと予期しないconsole errorがないことを確認

### 通知再送・論理削除

1. 受信者限定shipmentの詳細を表示
2. 通知再送APIが`202 Accepted`を返すことを確認
3. 再取得後に受信者の通知回数が増えることを確認
4. 削除確認ダイアログの文言を確認
5. 論理削除APIが成功することを確認
6. 一覧再取得後に状態が`deleted`になることを確認
7. 削除済みshipmentでは再送・削除操作を表示しないことを確認

## API fixture方針

送信履歴専用のAPI応答はPlaywrightの`page.route`で提供します。

対象API:

```text
GET    /api/v1/auth/me
GET    /api/v1/shipments
GET    /api/v1/shipments/{id}
POST   /api/v1/shipments/{id}/resend
DELETE /api/v1/shipments/{id}
```

### 採用理由

- 送信・受信フロー用mock APIへ送信履歴の状態管理を混在させない
- DesktopとMobile、各テスト間で再送回数や削除状態を共有しない
- API request method、path、body、statusをテスト内で明示できる
- 本番UIとAPIクライアントへE2E専用分岐を追加しない

## Fixture

以下の3種類を使用します。

| ID | 共有方法 | 状態 | 主な用途 |
|---|---|---|---|
| `shipment-history-1` | 受信者限定 | 送信済み | 詳細、再送、削除 |
| `shipment-history-2` | URL共有 | アクセス済み | ID検索、状態絞り込み |
| `shipment-history-3` | 受信者限定 | 期限切れ | 一覧の状態差異 |

固定日時を使用し、テスト実行時刻によって表示内容や状態が変わらないようにします。

## 状態変更

各テストでfixtureを新規生成します。

通知再送時:

- `notification_summary.total_notifications`を加算
- `notification_summary.queued_count`を加算
- 受信者の`notification_count`を加算
- 最終通知状態を`queued`へ更新

論理削除時:

- 詳細の`status`を`deleted`へ更新
- 一覧項目の`status`を`deleted`へ更新
- データ自体は一覧から除外しない

これはバックエンドの論理削除仕様に合わせたものです。

## 実行方法

```bash
cd web
npm run e2e
```

送信履歴だけを実行する場合:

```bash
npx playwright test e2e/specs/shipment-history.spec.ts
```

画面を表示して確認する場合:

```bash
npx playwright test e2e/specs/shipment-history.spec.ts --headed
```

## テスト観点

### 正常系

- 一覧・詳細の表示
- 件名・ID検索
- 状態絞り込み
- 通知再送
- 論理削除

### 異常系・境界

- 条件に一致する履歴がない場合
- URL共有では再送ボタンを表示しないこと
- 削除済みでは再送・削除ボタンを表示しないこと
- 未定義のmethod・pathは成功扱いしないこと
- 再送request bodyに`recipient_ids`がない場合は400とすること

## 影響範囲

- Playwrightのテストケース数
- GitHub ActionsのE2E Job実行時間
- 送信履歴画面・API契約変更時のfixture保守

本番コード、Go API、DB、AWS、Stripe、メール送信には変更を加えません。

## 未対応

- 実Go API・PostgreSQLを利用したHTTP E2E
- ページングで2ページ目へ移動するE2E
- API 401時の認証画面リダイレクト
- 再送失敗・削除失敗のUI表示
- 複数受信者の一部再送
