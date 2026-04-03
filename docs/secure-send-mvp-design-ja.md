# SecureSend（仮称）設計・実装方針（MVP + 拡張前提）

## 1. 要件の不足・曖昧点の洗い出し（抜け漏れ・矛盾の先出し）

以下は実装前に必ず確定すべき論点。未確定項目は「仮置き」で設計に反映する。

### 1-1. プロダクト方針の曖昧点
- **匿名送信と会員送信の優先順位**が未確定。
  - 要件では匿名送信必須、同時に送信履歴管理も重視。匿名中心にすると履歴永続管理は弱くなる。
- **MVPの収益導線**（無料のみか、容量制限付き有料化の布石を入れるか）が未定。
- **受信者限定アクセスの強度**が未定（メールリンクのみ / 追加パスコード / ログイン必須）。

### 1-2. セキュリティ・法務の未確定
- **違法コンテンツ対応SLA**（通報から何時間で一次対応するか）が未定。
- **データ保持期間**が具体値未定（監査ログ、メール履歴、IPログ）。
- **利用規約で禁止するファイル種別**（著作権侵害物、マルウェア等）の明文化が未定。
- **個人情報保護法対応の運用設計**（開示請求/削除請求フロー）が未定。

### 1-3. UXと運用の矛盾リスク
- 「3クリックで完了」と「細かい安全設定」はトレードオフ。
  - 解決には**デフォルト安全設定 + 詳細は折りたたみ**が必要。
- 「差し替え」機能は監査的に「同一リンクで別ファイル」問題を生む。
  - 法人利用を見据えるなら、差し替え時に**バージョン管理**か**新規発行 + 旧無効化**を明確化すべき。

### 1-4. 技術要件の未確定
- 最大ファイルサイズ（1ファイル/合計）が未定。
- 受信者数上限（1送信あたり）が未定。
- 期限の最短/最長（例: 1時間〜30日）が未定。
- ダウンロード回数上限（1〜999等）が未定。
- メールプロバイダの第一候補（SES/Resend/SendGrid）未確定。

---

## 2. 前提 assumptions と確認事項

## 2-1. 仮置き前提（MVP）
- リージョン: AWS ap-northeast-1（東京）単一リージョン。
- 最大ファイルサイズ: 1ファイル20GB、1送信合計50GB（仮置き）。
- 期限: 1日〜14日（デフォルト7日）。
- ダウンロード回数: 1〜20回（デフォルト10回）。
- 受信者数: 1送信あたり最大20件。
- 匿名送信: 許可。ただし匿名は履歴保持を短くし、操作用トークンで管理。
- 会員送信: メールアドレス + Magic Linkを推奨（MVPでパスワード運用コストを下げる）。

## 2-2. 要確認事項（意思決定が必要）
1. 匿名送信で「送信後の削除」をどう保証するか
   - 候補: 作成時に管理用シークレットURLを返す方式。
2. 受信者限定アクセスの最小要件
   - 候補: メールリンク + 追加の短い受取コード（6桁）を任意化。
3. ファイル差し替えの扱い
   - 推奨: MVPは「再発行」で代替し、差し替えは次フェーズ。
4. 不正利用対策
   - CAPTCHAv3導入タイミング、匿名送信のレート上限。
5. 保持期間
   - 例: 監査ログ180日、メール送信ログ90日、アクセスログ30日（仮置き）。

---

## 3. 推奨アーキテクチャ

## 3-1. 推奨案（MVP最適）
- **Frontend**: Next.js (TypeScript, App Router) + React Query + Zod
- **Backend API**: Go (Gin/Fiber/Echoのいずれか) + sqlc or ent
- **DB**: PostgreSQL (RDS or Aurora Serverless v2)
- **Object Storage**: S3（将来R2/MinIOへ抽象化）
- **Queue/Async**: SQS（メール送信・監査イベント非同期化）
- **Mail**: AWS SES（コスト優位）
- **Cache/Rate limit**: Redis (ElastiCache/Upstash)
- **Observability**: CloudWatch + OpenTelemetry + 構造化JSONログ

### 3-1-1. 構成の意図
- 大容量ファイルは**アプリサーバーを経由せず**、クライアントからS3へ直接アップロード（Presigned URL）。
- APIはメタデータ管理と認可判定に集中。
- メール送信・監査書き込みはジョブ化し、API応答を速く保つ。

### 3-1-2. アップロード方式（比較）
1. **推奨: S3 Multipart + Presigned URL（Resumable対応）**
   - 長所: 大容量・再開・並列アップロードに強い。
   - 短所: 実装は単純PUTより複雑。
2. 代替: 単一Presigned PUT
   - 長所: 実装容易。
   - 短所: 大容量で失敗時の再送コスト大。
3. 代替: tusプロトコル
   - 長所: 汎用再開機構。
   - 短所: S3直アップロードとの整合で追加コンポーネントが必要。

**採用基準**: 20GB級を想定するためMVPからMultipartを採用。

### 3-1-3. ダウンロード方式（推奨）
- 受信ページアクセス時にAPIが制約判定（期限/回数/受信者一致/パスワード）を実施。
- 条件を満たした場合のみ**短TTL（60秒）Presigned GET**を発行。
- 直リンク悪用防止のため、都度判定 + 短TTL + 回数消費をトランザクションで実施。

### 3-1-4. 匿名送信とログイン送信の両立
- `shipments.owner_type` を `anonymous | user` で統一。
- 匿名は「管理トークン」で編集・削除を許可。
- ログインユーザーはアカウントに紐づく履歴を永続表示。

### 3-1-5. AWS / R2 / MinIO比較（要点）
- AWS S3: エコシステム・運用実績が最も強い（推奨）。
- R2: egress優位。ただし機能差や運用知見は要検証。
- MinIO: 自前運用自由度は高いがMVPで運用負荷が重い。

---

## 4. データモデル案（URL共有型と受信者限定型を統一）

## 4-1. エンティティ
1. `users`
2. `shipments`（送信単位）
3. `files`（送信に紐づく複数ファイル）
4. `recipients`（受信者指定。URL共有型は0件で運用）
5. `access_tokens`（受信リンク/管理リンク/認証フロー）
6. `download_events`
7. `audit_logs`
8. `email_events`
9. `upload_sessions`（multipart管理）

## 4-2. 主要テーブル（抜粋）

### shipments
- id (ULID)
- owner_type (`anonymous`/`user`)
- owner_user_id (nullable)
- title
- message
- share_mode (`public_link`/`recipient_restricted`)
- password_hash (nullable, Argon2id)
- expires_at
- max_downloads
- current_downloads
- status (`draft`/`active`/`expired`/`deleted`)
- created_at, updated_at, deleted_at

### files
- id
- shipment_id
- original_name
- size_bytes
- mime_type
- storage_bucket
- storage_key
- checksum_sha256
- upload_status (`initiated`/`uploaded`/`verified`/`failed`)
- virus_scan_status (`pending`/`clean`/`infected`/`skipped`)
- created_at

### recipients
- id
- shipment_id
- email
- normalized_email
- access_mode (`magic_link`/`email_match`/`onetime`)
- status (`pending`/`notified`/`downloaded`/`blocked`)
- last_notified_at

### access_tokens
- id
- token_hash
- token_type (`download`/`manage`/`auth_magic_link`)
- shipment_id
- recipient_id (nullable)
- expires_at
- max_uses
- used_count
- revoked_at

### download_events
- id
- shipment_id
- file_id
- recipient_id (nullable)
- ip_hash
- user_agent
- result (`success`/`expired`/`over_limit`/`invalid_token`/`auth_failed`)
- created_at

## 4-3. ライフサイクル設計
- `draft` 作成 → multipart upload → `uploaded`検証 → 送信確定で `active`。
- 期限切れで `expired`。
- ユーザー削除/管理削除で `deleted`（論理削除） + 非同期でオブジェクト物理削除。
- 物理削除完了後に tombstone ログ保持。

---

## 5. API設計案（REST）

## 5-1. 認証
- 送信者（ログイン）: `/auth/magic-link/request`, `/auth/magic-link/verify`
- 管理用（匿名送信者）: 送信作成時に `manage_token` 発行
- 受信者: `download_token` + 必要ならメール一致チェック

## 5-2. 主要エンドポイント

### アップロード
- `POST /v1/uploads/initiate`
  - 入力: ファイル名, サイズ, MIME, checksum
  - 出力: upload_session_id, multipart_upload_id, part_url群
- `POST /v1/uploads/{id}/parts/complete`
- `POST /v1/uploads/{id}/finalize`

### 送信
- `POST /v1/shipments`
  - draft作成（設定 + ファイル紐付け）
- `POST /v1/shipments/{id}/activate`
  - URL発行または受信者通知を実行
- `GET /v1/shipments/{id}`
- `GET /v1/shipments?status=&cursor=`
- `POST /v1/shipments/{id}/resend`
- `DELETE /v1/shipments/{id}`

### 受信者ダウンロード
- `POST /v1/shipments/{id}/access/verify`
  - パスワード/受信者条件を検証
- `POST /v1/shipments/{id}/downloads/issue-url`
  - 短TTL presigned GETを発行

### 監査・運用
- `GET /v1/admin/audit-logs`
- `POST /v1/admin/shipments/{id}/force-delete`

## 5-3. API設計ポリシー
- すべての変更系APIに `Idempotency-Key` 対応。
- エラーコードは機械可読な `code` を必須化。
- 監査対象APIは actor, resource, action を強制記録。

---

## 6. 主要画面一覧（MVP）

1. **トップ/アップロード画面**
   - ファイル選択（ドラッグ&ドロップ）
   - 進捗、再開、失敗時リトライ
2. **送信設定画面**
   - 共有方法（URL共有 / 受信者限定）
   - 受信者メール、期限、回数、パスワード、件名、本文
3. **送信完了画面**
   - 共有URL、コピー、再通知、管理リンク表示
4. **受信者ダウンロード画面**
   - 条件チェック、ファイル一覧、ダウンロード実行
5. **送信履歴画面（ログイン時）**
   - ステータス、受信状況、削除、再通知
6. **ログイン画面（Magic Link）**
7. **最小管理画面**
   - 通報対応、削除、メール失敗キュー再実行

UX方針: デフォルト値で即送信可能。詳細設定は「詳細オプション」に格納。

---

## 7. 実装ステップを小さく分解した開発計画（6〜10週間想定）

## 7-1. フェーズ分割

### Phase 0（Week 1）
- 要件確定ワークショップ（保持期間、上限値、認証方針）
- ADR作成（upload方式、認証方式、削除方式）
- IaC雛形、CI/CD、静的解析

### Phase 1（Week 2-3）
- Go API土台（認証、shipment CRUD、監査ログ）
- DBスキーマv1 + migration
- Next.js基本UI（アップロード〜送信完了導線）

### Phase 2（Week 3-5）
- Multipart upload実装（initiate/complete/finalize）
- 受信者限定アクセス実装
- Presigned GET発行と制約判定

### Phase 3（Week 5-6）
- メール送信（SES） + 再送ジョブ
- 送信履歴・DL履歴画面
- レート制限 + 基本不正利用対策

### Phase 4（Week 7-8）
- 障害系処理（再試行、DL失敗導線、期限切れUI）
- 監視ダッシュボード、アラート
- 運用Runbook整備

### Phase 5（Week 9-10, バッファ）
- 負荷試験（upload/DL/mail）
- セキュリティ確認（認可、トークン、リンク悪用）
- リリース判定

## 7-2. MVP受け入れゲート
- P0障害ゼロ（アップロード不能、DL不能、期限判定不整合は不可）
- 主要シナリオ成功率 > 99%（社内試験）
- 監査ログ欠損率 0%

---

## 8. 技術的リスクと回避策

1. **大容量アップロード失敗率が高い**
   - 回避: multipart + part再送 + クライアント側checkpoint。
2. **受信者限定が形骸化する**
   - 回避: トークン単体依存を避け、メール一致/ワンタイムのモードを準備。
3. **メール不達でUX崩壊**
   - 回避: ドメイン認証（SPF/DKIM/DMARC）と再送キュー、失敗可視化。
4. **削除漏れによるコスト増**
   - 回避: DB TTLジョブ + S3 Lifecycle + 定期整合チェック。
5. **匿名悪用（スパム/違法配布）**
   - 回避: IP/ASNレート制限、ファイル種別制限、通報→凍結運用。
6. **将来拡張時のスキーマ破綻**
   - 回避: owner/tenant拡張余地を初期からカラム確保。

---

## 9. MVP案と将来拡張案

## 9-1. MVP優先案（推奨）
- 匿名送信 + ログイン送信（Magic Link）
- URL共有/受信者限定を同一`shipments`モデルで実装
- Multipart upload
- 期限/回数/パスワード/削除/再送
- 最小管理画面（削除、通報対応、メール失敗確認）

## 9-2. 代替MVP案（さらに短納期）
- 匿名送信のみに絞る + 管理トークン運用
- ログイン/履歴はβで追加

### 採用基準
- 6週間以内を厳守するなら代替案。
- 小規模法人への訴求を初期から取りに行くなら推奨案。

## 9-3. 将来拡張ポイント
- 組織/チーム/ロール（RBAC）
- SSO（OIDC/SAML）
- テナント課金（容量・送信数）
- 監査ログCSVエクスポート
- DLP/AVスキャン強化
- 外部API/Webhook

---

## 10. サンプルコード（Go + TypeScript, 方針確認用）

## 10-1. Go: ダウンロードURL発行前の制約判定（概略）

```go
func (s *Service) IssueDownloadURL(ctx context.Context, shipmentID string, recipientEmail *string) (string, error) {
    sh, err := s.repo.GetShipmentForUpdate(ctx, shipmentID)
    if err != nil { return "", err }

    now := time.Now().UTC()
    if sh.Status != "active" || now.After(sh.ExpiresAt) {
        return "", ErrExpired
    }
    if sh.CurrentDownloads >= sh.MaxDownloads {
        return "", ErrOverLimit
    }
    if sh.ShareMode == "recipient_restricted" {
        if recipientEmail == nil || !s.repo.IsAllowedRecipient(ctx, shipmentID, *recipientEmail) {
            return "", ErrForbiddenRecipient
        }
    }

    // 回数消費は発行時ではなくDL確定時でも可（要件次第）
    // ここでは発行時消費の例
    if err := s.repo.IncrementDownloadCount(ctx, shipmentID); err != nil {
        return "", err
    }

    return s.objectStore.PresignGet(ctx, sh.StorageKey, 60*time.Second)
}
```

## 10-2. TypeScript: uploadセッション開始（概略）

```ts
export async function initiateUpload(input: {
  fileName: string;
  sizeBytes: number;
  mimeType: string;
  checksumSha256: string;
}) {
  const res = await fetch('/api/v1/uploads/initiate', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  });

  if (!res.ok) {
    const e = await res.json();
    throw new Error(`initiate failed: ${e.code}`);
  }
  return res.json() as Promise<{
    uploadSessionId: string;
    multipartUploadId: string;
    parts: Array<{ partNumber: number; presignedUrl: string }>;
  }>;
}
```

---

## 補足: 削除・保持ポリシー（仮置き）
- ファイル本体: 期限到来 + 24時間以内に非同期削除。
- 論理削除済みメタデータ: 90日保持。
- 監査ログ: 180日（法人プランで1年拡張）。
- メール送信ログ: 90日。
- IP生データは短期保持、分析用途はハッシュ化して保持。

## 補足: 監視KPI（MVP）
- API 5xx率
- upload開始→完了率
- DL発行成功率
- メール送信成功率/遅延
- 期限切れ/回数超過/認証失敗比率
- ストレージ使用量増分（日次）
