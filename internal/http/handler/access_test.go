package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/example/vaultsend/internal/service"
	"github.com/example/vaultsend/internal/storage"
	"github.com/example/vaultsend/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type fakeAccessHandlerStore struct {
	passwordHash *string
}

func (f *fakeAccessHandlerStore) GetAccessTokenByHash(ctx context.Context, tokenHash string) (store.AccessToken, error) {
	shipID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	return store.AccessToken{ID: uuid.New(), ShipmentID: shipID, TokenType: "download_access", ExpiresAt: time.Now().UTC().Add(1 * time.Hour), MaxUses: 10, Status: "active"}, nil
}
func (f *fakeAccessHandlerStore) GetShipmentByID(ctx context.Context, id uuid.UUID) (store.Shipment, error) {
	return store.Shipment{ID: id, Status: "sent", ShareMode: "url_shared", Title: "件名", ExpiresAt: time.Now().UTC().Add(1 * time.Hour), MaxDownloads: 10, PasswordHash: f.passwordHash}, nil
}
func (f *fakeAccessHandlerStore) GetFilesByShipmentID(ctx context.Context, shipmentID uuid.UUID) ([]store.File, error) {
	return []store.File{{ID: uuid.New(), ShipmentID: shipmentID, OriginalName: "a.txt", SizeBytes: 10}}, nil
}
func (f *fakeAccessHandlerStore) GetFileByID(ctx context.Context, id uuid.UUID) (store.File, error) {
	shipID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	return store.File{ID: id, ShipmentID: shipID, StorageBucket: "b", StorageKey: "k"}, nil
}
func (f *fakeAccessHandlerStore) CountDownloadEventsByShipment(ctx context.Context, shipmentID uuid.UUID) (int32, error) {
	return 0, nil
}
func (f *fakeAccessHandlerStore) CreateDownloadEvent(ctx context.Context, arg store.CreateDownloadEventParams) (store.DownloadEvent, error) {
	return store.DownloadEvent{ID: 1}, nil
}
func (f *fakeAccessHandlerStore) UpdateAccessTokenUsage(ctx context.Context, tokenID uuid.UUID) error {
	return nil
}
func (f *fakeAccessHandlerStore) IncrementShipmentDownloadCount(ctx context.Context, shipmentID uuid.UUID) error {
	return nil
}

type fakeAccessHandlerObjectStore struct{}

func (f *fakeAccessHandlerObjectStore) CreateMultipartUpload(ctx context.Context, bucket, key, contentType string) (string, error) {
	panic("unexpected")
}
func (f *fakeAccessHandlerObjectStore) BatchPresignUploadParts(ctx context.Context, bucket, key, uploadID string, partCount int, expiresIn time.Duration) ([]storage.PresignedPart, error) {
	panic("unexpected")
}
func (f *fakeAccessHandlerObjectStore) CompleteMultipartUpload(ctx context.Context, bucket, key, uploadID string, parts []storage.CompletedPart) error {
	panic("unexpected")
}
func (f *fakeAccessHandlerObjectStore) GenerateDownloadURL(ctx context.Context, bucket, key string, expiresIn time.Duration) (string, error) {
	return "https://example.com/dl", nil
}
func (f *fakeAccessHandlerObjectStore) DeleteObject(ctx context.Context, bucket, key string) error {
	return nil
}

func TestInspectAccessHandler_Success(t *testing.T) {
	svc := &service.AccessService{Store: &fakeAccessHandlerStore{}}
	h := AccessHandler{Service: svc}
	r := chi.NewRouter()
	r.Get("/v1/access/{token}", h.InspectAccess)

	req := httptest.NewRequest(http.MethodGet, "/v1/access/raw-token", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestVerifyAccessHandler_SetsHttpOnlyGrantCookie(t *testing.T) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	hash := string(passwordHash)
	svc := &service.AccessService{
		Store:             &fakeAccessHandlerStore{passwordHash: &hash},
		AccessGrantSecret: "handler-test-access-grant-secret-32-bytes",
		AccessGrantTTL:    5 * time.Minute,
	}
	h := AccessHandler{Service: svc, CookieSecure: true, CookieSameSite: http.SameSiteLaxMode}
	r := chi.NewRouter()
	r.Post("/v1/access/{token}/verify", h.VerifyAccess)

	body, _ := json.Marshal(map[string]string{"password": "correct-password"})
	req := httptest.NewRequest(http.MethodPost, "/v1/access/raw-token/verify", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie got=%d", len(cookies))
	}
	cookie := cookies[0]
	if !strings.HasPrefix(cookie.Name, accessGrantCookiePrefix) || !cookie.HttpOnly || !cookie.Secure || cookie.Value == "" {
		t.Fatalf("unexpected cookie: %+v", cookie)
	}
}

func TestDownloadURLHandler_FileIDValidation(t *testing.T) {
	svc := &service.AccessService{Store: &fakeAccessHandlerStore{}, ObjectStore: &fakeAccessHandlerObjectStore{}}
	h := AccessHandler{Service: svc}
	r := chi.NewRouter()
	r.Get("/v1/files/{id}/download-url", h.GenerateDownloadURL)

	req := httptest.NewRequest(http.MethodGet, "/v1/files/bad/download-url?access_token=raw-token", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestDownloadURLHandler_ProtectedShipmentWithoutCookie(t *testing.T) {
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	hash := string(passwordHash)
	fileID := uuid.New()
	svc := &service.AccessService{
		Store:             &fakeAccessHandlerStore{passwordHash: &hash},
		ObjectStore:       &fakeAccessHandlerObjectStore{},
		AccessGrantSecret: "handler-test-access-grant-secret-32-bytes",
	}
	h := AccessHandler{Service: svc}
	r := chi.NewRouter()
	r.Get("/v1/files/{id}/download-url", h.GenerateDownloadURL)

	req := httptest.NewRequest(http.MethodGet, "/v1/files/"+fileID.String()+"/download-url?access_token=raw-token", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
