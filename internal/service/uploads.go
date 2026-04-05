package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/example/vaultsend/internal/storage"
	"github.com/example/vaultsend/internal/store"
	"github.com/google/uuid"
)

const (
	defaultPartSizeBytes int32 = 8 * 1024 * 1024
	defaultMaxFileSize   int64 = 10 * 1024 * 1024 * 1024 // 設計書の仮置き値: 10GB
	defaultMaxParts            = 1000                    // 仮置き: レスポンス肥大化を防ぐ上限
)

type APIError struct {
	Status          int
	Error           string
	Code            string
	Message         string
	UpgradeRequired bool
	UpgradeURL      string
	RecommendedPlan string
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

type UploadStore interface {
	CreateShipment(ctx context.Context, arg store.CreateShipmentParams) (store.Shipment, error)
	CreateUploadSession(ctx context.Context, arg store.CreateUploadSessionParams) (store.UploadSession, error)
	GetUploadSessionByID(ctx context.Context, id uuid.UUID) (store.UploadSession, error)
	CreateFile(ctx context.Context, arg store.CreateFileParams) (store.File, error)
	MarkUploadSessionCompleted(ctx context.Context, arg store.MarkUploadSessionCompletedParams) error
	CreateFileAndMarkUploadCompleted(ctx context.Context, arg store.CreateFileAndMarkUploadCompletedParams) (store.File, error)
}

type UploadService struct {
	Store               UploadStore
	ObjectStore         storage.ObjectStore
	Billing             *BillingService
	S3Bucket            string
	PartSizeBytes       int32
	UploadURLTTL        time.Duration
	UploadSessionTTL    time.Duration
	MaxFileSizeBytes    int64
	MaxPresignedPartNum int
}

type CreateUploadInput struct {
	ShipmentID     *uuid.UUID
	OwnerUserID    *uuid.UUID
	OrganizationID *uuid.UUID
	FileName       string
	ContentType    string
	FileSize       int64
	ChecksumSHA256 string
}

type CreateUploadOutput struct {
	UploadSessionID uuid.UUID               `json:"upload_session_id"`
	ShipmentID      *uuid.UUID              `json:"shipment_id,omitempty"`
	ObjectKey       string                  `json:"object_key"`
	S3UploadID      string                  `json:"s3_upload_id"`
	PartSize        int32                   `json:"part_size"`
	Parts           []storage.PresignedPart `json:"parts"`
	ExpiresAt       time.Time               `json:"expires_at"`
}

type CompleteUploadInput struct {
	UploadSessionID uuid.UUID
	Parts           []storage.CompletedPart
}

type CompleteUploadOutput struct {
	UploadSessionID uuid.UUID `json:"upload_session_id"`
	FileID          uuid.UUID `json:"file_id"`
	ShipmentID      uuid.UUID `json:"shipment_id"`
	Status          string    `json:"status"`
}

func (s *UploadService) CreateUploadSession(ctx context.Context, in CreateUploadInput) (CreateUploadOutput, error) {
	if err := s.validateCreateInput(in); err != nil {
		return CreateUploadOutput{}, err
	}
	if s.Billing != nil {
		if err := s.Billing.EnforceUploadLimit(ctx, in.OwnerUserID, in.OrganizationID, in.FileSize); err != nil {
			return CreateUploadOutput{}, err
		}
	}

	partSize := s.partSize()
	partCount := int(math.Ceil(float64(in.FileSize) / float64(partSize)))
	if partCount > s.maxParts() {
		return CreateUploadOutput{}, &APIError{Status: 400, Code: "file_too_large_for_single_session", Message: "パート数が上限を超えています"}
	}

	shipmentID := in.ShipmentID
	if shipmentID == nil {
		// 仮置き: shipment 作成API前の段階でも uploads 単体で進められるよう draft shipment を作成する。
		shipment, err := s.Store.CreateShipment(ctx, store.CreateShipmentParams{
			OwnerType:      "anonymous",
			OwnerUserID:    in.OwnerUserID,
			OrganizationID: in.OrganizationID,
			Status:         "uploading",
			ShareMode:      "recipient_restricted",
			Title:          "(仮置き) upload-in-progress",
			Message:        nil,
			MaxDownloads:   10,
			ExpiresAt:      time.Now().UTC().Add(7 * 24 * time.Hour),
		})
		if err != nil {
			return CreateUploadOutput{}, fmt.Errorf("create shipment: %w", err)
		}
		shipmentID = &shipment.ID
	}

	objectKey := buildObjectKey(*shipmentID, in.FileName)
	uploadID, err := s.ObjectStore.CreateMultipartUpload(ctx, s.S3Bucket, objectKey, in.ContentType)
	if err != nil {
		return CreateUploadOutput{}, fmt.Errorf("create multipart upload: %w", err)
	}

	expiresAt := time.Now().UTC().Add(s.sessionTTL())
	session, err := s.Store.CreateUploadSession(ctx, store.CreateUploadSessionParams{
		ShipmentID:        shipmentID,
		StorageBucket:     s.S3Bucket,
		StorageKey:        objectKey,
		MultipartUploadID: uploadID,
		PartSizeBytes:     partSize,
		Status:            "uploading",
		ExpiresAt:         expiresAt,
		FileName:          in.FileName,
		ContentType:       in.ContentType,
		FileSizeBytes:     in.FileSize,
		ChecksumSha256:    in.ChecksumSHA256,
		OwnerUserID:       in.OwnerUserID,
	})
	if err != nil {
		return CreateUploadOutput{}, fmt.Errorf("create upload session: %w", err)
	}

	parts, err := s.ObjectStore.BatchPresignUploadParts(ctx, s.S3Bucket, objectKey, uploadID, partCount, s.uploadURLTTL())
	if err != nil {
		return CreateUploadOutput{}, fmt.Errorf("batch presign upload parts: %w", err)
	}

	return CreateUploadOutput{
		UploadSessionID: session.ID,
		ShipmentID:      shipmentID,
		ObjectKey:       objectKey,
		S3UploadID:      uploadID,
		PartSize:        partSize,
		Parts:           parts,
		ExpiresAt:       expiresAt,
	}, nil
}

func (s *UploadService) CompleteUploadSession(ctx context.Context, in CompleteUploadInput) (CompleteUploadOutput, error) {
	if len(in.Parts) == 0 {
		return CompleteUploadOutput{}, &APIError{Status: 400, Code: "invalid_parts", Message: "parts は1件以上必要です"}
	}

	session, err := s.Store.GetUploadSessionByID(ctx, in.UploadSessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return CompleteUploadOutput{}, &APIError{Status: 404, Code: "upload_session_not_found", Message: "upload session が見つかりません"}
		}
		return CompleteUploadOutput{}, fmt.Errorf("get upload session: %w", err)
	}
	if session.Status == "completed" {
		return CompleteUploadOutput{}, &APIError{Status: 409, Code: "upload_session_already_completed", Message: "既に完了済みです"}
	}
	if session.Status != "uploading" && session.Status != "initiated" {
		return CompleteUploadOutput{}, &APIError{Status: 409, Code: "upload_session_status_conflict", Message: "現在の status では完了できません"}
	}
	if session.ShipmentID == nil {
		return CompleteUploadOutput{}, &APIError{Status: 409, Code: "upload_session_missing_shipment", Message: "shipment_id が未設定です"}
	}

	if err := s.ObjectStore.CompleteMultipartUpload(ctx, session.StorageBucket, session.StorageKey, session.MultipartUploadID, in.Parts); err != nil {
		return CompleteUploadOutput{}, fmt.Errorf("complete multipart upload: %w", err)
	}

	file, err := s.Store.CreateFileAndMarkUploadCompleted(ctx, store.CreateFileAndMarkUploadCompletedParams{
		UploadSessionID: session.ID,
		CreateFile: store.CreateFileParams{
			ShipmentID:     *session.ShipmentID,
			OriginalName:   session.FileName,
			SizeBytes:      session.FileSizeBytes,
			MimeType:       session.ContentType,
			StorageBucket:  session.StorageBucket,
			StorageKey:     session.StorageKey,
			ChecksumSha256: session.ChecksumSha256,
			UploadStatus:   "completed",
		},
	})
	if err != nil {
		return CompleteUploadOutput{}, fmt.Errorf("create file and mark upload completed: %w", err)
	}

	return CompleteUploadOutput{UploadSessionID: session.ID, FileID: file.ID, ShipmentID: *session.ShipmentID, Status: "completed"}, nil
}

func (s *UploadService) validateCreateInput(in CreateUploadInput) error {
	if strings.TrimSpace(in.FileName) == "" {
		return &APIError{Status: 400, Code: "invalid_file_name", Message: "file_name は必須です"}
	}
	if in.FileSize <= 0 {
		return &APIError{Status: 400, Code: "invalid_file_size", Message: "file_size は正の値が必要です"}
	}
	if in.FileSize > s.maxFileSize() {
		return &APIError{Status: 400, Code: "file_size_exceeded", Message: "file_size が上限を超えています"}
	}
	if !isContentTypeLike(in.ContentType) {
		return &APIError{Status: 400, Code: "invalid_content_type", Message: "content_type が不正です"}
	}
	return nil
}

func buildObjectKey(shipmentID uuid.UUID, fileName string) string {
	safeName := filepath.Base(fileName)
	return fmt.Sprintf("uploads/%s/%s/%s", shipmentID.String(), uuid.NewString(), safeName)
}

func isContentTypeLike(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > 120 {
		return false
	}
	split := strings.Split(v, "/")
	return len(split) == 2 && split[0] != "" && split[1] != ""
}

func (s *UploadService) partSize() int32 {
	if s.PartSizeBytes > 0 {
		return s.PartSizeBytes
	}
	return defaultPartSizeBytes
}

func (s *UploadService) uploadURLTTL() time.Duration {
	if s.UploadURLTTL > 0 {
		return s.UploadURLTTL
	}
	return 15 * time.Minute
}

func (s *UploadService) sessionTTL() time.Duration {
	if s.UploadSessionTTL > 0 {
		return s.UploadSessionTTL
	}
	return 15 * time.Minute
}

func (s *UploadService) maxFileSize() int64 {
	if s.MaxFileSizeBytes > 0 {
		return s.MaxFileSizeBytes
	}
	return defaultMaxFileSize
}

func (s *UploadService) maxParts() int {
	if s.MaxPresignedPartNum > 0 {
		return s.MaxPresignedPartNum
	}
	return defaultMaxParts
}
