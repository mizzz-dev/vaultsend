package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrNotFound = errors.New("store: not found")
var ErrConflict = errors.New("store: conflict")

// TODO: 次PRで sqlc generated code に置き換え、手書きSQLを段階的に削除する。

type CreateShipmentParams struct {
	OwnerType    string
	OwnerUserID  *uuid.UUID
	Status       string
	ShareMode    string
	Title        string
	Message      *string
	MaxDownloads int32
	ExpiresAt    time.Time
}

type Shipment struct {
	ID               uuid.UUID `json:"id"`
	OwnerType        string    `json:"owner_type"`
	OwnerUserID      *uuid.UUID
	Status           string `json:"status"`
	ShareMode        string `json:"share_mode"`
	Title            string `json:"title"`
	Message          *string
	PasswordHash     *string
	MaxDownloads     int32     `json:"max_downloads"`
	CurrentDownloads int32     `json:"current_downloads"`
	ExpiresAt        time.Time `json:"expires_at"`
	SentAt           *time.Time
	RevokedAt        *time.Time
	DeletedAt        *time.Time
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (q *Queries) CreateShipment(ctx context.Context, arg CreateShipmentParams) (Shipment, error) {
	return q.createShipment(ctx, q.db, arg)
}

func (q *Queries) createShipment(ctx context.Context, db dbtx, arg CreateShipmentParams) (Shipment, error) {
	const query = `
INSERT INTO shipments (
    owner_type, owner_user_id, status, share_mode, title, message, max_downloads, expires_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING id, owner_type, owner_user_id, status, share_mode, title, message, password_hash, max_downloads,
          current_downloads, expires_at, sent_at, revoked_at, deleted_at, created_at, updated_at`
	row := db.QueryRow(ctx, query,
		arg.OwnerType,
		arg.OwnerUserID,
		arg.Status,
		arg.ShareMode,
		arg.Title,
		arg.Message,
		arg.MaxDownloads,
		arg.ExpiresAt,
	)
	var s Shipment
	err := scanShipment(row, &s)
	return s, err
}

func (q *Queries) GetShipment(ctx context.Context, id uuid.UUID) (Shipment, error) {
	const query = `
SELECT id, owner_type, owner_user_id, status, share_mode, title, message, password_hash, max_downloads,
       current_downloads, expires_at, sent_at, revoked_at, deleted_at, created_at, updated_at
FROM shipments
WHERE id = $1`
	row := q.db.QueryRow(ctx, query, id)
	var s Shipment
	err := scanShipment(row, &s)
	if errors.Is(err, pgx.ErrNoRows) {
		return Shipment{}, ErrNotFound
	}
	return s, err
}

type FinalizeShipmentParams struct {
	ShipmentID               uuid.UUID
	ExpectedStatuses         []string
	Title                    string
	Message                  *string
	ShareMode                string
	Status                   string
	ExpiresAt                time.Time
	MaxDownloads             int32
	PasswordHash             *string
	OwnerUserID              *uuid.UUID
	FileIDs                  []uuid.UUID
	Recipients               []CreateRecipientParams
	AccessTokens             []CreateAccessTokenParams
	PlainRecipientTokenByKey map[string]string
}

type ShipmentFinalizeResult struct {
	Shipment   Shipment
	Files      []File
	Recipients []Recipient
}

func (q *Queries) FinalizeShipment(ctx context.Context, arg FinalizeShipmentParams) (ShipmentFinalizeResult, error) {
	tx, err := q.db.Begin(ctx)
	if err != nil {
		return ShipmentFinalizeResult{}, err
	}
	defer tx.Rollback(ctx)

	const lockQuery = `SELECT id, owner_type, owner_user_id, status, share_mode, title, message, password_hash, max_downloads,
       current_downloads, expires_at, sent_at, revoked_at, deleted_at, created_at, updated_at
FROM shipments WHERE id = $1 FOR UPDATE`
	var current Shipment
	if err := scanShipment(tx.QueryRow(ctx, lockQuery, arg.ShipmentID), &current); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ShipmentFinalizeResult{}, ErrNotFound
		}
		return ShipmentFinalizeResult{}, err
	}
	if len(arg.ExpectedStatuses) > 0 {
		ok := false
		for _, st := range arg.ExpectedStatuses {
			if current.Status == st {
				ok = true
				break
			}
		}
		if !ok {
			return ShipmentFinalizeResult{}, ErrConflict
		}
	}

	const updateShipment = `
UPDATE shipments
SET title=$2, message=$3, share_mode=$4, status=$5, expires_at=$6, max_downloads=$7, password_hash=$8, owner_user_id=COALESCE($9, owner_user_id), sent_at=CASE WHEN $5 = 'sent' THEN now() ELSE sent_at END
WHERE id=$1
RETURNING id, owner_type, owner_user_id, status, share_mode, title, message, password_hash, max_downloads,
          current_downloads, expires_at, sent_at, revoked_at, deleted_at, created_at, updated_at`
	var shipment Shipment
	if err := scanShipment(tx.QueryRow(ctx, updateShipment,
		arg.ShipmentID,
		arg.Title,
		arg.Message,
		arg.ShareMode,
		arg.Status,
		arg.ExpiresAt,
		arg.MaxDownloads,
		arg.PasswordHash,
		arg.OwnerUserID,
	), &shipment); err != nil {
		return ShipmentFinalizeResult{}, err
	}

	if len(arg.FileIDs) > 0 {
		const attachFiles = `UPDATE files SET shipment_id = $1 WHERE id = ANY($2::uuid[])`
		cmd, execErr := tx.Exec(ctx, attachFiles, arg.ShipmentID, arg.FileIDs)
		if execErr != nil {
			return ShipmentFinalizeResult{}, execErr
		}
		if int(cmd.RowsAffected()) != len(arg.FileIDs) {
			return ShipmentFinalizeResult{}, ErrNotFound
		}
	}

	recipients := make([]Recipient, 0, len(arg.Recipients))
	if len(arg.Recipients) > 0 {
		created, createErr := q.createRecipients(ctx, tx, arg.ShipmentID, arg.Recipients)
		if createErr != nil {
			if isUniqueViolation(createErr) {
				return ShipmentFinalizeResult{}, ErrConflict
			}
			return ShipmentFinalizeResult{}, createErr
		}
		recipients = created
	}

	if len(arg.AccessTokens) > 0 {
		recipientMap := map[string]uuid.UUID{}
		for _, r := range recipients {
			recipientMap[r.EmailNormalized] = r.ID
		}
		for i := range arg.AccessTokens {
			if key := arg.AccessTokens[i].RecipientEmailNormalized; key != "" {
				id, ok := recipientMap[key]
				if !ok {
					return ShipmentFinalizeResult{}, ErrConflict
				}
				arg.AccessTokens[i].RecipientID = &id
			}
		}
		if err := q.createAccessTokens(ctx, tx, arg.ShipmentID, arg.AccessTokens); err != nil {
			if isUniqueViolation(err) {
				return ShipmentFinalizeResult{}, ErrConflict
			}
			return ShipmentFinalizeResult{}, err
		}
	}

	files, err := q.getFilesByShipmentID(ctx, tx, arg.ShipmentID)
	if err != nil {
		return ShipmentFinalizeResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ShipmentFinalizeResult{}, err
	}
	return ShipmentFinalizeResult{Shipment: shipment, Files: files, Recipients: recipients}, nil
}

type CreateFileParams struct {
	ShipmentID     uuid.UUID
	OriginalName   string
	SizeBytes      int64
	MimeType       string
	StorageBucket  string
	StorageKey     string
	ChecksumSha256 string
	UploadStatus   string
}

type File struct {
	ID             uuid.UUID `json:"id"`
	ShipmentID     uuid.UUID `json:"shipment_id"`
	OriginalName   string    `json:"original_name"`
	SizeBytes      int64     `json:"size_bytes"`
	MimeType       string    `json:"mime_type"`
	StorageBucket  string    `json:"storage_bucket"`
	StorageKey     string    `json:"storage_key"`
	ChecksumSha256 string    `json:"checksum_sha256"`
	UploadStatus   string    `json:"upload_status"`
	CreatedAt      time.Time `json:"created_at"`
}

type FileWithShipment struct {
	File
	ShipmentStatus string
	OwnerUserID    *uuid.UUID
}

type AccessToken struct {
	ID          uuid.UUID
	ShipmentID  uuid.UUID
	RecipientID *uuid.UUID
	TokenType   string
	TokenHash   string
	ExpiresAt   time.Time
	MaxUses     int32
	UsedCount   int32
	UsedAt      *time.Time
	RevokedAt   *time.Time
	Status      string
	CreatedAt   time.Time
}

type DownloadEvent struct {
	ID          int64
	ShipmentID  uuid.UUID
	FileID      uuid.UUID
	RecipientID *uuid.UUID
	Result      string
	IPHash      string
	UserAgent   *string
	CreatedAt   time.Time
}

func (q *Queries) CreateFile(ctx context.Context, arg CreateFileParams) (File, error) {
	return q.createFile(ctx, q.db, arg)
}

func (q *Queries) createFile(ctx context.Context, db dbtx, arg CreateFileParams) (File, error) {
	const query = `
INSERT INTO files (
    shipment_id, original_name, size_bytes, mime_type, storage_bucket,
    storage_key, checksum_sha256, upload_status
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING id, shipment_id, original_name, size_bytes, mime_type, storage_bucket,
          storage_key, checksum_sha256, upload_status, created_at`
	row := db.QueryRow(ctx, query,
		arg.ShipmentID,
		arg.OriginalName,
		arg.SizeBytes,
		arg.MimeType,
		arg.StorageBucket,
		arg.StorageKey,
		arg.ChecksumSha256,
		arg.UploadStatus,
	)
	var f File
	err := scanFile(row, &f)
	return f, err
}

func (q *Queries) GetFilesByIDs(ctx context.Context, ids []uuid.UUID) ([]FileWithShipment, error) {
	if len(ids) == 0 {
		return []FileWithShipment{}, nil
	}
	const query = `
SELECT f.id, f.shipment_id, f.original_name, f.size_bytes, f.mime_type, f.storage_bucket, f.storage_key, f.checksum_sha256, f.upload_status, f.created_at,
       s.status,
       us.owner_user_id
FROM files f
JOIN shipments s ON s.id = f.shipment_id
LEFT JOIN upload_sessions us ON us.file_id = f.id
WHERE f.id = ANY($1::uuid[])`
	rows, err := q.db.Query(ctx, query, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]FileWithShipment, 0, len(ids))
	for rows.Next() {
		var item FileWithShipment
		if err := rows.Scan(
			&item.ID,
			&item.ShipmentID,
			&item.OriginalName,
			&item.SizeBytes,
			&item.MimeType,
			&item.StorageBucket,
			&item.StorageKey,
			&item.ChecksumSha256,
			&item.UploadStatus,
			&item.CreatedAt,
			&item.ShipmentStatus,
			&item.OwnerUserID,
		); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (q *Queries) GetFilesByShipmentID(ctx context.Context, shipmentID uuid.UUID) ([]File, error) {
	return q.getFilesByShipmentID(ctx, q.db, shipmentID)
}

func (q *Queries) GetFileByID(ctx context.Context, id uuid.UUID) (File, error) {
	const query = `
SELECT id, shipment_id, original_name, size_bytes, mime_type, storage_bucket, storage_key, checksum_sha256, upload_status, created_at
FROM files
WHERE id = $1`
	row := q.db.QueryRow(ctx, query, id)
	var f File
	if err := scanFile(row, &f); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return File{}, ErrNotFound
		}
		return File{}, err
	}
	return f, nil
}

func (q *Queries) getFilesByShipmentID(ctx context.Context, db dbtx, shipmentID uuid.UUID) ([]File, error) {
	const query = `
SELECT id, shipment_id, original_name, size_bytes, mime_type, storage_bucket, storage_key, checksum_sha256, upload_status, created_at
FROM files WHERE shipment_id = $1 ORDER BY created_at ASC`
	rows, err := db.Query(ctx, query, shipmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []File
	for rows.Next() {
		var f File
		if err := scanFile(rows, &f); err != nil {
			return nil, err
		}
		items = append(items, f)
	}
	return items, rows.Err()
}

type CreateRecipientParams struct {
	Email           string
	EmailNormalized string
	Status          string
}

type Recipient struct {
	ID              uuid.UUID
	ShipmentID      uuid.UUID
	Email           string
	EmailNormalized string
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (q *Queries) GetRecipientsByShipmentID(ctx context.Context, shipmentID uuid.UUID) ([]Recipient, error) {
	const query = `SELECT id, shipment_id, email, email_normalized, status, created_at, updated_at FROM recipients WHERE shipment_id=$1 ORDER BY created_at ASC`
	rows, err := q.db.Query(ctx, query, shipmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var recipients []Recipient
	for rows.Next() {
		var r Recipient
		if err := rows.Scan(&r.ID, &r.ShipmentID, &r.Email, &r.EmailNormalized, &r.Status, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		recipients = append(recipients, r)
	}
	return recipients, rows.Err()
}

func (q *Queries) createRecipients(ctx context.Context, db dbtx, shipmentID uuid.UUID, recipients []CreateRecipientParams) ([]Recipient, error) {
	result := make([]Recipient, 0, len(recipients))
	const query = `
INSERT INTO recipients (shipment_id, email, email_normalized, status)
VALUES ($1, $2, $3, $4)
RETURNING id, shipment_id, email, email_normalized, status, created_at, updated_at`
	for _, r := range recipients {
		var created Recipient
		if err := db.QueryRow(ctx, query, shipmentID, r.Email, r.EmailNormalized, r.Status).Scan(
			&created.ID,
			&created.ShipmentID,
			&created.Email,
			&created.EmailNormalized,
			&created.Status,
			&created.CreatedAt,
			&created.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, created)
	}
	return result, nil
}

type CreateAccessTokenParams struct {
	RecipientID              *uuid.UUID
	RecipientEmailNormalized string
	TokenType                string
	TokenHash                string
	ExpiresAt                time.Time
	MaxUses                  int32
	Status                   string
}

func (q *Queries) createAccessTokens(ctx context.Context, db dbtx, shipmentID uuid.UUID, tokens []CreateAccessTokenParams) error {
	const query = `
INSERT INTO access_tokens (shipment_id, recipient_id, token_type, token_hash, expires_at, max_uses, status)
VALUES ($1,$2,$3,$4,$5,$6,$7)`
	for _, t := range tokens {
		if _, err := db.Exec(ctx, query, shipmentID, t.RecipientID, t.TokenType, t.TokenHash, t.ExpiresAt, t.MaxUses, t.Status); err != nil {
			return err
		}
	}
	return nil
}

func (q *Queries) GetAccessTokenByHash(ctx context.Context, tokenHash string) (AccessToken, error) {
	const query = `
SELECT id, shipment_id, recipient_id, token_type, token_hash, expires_at, max_uses, used_count, used_at, revoked_at, status, created_at
FROM access_tokens
WHERE token_hash = $1`
	row := q.db.QueryRow(ctx, query, tokenHash)
	var t AccessToken
	if err := scanAccessToken(row, &t); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AccessToken{}, ErrNotFound
		}
		return AccessToken{}, err
	}
	return t, nil
}

func (q *Queries) GetShipmentByID(ctx context.Context, id uuid.UUID) (Shipment, error) {
	return q.GetShipment(ctx, id)
}

func (q *Queries) CountDownloadEventsByShipment(ctx context.Context, shipmentID uuid.UUID) (int32, error) {
	const query = `SELECT COUNT(1) FROM download_events WHERE shipment_id = $1 AND result = 'success'`
	var count int32
	if err := q.db.QueryRow(ctx, query, shipmentID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

type CreateDownloadEventParams struct {
	ShipmentID  uuid.UUID
	FileID      uuid.UUID
	RecipientID *uuid.UUID
	Result      string
	IPHash      string
	UserAgent   *string
}

func (q *Queries) CreateDownloadEvent(ctx context.Context, arg CreateDownloadEventParams) (DownloadEvent, error) {
	const query = `
INSERT INTO download_events (shipment_id, file_id, recipient_id, result, ip_hash, user_agent)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, shipment_id, file_id, recipient_id, result, ip_hash, user_agent, created_at`
	var ev DownloadEvent
	err := q.db.QueryRow(ctx, query, arg.ShipmentID, arg.FileID, arg.RecipientID, arg.Result, arg.IPHash, arg.UserAgent).Scan(
		&ev.ID,
		&ev.ShipmentID,
		&ev.FileID,
		&ev.RecipientID,
		&ev.Result,
		&ev.IPHash,
		&ev.UserAgent,
		&ev.CreatedAt,
	)
	return ev, err
}

func (q *Queries) UpdateAccessTokenUsage(ctx context.Context, tokenID uuid.UUID) error {
	const query = `
UPDATE access_tokens
SET used_count = used_count + 1,
    used_at = COALESCE(used_at, now()),
    status = CASE WHEN used_count + 1 >= max_uses THEN 'used' ELSE status END
WHERE id = $1`
	cmd, err := q.db.Exec(ctx, query, tokenID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (q *Queries) IncrementShipmentDownloadCount(ctx context.Context, shipmentID uuid.UUID) error {
	const query = `
UPDATE shipments
SET current_downloads = current_downloads + 1,
    status = CASE WHEN status = 'sent' THEN 'accessed' ELSE status END
WHERE id = $1`
	cmd, err := q.db.Exec(ctx, query, shipmentID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type CreateUploadSessionParams struct {
	ShipmentID        *uuid.UUID
	FileID            *uuid.UUID
	StorageBucket     string
	StorageKey        string
	MultipartUploadID string
	PartSizeBytes     int32
	Status            string
	ExpiresAt         time.Time
	FileName          string
	ContentType       string
	FileSizeBytes     int64
	ChecksumSha256    string
	OwnerUserID       *uuid.UUID
}

type UploadSession struct {
	ID                uuid.UUID  `json:"id"`
	ShipmentID        *uuid.UUID `json:"shipment_id"`
	FileID            *uuid.UUID `json:"file_id"`
	StorageBucket     string     `json:"storage_bucket"`
	StorageKey        string     `json:"storage_key"`
	MultipartUploadID string     `json:"multipart_upload_id"`
	PartSizeBytes     int32      `json:"part_size_bytes"`
	Status            string     `json:"status"`
	ExpiresAt         time.Time  `json:"expires_at"`
	FileName          string     `json:"file_name"`
	ContentType       string     `json:"content_type"`
	FileSizeBytes     int64      `json:"file_size_bytes"`
	ChecksumSha256    string     `json:"checksum_sha256"`
	OwnerUserID       *uuid.UUID `json:"owner_user_id"`
	CreatedAt         time.Time  `json:"created_at"`
}

func (q *Queries) CreateUploadSession(ctx context.Context, arg CreateUploadSessionParams) (UploadSession, error) {
	const query = `
INSERT INTO upload_sessions (
    shipment_id, file_id, storage_bucket, storage_key, multipart_upload_id,
    part_size_bytes, status, expires_at, file_name, content_type, file_size_bytes,
    checksum_sha256, owner_user_id
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
RETURNING id, shipment_id, file_id, storage_bucket, storage_key, multipart_upload_id,
          part_size_bytes, status, expires_at, file_name, content_type, file_size_bytes,
          checksum_sha256, owner_user_id, created_at`
	row := q.db.QueryRow(ctx, query,
		arg.ShipmentID,
		arg.FileID,
		arg.StorageBucket,
		arg.StorageKey,
		arg.MultipartUploadID,
		arg.PartSizeBytes,
		arg.Status,
		arg.ExpiresAt,
		arg.FileName,
		arg.ContentType,
		arg.FileSizeBytes,
		arg.ChecksumSha256,
		arg.OwnerUserID,
	)
	var s UploadSession
	err := scanUploadSession(row, &s)
	return s, err
}

func (q *Queries) GetUploadSessionByID(ctx context.Context, id uuid.UUID) (UploadSession, error) {
	const query = `
SELECT id, shipment_id, file_id, storage_bucket, storage_key, multipart_upload_id,
       part_size_bytes, status, expires_at, file_name, content_type, file_size_bytes,
       checksum_sha256, owner_user_id, created_at
FROM upload_sessions
WHERE id = $1`
	row := q.db.QueryRow(ctx, query, id)
	var s UploadSession
	err := scanUploadSession(row, &s)
	if errors.Is(err, pgx.ErrNoRows) {
		return UploadSession{}, ErrNotFound
	}
	return s, err
}

type MarkUploadSessionCompletedParams struct {
	ID     uuid.UUID
	FileID uuid.UUID
}

func (q *Queries) MarkUploadSessionCompleted(ctx context.Context, arg MarkUploadSessionCompletedParams) error {
	return q.markUploadSessionCompleted(ctx, q.db, arg)
}

func (q *Queries) markUploadSessionCompleted(ctx context.Context, db dbtx, arg MarkUploadSessionCompletedParams) error {
	const query = `
UPDATE upload_sessions
SET status = 'completed', file_id = $2
WHERE id = $1 AND status <> 'completed'`
	cmd, err := db.Exec(ctx, query, arg.ID, arg.FileID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type CreateFileAndMarkUploadCompletedParams struct {
	UploadSessionID uuid.UUID
	CreateFile      CreateFileParams
}

func (q *Queries) CreateFileAndMarkUploadCompleted(ctx context.Context, arg CreateFileAndMarkUploadCompletedParams) (File, error) {
	tx, err := q.db.Begin(ctx)
	if err != nil {
		return File{}, err
	}
	defer tx.Rollback(ctx)

	f, err := q.createFile(ctx, tx, arg.CreateFile)
	if err != nil {
		return File{}, err
	}
	if err := q.markUploadSessionCompleted(ctx, tx, MarkUploadSessionCompletedParams{ID: arg.UploadSessionID, FileID: f.ID}); err != nil {
		return File{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return File{}, err
	}
	return f, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

type dbtx interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func scanUploadSession(row rowScanner, s *UploadSession) error {
	return row.Scan(
		&s.ID,
		&s.ShipmentID,
		&s.FileID,
		&s.StorageBucket,
		&s.StorageKey,
		&s.MultipartUploadID,
		&s.PartSizeBytes,
		&s.Status,
		&s.ExpiresAt,
		&s.FileName,
		&s.ContentType,
		&s.FileSizeBytes,
		&s.ChecksumSha256,
		&s.OwnerUserID,
		&s.CreatedAt,
	)
}

func scanShipment(row rowScanner, s *Shipment) error {
	return row.Scan(
		&s.ID,
		&s.OwnerType,
		&s.OwnerUserID,
		&s.Status,
		&s.ShareMode,
		&s.Title,
		&s.Message,
		&s.PasswordHash,
		&s.MaxDownloads,
		&s.CurrentDownloads,
		&s.ExpiresAt,
		&s.SentAt,
		&s.RevokedAt,
		&s.DeletedAt,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
}

func scanFile(row rowScanner, f *File) error {
	return row.Scan(
		&f.ID,
		&f.ShipmentID,
		&f.OriginalName,
		&f.SizeBytes,
		&f.MimeType,
		&f.StorageBucket,
		&f.StorageKey,
		&f.ChecksumSha256,
		&f.UploadStatus,
		&f.CreatedAt,
	)
}

func scanAccessToken(row rowScanner, t *AccessToken) error {
	return row.Scan(
		&t.ID,
		&t.ShipmentID,
		&t.RecipientID,
		&t.TokenType,
		&t.TokenHash,
		&t.ExpiresAt,
		&t.MaxUses,
		&t.UsedCount,
		&t.UsedAt,
		&t.RevokedAt,
		&t.Status,
		&t.CreatedAt,
	)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

type User struct {
	ID              uuid.UUID
	Email           string
	EmailNormalized string
	PasswordHash    string
	DisplayName     *string
	Status          string
	EmailVerifiedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type CreateUserParams struct {
	Email           string
	EmailNormalized string
	PasswordHash    string
	DisplayName     *string
	Status          string
}

func (q *Queries) CreateUser(ctx context.Context, arg CreateUserParams) (User, error) {
	const query = `
INSERT INTO users (email, email_normalized, password_hash, display_name, status)
VALUES ($1,$2,$3,$4,$5)
RETURNING id, email, email_normalized, password_hash, display_name, status, email_verified_at, created_at, updated_at`
	var out User
	err := q.db.QueryRow(ctx, query, arg.Email, arg.EmailNormalized, arg.PasswordHash, arg.DisplayName, arg.Status).Scan(
		&out.ID,
		&out.Email,
		&out.EmailNormalized,
		&out.PasswordHash,
		&out.DisplayName,
		&out.Status,
		&out.EmailVerifiedAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrConflict
		}
		return User{}, err
	}
	return out, nil
}

func (q *Queries) GetUserByEmail(ctx context.Context, emailNormalized string) (User, error) {
	const query = `
SELECT id, email, email_normalized, password_hash, display_name, status, email_verified_at, created_at, updated_at
FROM users WHERE email_normalized = $1`
	var out User
	err := q.db.QueryRow(ctx, query, emailNormalized).Scan(&out.ID, &out.Email, &out.EmailNormalized, &out.PasswordHash, &out.DisplayName, &out.Status, &out.EmailVerifiedAt, &out.CreatedAt, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return out, err
}

func (q *Queries) GetUserByID(ctx context.Context, id uuid.UUID) (User, error) {
	const query = `
SELECT id, email, email_normalized, password_hash, display_name, status, email_verified_at, created_at, updated_at
FROM users WHERE id = $1`
	var out User
	err := q.db.QueryRow(ctx, query, id).Scan(&out.ID, &out.Email, &out.EmailNormalized, &out.PasswordHash, &out.DisplayName, &out.Status, &out.EmailVerifiedAt, &out.CreatedAt, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return out, err
}

type Session struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	TokenHash  string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
	UserAgent  *string
	IPHash     *string
}

type SessionWithUser struct {
	Session Session
	User    User
}

type CreateSessionParams struct {
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	UserAgent *string
	IPHash    *string
}

func (q *Queries) CreateSession(ctx context.Context, arg CreateSessionParams) (Session, error) {
	const query = `
INSERT INTO sessions (user_id, token_hash, expires_at, user_agent, ip_hash)
VALUES ($1,$2,$3,$4,$5)
RETURNING id, user_id, token_hash, expires_at, created_at, last_used_at, revoked_at, user_agent, ip_hash`
	var out Session
	err := q.db.QueryRow(ctx, query, arg.UserID, arg.TokenHash, arg.ExpiresAt, arg.UserAgent, arg.IPHash).Scan(
		&out.ID, &out.UserID, &out.TokenHash, &out.ExpiresAt, &out.CreatedAt, &out.LastUsedAt, &out.RevokedAt, &out.UserAgent, &out.IPHash,
	)
	return out, err
}

func (q *Queries) GetSessionByHash(ctx context.Context, tokenHash string) (Session, error) {
	const query = `
SELECT id, user_id, token_hash, expires_at, created_at, last_used_at, revoked_at, user_agent, ip_hash
FROM sessions WHERE token_hash = $1`
	var out Session
	err := q.db.QueryRow(ctx, query, tokenHash).Scan(&out.ID, &out.UserID, &out.TokenHash, &out.ExpiresAt, &out.CreatedAt, &out.LastUsedAt, &out.RevokedAt, &out.UserAgent, &out.IPHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	return out, err
}

func (q *Queries) GetSessionByHashWithUser(ctx context.Context, tokenHash string) (SessionWithUser, error) {
	const query = `
SELECT s.id, s.user_id, s.token_hash, s.expires_at, s.created_at, s.last_used_at, s.revoked_at, s.user_agent, s.ip_hash,
       u.id, u.email, u.email_normalized, u.password_hash, u.display_name, u.status, u.email_verified_at, u.created_at, u.updated_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1`
	var out SessionWithUser
	err := q.db.QueryRow(ctx, query, tokenHash).Scan(
		&out.Session.ID, &out.Session.UserID, &out.Session.TokenHash, &out.Session.ExpiresAt, &out.Session.CreatedAt, &out.Session.LastUsedAt, &out.Session.RevokedAt, &out.Session.UserAgent, &out.Session.IPHash,
		&out.User.ID, &out.User.Email, &out.User.EmailNormalized, &out.User.PasswordHash, &out.User.DisplayName, &out.User.Status, &out.User.EmailVerifiedAt, &out.User.CreatedAt, &out.User.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionWithUser{}, ErrNotFound
	}
	return out, err
}

func (q *Queries) RevokeSession(ctx context.Context, tokenHash string) error {
	const query = `UPDATE sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`
	cmd, err := q.db.Exec(ctx, query, tokenHash)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (q *Queries) UpdateSessionLastUsed(ctx context.Context, tokenHash string, lastUsedAt time.Time) error {
	const query = `UPDATE sessions SET last_used_at = $2 WHERE token_hash = $1`
	cmd, err := q.db.Exec(ctx, query, tokenHash, lastUsedAt)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
