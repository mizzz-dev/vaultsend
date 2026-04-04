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

// TODO: 次PRで sqlc generated code に置き換え、手書きSQLを段階的に削除する。

// --- shipments ---

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
	const query = `
INSERT INTO shipments (
    owner_type, owner_user_id, status, share_mode, title, message, max_downloads, expires_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING id, owner_type, owner_user_id, status, share_mode, title, message, max_downloads,
          current_downloads, expires_at, sent_at, revoked_at, deleted_at, created_at, updated_at`
	row := q.db.QueryRow(ctx, query,
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
	err := row.Scan(
		&s.ID,
		&s.OwnerType,
		&s.OwnerUserID,
		&s.Status,
		&s.ShareMode,
		&s.Title,
		&s.Message,
		&s.MaxDownloads,
		&s.CurrentDownloads,
		&s.ExpiresAt,
		&s.SentAt,
		&s.RevokedAt,
		&s.DeletedAt,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	return s, err
}

func (q *Queries) GetShipment(ctx context.Context, id uuid.UUID) (Shipment, error) {
	const query = `
SELECT id, owner_type, owner_user_id, status, share_mode, title, message, max_downloads,
       current_downloads, expires_at, sent_at, revoked_at, deleted_at, created_at, updated_at
FROM shipments
WHERE id = $1`
	row := q.db.QueryRow(ctx, query, id)
	var s Shipment
	err := row.Scan(
		&s.ID,
		&s.OwnerType,
		&s.OwnerUserID,
		&s.Status,
		&s.ShareMode,
		&s.Title,
		&s.Message,
		&s.MaxDownloads,
		&s.CurrentDownloads,
		&s.ExpiresAt,
		&s.SentAt,
		&s.RevokedAt,
		&s.DeletedAt,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Shipment{}, ErrNotFound
	}
	return s, err
}

// --- files ---

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
	err := row.Scan(
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
	return f, err
}

// --- upload_sessions ---

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
