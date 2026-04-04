package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/vaultsend/internal/storage"
	"github.com/example/vaultsend/internal/store"
	"github.com/google/uuid"
)

type fakeStore struct {
	session        store.UploadSession
	shipment       store.Shipment
	file           store.File
	getErr         error
	createSession  store.CreateUploadSessionParams
	completeCalled bool
}

func (f *fakeStore) CreateShipment(ctx context.Context, arg store.CreateShipmentParams) (store.Shipment, error) {
	if f.shipment.ID == uuid.Nil {
		f.shipment.ID = uuid.New()
	}
	return f.shipment, nil
}
func (f *fakeStore) CreateUploadSession(ctx context.Context, arg store.CreateUploadSessionParams) (store.UploadSession, error) {
	f.createSession = arg
	if f.session.ID == uuid.Nil {
		f.session = store.UploadSession{ID: uuid.New(), ShipmentID: arg.ShipmentID, StorageBucket: arg.StorageBucket, StorageKey: arg.StorageKey, MultipartUploadID: arg.MultipartUploadID, Status: arg.Status, FileName: arg.FileName, ContentType: arg.ContentType, FileSizeBytes: arg.FileSizeBytes, ChecksumSha256: arg.ChecksumSha256}
	}
	return f.session, nil
}
func (f *fakeStore) GetUploadSessionByID(ctx context.Context, id uuid.UUID) (store.UploadSession, error) {
	if f.getErr != nil {
		return store.UploadSession{}, f.getErr
	}
	return f.session, nil
}
func (f *fakeStore) CreateFile(ctx context.Context, arg store.CreateFileParams) (store.File, error) {
	return store.File{}, nil
}
func (f *fakeStore) MarkUploadSessionCompleted(ctx context.Context, arg store.MarkUploadSessionCompletedParams) error {
	return nil
}
func (f *fakeStore) CreateFileAndMarkUploadCompleted(ctx context.Context, arg store.CreateFileAndMarkUploadCompletedParams) (store.File, error) {
	f.completeCalled = true
	if f.file.ID == uuid.Nil {
		f.file = store.File{ID: uuid.New(), ShipmentID: arg.CreateFile.ShipmentID}
	}
	return f.file, nil
}

type fakeObjectStore struct {
	completeErr error
}

func (f *fakeObjectStore) CreateMultipartUpload(ctx context.Context, bucket, key, contentType string) (string, error) {
	return "upload-id", nil
}
func (f *fakeObjectStore) BatchPresignUploadParts(ctx context.Context, bucket, key, uploadID string, partCount int, expiresIn time.Duration) ([]storage.PresignedPart, error) {
	return []storage.PresignedPart{{PartNumber: 1, URL: "https://example.com/1"}}, nil
}
func (f *fakeObjectStore) CompleteMultipartUpload(ctx context.Context, bucket, key, uploadID string, parts []storage.CompletedPart) error {
	return f.completeErr
}

func TestCreateUploadSession_ValidationError(t *testing.T) {
	svc := &UploadService{Store: &fakeStore{}, ObjectStore: &fakeObjectStore{}, S3Bucket: "bucket"}
	_, err := svc.CreateUploadSession(context.Background(), CreateUploadInput{FileName: "", ContentType: "text/plain", FileSize: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCompleteUploadSession_Success(t *testing.T) {
	shipmentID := uuid.New()
	fs := &fakeStore{session: store.UploadSession{ID: uuid.New(), ShipmentID: &shipmentID, StorageBucket: "b", StorageKey: "k", MultipartUploadID: "u", Status: "uploading", FileName: "a.txt", ContentType: "text/plain", FileSizeBytes: 10, ChecksumSha256: "abc"}}
	svc := &UploadService{Store: fs, ObjectStore: &fakeObjectStore{}, S3Bucket: "bucket"}
	out, err := svc.CompleteUploadSession(context.Background(), CompleteUploadInput{UploadSessionID: fs.session.ID, Parts: []storage.CompletedPart{{PartNumber: 1, ETag: "etag"}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "completed" || !fs.completeCalled {
		t.Fatal("expected completed flow")
	}
}

func TestCompleteUploadSession_AlreadyCompleted(t *testing.T) {
	shipmentID := uuid.New()
	fs := &fakeStore{session: store.UploadSession{ID: uuid.New(), ShipmentID: &shipmentID, Status: "completed"}}
	svc := &UploadService{Store: fs, ObjectStore: &fakeObjectStore{}, S3Bucket: "bucket"}
	_, err := svc.CompleteUploadSession(context.Background(), CompleteUploadInput{UploadSessionID: fs.session.ID, Parts: []storage.CompletedPart{{PartNumber: 1, ETag: "etag"}}})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 409 {
		t.Fatalf("expected 409 api error got=%v", err)
	}
}

func TestCompleteUploadSession_NotFound(t *testing.T) {
	fs := &fakeStore{getErr: store.ErrNotFound}
	svc := &UploadService{Store: fs, ObjectStore: &fakeObjectStore{}, S3Bucket: "bucket"}
	_, err := svc.CompleteUploadSession(context.Background(), CompleteUploadInput{UploadSessionID: uuid.New(), Parts: []storage.CompletedPart{{PartNumber: 1, ETag: "etag"}}})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 404 {
		t.Fatalf("expected 404 api error got=%v", err)
	}
}
