# 受信リンク・ダウンロードフロー 設計／運用仕様

## 1. 目的

受信者が `/r/{token}` からshipment情報を確認し、必要に応じてパスワードを検証した上で、S3の短命Presigned URLからファイルを取得できる状態にします。

画面だけでパスワード入力を制御せず、ダウンロードURL発行API側でも検証済み状態を必須にします。

## 2. 対象範囲

- URL共有／受信者限定共有
- token・shipment・期限・回数上限の確認
- パスワード検証
- HttpOnly Access Grant Cookie
- ファイル一覧と受信情報表示
- ファイル単位の短命ダウンロードURL発行
- 期限切れ、無効化、上限超過、レート制限のエラー表示

対象外:

- ダウンロード完了コールバック
- 複数ファイルのZIP一括ダウンロード
- ダウンロード中断時の回数消費取り消し
- Access GrantのDB保存
- Playwright E2E

## 3. セキュリティ上の問題と対策

### 3-1. 修正前の問題

`POST /v1/access/{token}/verify` はパスワードの正否を返すだけで、`GET /v1/files/{id}/download-url` は検証済み状態を確認していませんでした。

そのため、画面上でパスワード入力を要求しても、APIを直接呼び出すことでパスワード検証を迂回できる状態でした。

### 3-2. 修正後

パスワード検証成功時、APIは以下を含む短命payloadへHMAC-SHA256署名します。

- access tokenのSHA-256 hash
- Grant有効期限

署名付きGrantはHttpOnly Cookieとして返します。

ダウンロードURL発行APIは、パスワード付きshipmentの場合に以下を検証します。

1. Grant Cookieが存在する
2. HMAC署名が正しい
3. Grant内のtoken hashがリクエストtokenと一致する
4. Grantが期限内である
5. 元のaccess tokenとshipmentも有効である

検証に失敗した場合は `401 access_verification_required` を返します。

## 4. Access Grant Cookie

Cookie名:

```text
vaultsend_access_grant_{token_sha256先頭16桁}
```

属性:

- `HttpOnly=true`
- `Secure=COOKIE_SECURE`
- `SameSite=COOKIE_SAMESITE`
- `Domain=COOKIE_DOMAIN`
- `Path=/`
- `Expires=min(now + ACCESS_GRANT_TTL_SEC, token.expires_at, shipment.expires_at)`

tokenごとにCookie名を分けるため、同じブラウザで複数の受信リンクを開いてもGrantが上書きされません。

## 5. 環境変数

### ACCESS_GRANT_SECRET

- APIプロセスで必須
- 32バイト以上
- HMAC-SHA256署名鍵
- 本番ではSecret Manager等から注入
- ログ出力、ブラウザ露出、リポジトリ保存は禁止
- mail worker / cleanup workerでは必須にしない

`APP_ENV=local|test` かつ未設定の場合のみ、開発用固定値を使用します。

### ACCESS_GRANT_TTL_SEC

- 既定: `600`秒
- 正の整数のみ
- 長くしすぎると、パスワード変更後も既存Grantが有効な時間が増えます

## 6. APIフロー

### 6-1. Inspect

```http
GET /v1/access/{token}
```

返却内容:

- `requires_password`
- shipment件名、メッセージ、共有方式、期限、最大DL回数
- ファイルID、名前、サイズ

### 6-2. Verify

```http
POST /v1/access/{token}/verify
Content-Type: application/json

{"password":"..."}
```

パスワード付きshipment:

- 成功時にAccess Grant Cookieを発行
- レスポンスは `granted=true` とGrant期限
- Grant本体はJSONへ含めない

パスワードなしshipment:

- `granted=true`
- Grant Cookieは不要

### 6-3. Download URL

```http
GET /v1/files/{file_id}/download-url?access_token={token}
```

パスワード付きshipmentではAccess Grant Cookieが必須です。

成功時:

- S3 Presigned URL
- Presigned URL有効期限

## 7. 状態別UI

- 読み込み中: tokenとshipmentを確認中
- 正常・パスワードなし: ファイル一覧を表示
- 正常・パスワードあり・未検証: パスワードフォーム
- 検証済み: ファイル一覧と確認済み表示
- Grant期限切れ: パスワードフォームへ戻す
- token/shipment期限切れ: 再送依頼を案内
- revoked/deleted: 利用不可を案内
- 回数上限: 再送依頼を案内
- verify locked: 時間をおいた再試行を案内
- download rate limited: 一時的な再試行を案内

## 8. ログ・プライバシー

ログへ出してはいけない情報:

- raw access token
- password
- Access Grant Cookie値
- Presigned URL
- ACCESS_GRANT_SECRET

既存の監査ログではtoken hash、IP hash、結果のみを利用します。

## 9. テスト観点

### Service

- 正しいパスワードでGrantを発行
- 不正パスワードでGrantを発行しない
- Grantなしで保護ファイルを取得できない
- 正しいGrantで取得できる
- 改ざんGrantを拒否
- 期限切れGrantを拒否
- 別tokenのGrantを拒否
- 別署名鍵のGrantを拒否
- パスワードなしshipmentは従来どおり取得可能
- 検証失敗時にtoken利用回数とdownload eventを更新しない

### Handler

- Grant CookieがHttpOnly/Secure/SameSite設定を持つ
- tokenごとにCookie名が異なる
- Cookieなしの保護ファイル取得は401
- 不正file IDは400

### Web

- パスワードあり／なし
- 正しい／不正パスワード
- Grant期限切れからの再検証
- 期限切れ、無効化、上限超過
- ファイル0件／1件／複数件
- 長い件名・ファイル名・メッセージ
- 320px幅、キーボード操作、フォーカス表示

## 10. 運用上の注意

- APIを複数台構成にしてもGrantはステートレス検証でき、共有メモリやsticky sessionは不要です。
- `ACCESS_GRANT_SECRET` を変更すると既存Grantは即時無効になります。
- Cookie DomainはNext.js proxy経由のフロントエンドOriginで受け入れ可能な値にします。
- ダウンロード回数はPresigned URL発行時に消費します。実ダウンロード完了を厳密に計測する場合はCloudFront/S3イベント等を利用する別設計が必要です。
