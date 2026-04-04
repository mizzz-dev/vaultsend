package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

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
	const query = `
INSERT INTO files (
    shipment_id, original_name, size_bytes, mime_type, storage_bucket,
    storage_key, checksum_sha256, upload_status
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING id, shipment_id, original_name, size_bytes, mime_type, storage_bucket,
          storage_key, checksum_sha256, upload_status, created_at`
	row := q.db.QueryRow(ctx, query,
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
	CreatedAt         time.Time  `json:"created_at"`
}

func (q *Queries) CreateUploadSession(ctx context.Context, arg CreateUploadSessionParams) (UploadSession, error) {
	const query = `
INSERT INTO upload_sessions (
    shipment_id, file_id, storage_bucket, storage_key, multipart_upload_id,
    part_size_bytes, status, expires_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING id, shipment_id, file_id, storage_bucket, storage_key, multipart_upload_id,
          part_size_bytes, status, expires_at, created_at`
	row := q.db.QueryRow(ctx, query,
		arg.ShipmentID,
		arg.FileID,
		arg.StorageBucket,
		arg.StorageKey,
		arg.MultipartUploadID,
		arg.PartSizeBytes,
		arg.Status,
		arg.ExpiresAt,
	)
	var s UploadSession
	err := row.Scan(
		&s.ID,
		&s.ShipmentID,
		&s.FileID,
		&s.StorageBucket,
		&s.StorageKey,
		&s.MultipartUploadID,
		&s.PartSizeBytes,
		&s.Status,
		&s.ExpiresAt,
		&s.CreatedAt,
	)
	return s, err
}

func (q *Queries) GetUploadSession(ctx context.Context, id uuid.UUID) (UploadSession, error) {
	const query = `
SELECT id, shipment_id, file_id, storage_bucket, storage_key, multipart_upload_id,
       part_size_bytes, status, expires_at, created_at
FROM upload_sessions
WHERE id = $1`
	row := q.db.QueryRow(ctx, query, id)
	var s UploadSession
	err := row.Scan(
		&s.ID,
		&s.ShipmentID,
		&s.FileID,
		&s.StorageBucket,
		&s.StorageKey,
		&s.MultipartUploadID,
		&s.PartSizeBytes,
		&s.Status,
		&s.ExpiresAt,
		&s.CreatedAt,
	)
	return s, err
}
