package storage

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3ObjectStore は AWS S3 を利用した ObjectStore 実装。
type S3ObjectStore struct {
	client   *s3.Client
	presign  *s3.PresignClient
	partSize int64
}

func NewS3ObjectStore(client *s3.Client) *S3ObjectStore {
	return &S3ObjectStore{
		client:   client,
		presign:  s3.NewPresignClient(client),
		partSize: 8 * 1024 * 1024,
	}
}

func (s *S3ObjectStore) CreateMultipartUpload(ctx context.Context, bucket, key, contentType string) (string, error) {
	out, err := s.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("create multipart upload: %w", err)
	}
	if out.UploadId == nil || *out.UploadId == "" {
		return "", fmt.Errorf("create multipart upload: empty upload id")
	}
	return *out.UploadId, nil
}

func (s *S3ObjectStore) BatchPresignUploadParts(ctx context.Context, bucket, key, uploadID string, partCount int, expiresIn time.Duration) ([]PresignedPart, error) {
	parts := make([]PresignedPart, 0, partCount)
	for i := 1; i <= partCount; i++ {
		partNo := int32(i)
		out, err := s.presign.PresignUploadPart(ctx, &s3.UploadPartInput{
			Bucket:     aws.String(bucket),
			Key:        aws.String(key),
			UploadId:   aws.String(uploadID),
			PartNumber: aws.Int32(partNo),
		}, s3.WithPresignExpires(expiresIn))
		if err != nil {
			return nil, fmt.Errorf("presign upload part %d: %w", i, err)
		}
		parts = append(parts, PresignedPart{PartNumber: partNo, URL: out.URL})
	}
	return parts, nil
}

func (s *S3ObjectStore) CompleteMultipartUpload(ctx context.Context, bucket, key, uploadID string, parts []CompletedPart) error {
	sorted := append([]CompletedPart(nil), parts...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].PartNumber < sorted[j].PartNumber })

	s3Parts := make([]types.CompletedPart, 0, len(sorted))
	for _, p := range sorted {
		s3Parts = append(s3Parts, types.CompletedPart{
			ETag:       aws.String(p.ETag),
			PartNumber: aws.Int32(p.PartNumber),
		})
	}

	_, err := s.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: s3Parts,
		},
	})
	if err != nil {
		return fmt.Errorf("complete multipart upload: %w", err)
	}
	return nil
}
