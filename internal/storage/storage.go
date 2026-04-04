package storage

import (
	"context"
	"time"
)

// PresignedPart は multipart upload の各パートURLを表す。
type PresignedPart struct {
	PartNumber int32  `json:"part_number"`
	URL        string `json:"presigned_url"`
}

// CompletedPart は multipart upload 完了時に必要なパート情報。
type CompletedPart struct {
	PartNumber int32
	ETag       string
}

// ObjectStore は S3 依存を隠蔽する抽象。
// handler/service から AWS SDK を直接触らないための境界として利用する。
type ObjectStore interface {
	CreateMultipartUpload(ctx context.Context, bucket, key, contentType string) (uploadID string, err error)
	BatchPresignUploadParts(ctx context.Context, bucket, key, uploadID string, partCount int, expiresIn time.Duration) ([]PresignedPart, error)
	CompleteMultipartUpload(ctx context.Context, bucket, key, uploadID string, parts []CompletedPart) error
	GenerateDownloadURL(ctx context.Context, bucket, key string, expiresIn time.Duration) (string, error)
}
