# Multipart Upload送信ウィザード 設計・運用仕様

## 1. 目的

VaultSendの既存uploads/shipments APIを利用し、APIサーバーへファイル本体を中継せず、ブラウザからS3へ大容量ファイルを直接送信するWebフローを提供します。

## 2. 対象範囲

- ログインユーザー向け `/send`
- 1ファイル10GBまで
- 1 shipmentあたり20ファイルまで
- URL共有／受信者限定共有
- 有効期限1〜14日
- 最大ダウンロード回数1〜100回
- 任意パスワード
- 同一画面内のパート再試行

対象外:

- 画面再読み込み後のupload session復元
- multipart upload abort API
- S3 complete成功後、DB更新前に失敗したケースの自動修復
- organization選択UI
- 受信者向けダウンロード画面

## 3. フロー

1. ユーザーがファイルを選択する。
2. ブラウザが4MiB単位でファイルを読み込み、SHA-256を計算する。
3. `POST /v1/uploads` でupload sessionを作成する。
4. 返却されたPresigned URLへ、最大3パートを並列PUTする。
5. 各パートは最大3回まで指数バックオフ付きで再試行する。
6. 全パートのETagを `POST /v1/uploads/{id}/complete` へ渡す。
7. 全ファイル完了後、`POST /v1/shipments` でshipmentを確定する。

## 4. パートサイズ

既定値は8MiBです。ただし、ファイルサイズを既定値で分割するとPresigned URL上限1000件を超える場合、必要サイズをMiB単位で切り上げます。

例:

- 1GiB: 8MiB × 128パート
- 10GiB: 11MiB × 約931パート

これにより、API上の10GB上限と実際のmultipart session上限を一致させます。

## 5. 再試行と冪等性

### 5-1. S3 PUT

- 完了済みパートの `part_number` と `ETag` を画面状態へ保持します。
- 再試行時は未完了パートのみPUTします。
- Presigned URLの有効期限後は同じsessionを再利用しません。

### 5-2. Upload Complete API

`POST /v1/uploads/{id}/complete` は、既にDB上でcompletedの場合、既存の `file_id` / `shipment_id` を返します。

これにより、S3・DB更新成功後にHTTPレスポンスだけ失われた場合でも、クライアントは同じcomplete要求を安全に再試行できます。

ただし、S3 complete成功後かつDB更新前に失敗した場合、S3側のupload IDが消費済みになる可能性があります。このケースの自動修復は別タスクとします。

## 6. 所有権・整合性

### 6-1. Existing Shipmentへのファイル追加

`shipment_id`を指定してupload sessionを追加する場合、以下をサービス層で検証します。

- shipmentが存在すること
- statusが `draft` / `uploading` / `ready` であること
- `owner_user_id` が一致すること
- `organization_id` が一致すること

### 6-2. Upload Complete

owner付きupload sessionは、同一の認証ユーザーだけがcompleteできます。

匿名sessionとログインsessionのowner状態を暗黙に切り替えることは許可しません。

### 6-3. FileのShipment固定

`files.shipment_id` は作成後変更不可です。

migration `000011_files_shipment_immutable` のDB triggerにより、別shipmentへの付け替えをトランザクション単位で拒否します。

## 7. S3 CORS

ブラウザはS3レスポンスのETagを取得する必要があります。

```json
[
  {
    "AllowedHeaders": ["*"],
    "AllowedMethods": ["PUT"],
    "AllowedOrigins": ["https://app.example.com"],
    "ExposeHeaders": ["ETag"],
    "MaxAgeSeconds": 300
  }
]
```

本番では `AllowedOrigins` にワイルドカードを使わず、実際のフロントエンドOriginへ限定します。

## 8. エラー時の挙動

- ハッシュ計算失敗: 対象ファイルをfailedにして停止
- S3 PUT失敗: 最大3回再試行後、対象ファイルをfailedにして停止
- ETag非公開: CORS設定エラーを画面に表示
- complete失敗: 同一session・完了済みパートを維持して再試行可能
- shipment確定失敗: アップロード済みfile IDを維持し、送信設定画面で再実行可能
- 画面離脱中: `beforeunload` で警告

## 9. テスト観点

### Go

- 10GBファイルが1000パート以内になること
- ログイン時に `owner_type=user` でdraft shipmentを作成すること
- 別ownerのshipmentへ追加uploadできないこと
- 別ownerのupload sessionをcompleteできないこと
- completed sessionのcomplete再実行で既存file IDを返すこと
- completedだがfile IDがない不整合を409にすること
- `files.shipment_id` の変更がDB triggerで拒否されること

### Web

- 1件／20件／21件のファイル選択
- 0B／10GB／10GB超の境界値
- 同一ファイルの重複追加
- SHA-256進捗
- 並列PUTとパート再試行
- ETagを取得できないCORS設定
- URL期限切れ
- URL共有／受信者限定の入力差
- 受信者重複除去とメール形式
- 有効期限1日／14日
- 最大DL回数1回／100回
- モバイル幅とキーボード操作

## 10. 次の改善

1. multipart abort APIと期限切れsessionの自動abort
2. upload session再取得・Presigned URL再発行API
3. IndexedDB等を利用した画面再読み込み後の再開
4. S3 completeとDB更新間の補償処理
5. Playwrightによる主要送信フローE2E
6. organization選択と組織ストレージ使用量表示
