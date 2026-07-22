package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/vaultsend/internal/http/middleware"
	"github.com/example/vaultsend/internal/service"
	"github.com/example/vaultsend/internal/storage"
	"github.com/example/vaultsend/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type handlerStore struct {
	lastOwnerUserID *uuid.UUID
	session         *store.UploadSession
}

func (h *handlerStore) CreateShipment(ctx context.Context, arg store.CreateShipmentParams) (store.Shipment, error) {
	return store.Shipment{ID: uuid.New()}, nil
}
func (h *handlerStore) GetShipment(ctx context.Context, id uuid.UUID) (store.Shipment, error) {
	return store.Shipment{ID: id, Status: "uploading"}, nil
}
func (h *handlerStore) CreateUploadSession(ctx context.Context, arg store.CreateUploadSessionParams) (store.UploadSession, error) {
	h.lastOwnerUserID = arg.OwnerUserID
	return store.UploadSession{ID: uuid.New(), ShipmentID: arg.ShipmentID}, nil
}
func (h *handlerStore) GetUploadSessionByID(ctx context.Context, id uuid.UUID) (store.UploadSession, error) {
	if h.session != nil {
		return *h.session, nil
	}
	shipmentID := uuid.New()
	return store.UploadSession{ID: id, ShipmentID: &shipmentID, Status: "completed"}, nil
}
func (h *handlerStore) CreateFile(ctx context.Context, arg store.CreateFileParams) (store.File, error) {
	return store.File{}, nil
}
func (h *handlerStore) MarkUploadSessionCompleted(ctx context.Context, arg store.MarkUploadSessionCompletedParams) error {
	return nil
}
func (h *handlerStore) CreateFileAndMarkUploadCompleted(ctx context.Context, arg store.CreateFileAndMarkUploadCompletedParams) (store.File, error) {
	return store.File{ID: uuid.New(), ShipmentID: arg.CreateFile.ShipmentID}, nil
}

type handlerObj struct{}

func (o *handlerObj) CreateMultipartUpload(ctx context.Context, bucket, key, contentType string) (string, error) {
	return "upload", nil
}
func (o *handlerObj) BatchPresignUploadParts(ctx context.Context, bucket, key, uploadID string, partCount int, expiresIn time.Duration) ([]storage.PresignedPart, error) {
	return []storage.PresignedPart{{PartNumber: 1, URL: "https://example"}}, nil
}
func (o *handlerObj) CompleteMultipartUpload(ctx context.Context, bucket, key, uploadID string, parts []storage.CompletedPart) error {
	return nil
}
func (o *handlerObj) GenerateDownloadURL(ctx context.Context, bucket, key string, expiresIn time.Duration) (string, error) {
	return "https://example.com/download", nil
}
func (o *handlerObj) DeleteObject(ctx context.Context, bucket, key string) error {
	return nil
}

func TestCreateUpload_BadRequest(t *testing.T) {
	svc := &service.UploadService{Store: &handlerStore{}, ObjectStore: &handlerObj{}, S3Bucket: "bucket"}
	h := UploadHandler{Service: svc}
	body := []byte(`{"file_name":"","file_size":100,"content_type":"text/plain"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/uploads", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.CreateUpload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCompleteUpload_AlreadyCompletedWithoutFile(t *testing.T) {
	svc := &service.UploadService{Store: &handlerStore{}, ObjectStore: &handlerObj{}, S3Bucket: "bucket"}
	h := UploadHandler{Service: svc}
	r := chi.NewRouter()
	r.Post("/v1/uploads/{id}/complete", h.CompleteUpload)

	payload, _ := json.Marshal(map[string]any{"parts": []map[string]any{{"part_number": 1, "etag": "etag"}}})
	req := httptest.NewRequest(http.MethodPost, "/v1/uploads/"+uuid.NewString()+"/complete", bytes.NewReader(payload))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateUpload_WithAuthOwnerUserID(t *testing.T) {
	store := &handlerStore{}
	svc := &service.UploadService{Store: store, ObjectStore: &handlerObj{}, S3Bucket: "bucket"}
	h := UploadHandler{Service: svc}
	body := []byte(`{"file_name":"a.txt","file_size":100,"content_type":"text/plain"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/uploads", bytes.NewReader(body))
	userID := uuid.New()
	req = req.WithContext(middleware.WithAuthUser(req.Context(), service.AuthUser{ID: userID, Email: "a@example.com"}))
	w := httptest.NewRecorder()
	h.CreateUpload(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if store.lastOwnerUserID == nil || *store.lastOwnerUserID != userID {
		t.Fatal("owner_user_id should be propagated")
	}
}

func TestCompleteUpload_WithAuthOwnerUserID(t *testing.T) {
	userID := uuid.New()
	shipmentID := uuid.New()
	uploadSessionID := uuid.New()
	store := &handlerStore{session: &store.UploadSession{
		ID:                uploadSessionID,
		ShipmentID:        &shipmentID,
		OwnerUserID:       &userID,
		StorageBucket:     "b",
		StorageKey:        "k",
		MultipartUploadID: "u",
		Status:            "uploading",
		FileName:          "a.txt",
		ContentType:       "text/plain",
		FileSizeBytes:     10,
		ChecksumSha256:    "abc",
	}}
	svc := &service.UploadService{Store: store, ObjectStore: &handlerObj{}, S3Bucket: "bucket"}
	h := UploadHandler{Service: svc}
	r := chi.NewRouter()
	r.Post("/v1/uploads/{id}/complete", h.CompleteUpload)

	payload, _ := json.Marshal(map[string]any{"parts": []map[string]any{{"part_number": 1, "etag": "etag"}}})
	req := httptest.NewRequest(http.MethodPost, "/v1/uploads/"+uploadSessionID.String()+"/complete", bytes.NewReader(payload))
	req = req.WithContext(middleware.WithAuthUser(req.Context(), service.AuthUser{ID: userID, Email: "a@example.com"}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
