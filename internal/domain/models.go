package domain

import (
	"time"

	"github.com/google/uuid"
)

// ShipmentStatus は shipment の状態遷移を Go 側でも型として扱うための定義。
type ShipmentStatus string

const (
	ShipmentStatusDraft     ShipmentStatus = "draft"
	ShipmentStatusUploading ShipmentStatus = "uploading"
	ShipmentStatusReady     ShipmentStatus = "ready"
	ShipmentStatusSent      ShipmentStatus = "sent"
	ShipmentStatusAccessed  ShipmentStatus = "accessed"
	ShipmentStatusExpired   ShipmentStatus = "expired"
	ShipmentStatusDeleted   ShipmentStatus = "deleted"
	ShipmentStatusRevoked   ShipmentStatus = "revoked"
)

// UploadSessionStatus は multipart upload セッションの状態を表す。
type UploadSessionStatus string

const (
	UploadSessionStatusInitiated UploadSessionStatus = "initiated"
	UploadSessionStatusUploading UploadSessionStatus = "uploading"
	UploadSessionStatusCompleted UploadSessionStatus = "completed"
	UploadSessionStatusAborted   UploadSessionStatus = "aborted"
)

// Shipment は一覧/詳細で利用される最小モデル。
// TODO: 次PRで recipients/files 集約モデルを別structで追加する。
type Shipment struct {
	ID               uuid.UUID
	OwnerType        string
	OwnerUserID      *uuid.UUID
	Status           ShipmentStatus
	ShareMode        string
	Title            string
	Message          *string
	MaxDownloads     int32
	CurrentDownloads int32
	ExpiresAt        time.Time
	SentAt           *time.Time
	RevokedAt        *time.Time
	DeletedAt        *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
