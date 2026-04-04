package storage

import "context"

// PresignedPart は multipart upload の各パートURLを表す。
type PresignedPart struct {
	PartNumber int    `json:"part_number"`
	URL        string `json:"url"`
}

// ObjectStore は S3 への依存を抽象化する。
// TODO: 次PRで initiate/complete/abort の本実装を追加する。
type ObjectStore interface {
	CreateMultipartUpload(ctx context.Context, bucket, key string, partCount int) (uploadID string, parts []PresignedPart, err error)
	CompleteMultipartUpload(ctx context.Context, bucket, key, uploadID string, etags map[int]string) error
}
