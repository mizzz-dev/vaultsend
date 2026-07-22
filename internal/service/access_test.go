package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/vaultsend/internal/storage"
	"github.com/example/vaultsend/internal/store"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const testAccessGrantSecret = "test-access-grant-secret-at-least-32-bytes"

type fakeAccessStore struct {
	token         store.AccessToken
	shipment      store.Shipment
	filesByShip   []store.File
	fileByID      store.File
	downloadCount int32
	usageUpdated  bool
	events        []store.CreateDownloadEventParams
}

func (f *fakeAccessStore) GetAccessTokenByHash(ctx context.Context, tokenHash string) (store.AccessToken, error) {
	if f.token.ID == uuid.Nil {
		return store.AccessToken{}, store.ErrNotFound
	}
	return f.token, nil
}
func (f *fakeAccessStore) GetShipmentByID(ctx context.Context, id uuid.UUID) (store.Shipment, error) {
	if f.shipment.ID == uuid.Nil {
		return store.Shipment{}, store.ErrNotFound
	}
	return f.shipment, nil
}
func (f *fakeAccessStore) GetFilesByShipmentID(ctx context.Context, shipmentID uuid.UUID) ([]store.File, error) {
	return f.filesByShip, nil
}
func (f *fakeAccessStore) GetFileByID(ctx context.Context, id uuid.UUID) (store.File, error) {
	if f.fileByID.ID == uuid.Nil {
		return store.File{}, store.ErrNotFound
	}
	return f.fileByID, nil
}
func (f *fakeAccessStore) CountDownloadEventsByShipment(ctx context.Context, shipmentID uuid.UUID) (int32, error) {
	return f.downloadCount, nil
}
func (f *fakeAccessStore) CreateDownloadEvent(ctx context.Context, arg store.CreateDownloadEventParams) (store.DownloadEvent, error) {
	f.events = append(f.events, arg)
	return store.DownloadEvent{ID: int64(len(f.events))}, nil
}
func (f *fakeAccessStore) UpdateAccessTokenUsage(ctx context.Context, tokenID uuid.UUID) error {
	f.usageUpdated = true
	return nil
}
func (f *fakeAccessStore) IncrementShipmentDownloadCount(ctx context.Context, shipmentID uuid.UUID) error {
	return nil
}

type fakeAccessObjectStore struct{}

func (f *fakeAccessObjectStore) CreateMultipartUpload(ctx context.Context, bucket, key, contentType string) (string, error) {
	panic("unexpected call")
}
func (f *fakeAccessObjectStore) BatchPresignUploadParts(ctx context.Context, bucket, key, uploadID string, partCount int, expiresIn time.Duration) ([]storage.PresignedPart, error) {
	panic("unexpected call")
}
func (f *fakeAccessObjectStore) CompleteMultipartUpload(ctx context.Context, bucket, key, uploadID string, parts []storage.CompletedPart) error {
	panic("unexpected call")
}
func (f *fakeAccessObjectStore) GenerateDownloadURL(ctx context.Context, bucket, key string, expiresIn time.Duration) (string, error) {
	return "https://example.com/dl", nil
}
func (f *fakeAccessObjectStore) DeleteObject(ctx context.Context, bucket, key string) error {
	return nil
}

func TestInspectAccess_Success(t *testing.T) {
	shipID := uuid.New()
	tk := store.AccessToken{ID: uuid.New(), ShipmentID: shipID, TokenType: "download_access", ExpiresAt: time.Now().UTC().Add(1 * time.Hour), MaxUses: 10, Status: "active"}
	fs := &fakeAccessStore{
		token:       tk,
		shipment:    store.Shipment{ID: shipID, Status: "sent", ShareMode: "url_shared", Title: "件名", ExpiresAt: time.Now().UTC().Add(1 * time.Hour), MaxDownloads: 10},
		filesByShip: []store.File{{ID: uuid.New(), OriginalName: "a.txt", SizeBytes: 10}},
	}
	svc := &AccessService{Store: fs}
	out, err := svc.InspectAccess(context.Background(), "raw-token")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.Shipment.ID != shipID || len(out.Files) != 1 {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestVerifyAccess_InvalidPassword(t *testing.T) {
	fs := protectedAccessStore(t)
	svc := &AccessService{Store: fs, AccessGrantSecret: testAccessGrantSecret}
	wrong := "wrong-password"
	_, err := svc.VerifyAccess(context.Background(), VerifyAccessInput{Token: "raw-token", Password: &wrong})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 401 {
		t.Fatalf("expected 401 got=%v", err)
	}
}

func TestVerifyAccess_SuccessIssuesGrant(t *testing.T) {
	fs := protectedAccessStore(t)
	svc := &AccessService{Store: fs, AccessGrantSecret: testAccessGrantSecret, AccessGrantTTL: 5 * time.Minute}
	password := "correct-password"
	out, err := svc.VerifyAccess(context.Background(), VerifyAccessInput{Token: "raw-token", Password: &password})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !out.Granted || out.Grant == "" || !out.ExpiresAt.After(time.Now().UTC()) {
		t.Fatalf("unexpected output: %+v", out)
	}
	if err := validateAccessGrant(testAccessGrantSecret, "raw-token", out.Grant, time.Now().UTC()); err != nil {
		t.Fatalf("issued grant should be valid: %v", err)
	}
}

func TestGenerateDownloadURL_ProtectedShipmentRequiresGrant(t *testing.T) {
	fs := protectedAccessStore(t)
	fileID := uuid.New()
	fs.fileByID = store.File{ID: fileID, ShipmentID: fs.shipment.ID, StorageBucket: "b", StorageKey: "k"}
	fs.filesByShip = []store.File{fs.fileByID}
	svc := &AccessService{Store: fs, ObjectStore: &fakeAccessObjectStore{}, AccessGrantSecret: testAccessGrantSecret}

	_, err := svc.GenerateDownloadURL(context.Background(), DownloadURLInput{Token: "raw-token", FileID: fileID})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 401 || apiErr.Code != "access_verification_required" {
		t.Fatalf("expected verification required got=%v", err)
	}
	if fs.usageUpdated || len(fs.events) != 0 {
		t.Fatal("unverified request must not consume token or create events")
	}
}

func TestGenerateDownloadURL_ProtectedShipmentAcceptsValidGrant(t *testing.T) {
	fs := protectedAccessStore(t)
	fileID := uuid.New()
	fs.fileByID = store.File{ID: fileID, ShipmentID: fs.shipment.ID, StorageBucket: "b", StorageKey: "k"}
	fs.filesByShip = []store.File{fs.fileByID}
	svc := &AccessService{Store: fs, ObjectStore: &fakeAccessObjectStore{}, AccessGrantSecret: testAccessGrantSecret}
	grant, err := issueAccessGrant(testAccessGrantSecret, "raw-token", time.Now().UTC().Add(5*time.Minute))
	if err != nil {
		t.Fatalf("issue grant: %v", err)
	}

	out, err := svc.GenerateDownloadURL(context.Background(), DownloadURLInput{Token: "raw-token", AccessGrant: grant, FileID: fileID, IPAddress: "127.0.0.1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.URL == "" || !fs.usageUpdated || len(fs.events) != 1 {
		t.Fatalf("unexpected result out=%+v events=%d", out, len(fs.events))
	}
}

func TestGenerateDownloadURL_RejectsGrantForDifferentToken(t *testing.T) {
	fs := protectedAccessStore(t)
	fileID := uuid.New()
	fs.fileByID = store.File{ID: fileID, ShipmentID: fs.shipment.ID, StorageBucket: "b", StorageKey: "k"}
	svc := &AccessService{Store: fs, ObjectStore: &fakeAccessObjectStore{}, AccessGrantSecret: testAccessGrantSecret}
	grant, _ := issueAccessGrant(testAccessGrantSecret, "another-token", time.Now().UTC().Add(5*time.Minute))

	_, err := svc.GenerateDownloadURL(context.Background(), DownloadURLInput{Token: "raw-token", AccessGrant: grant, FileID: fileID})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "access_verification_required" {
		t.Fatalf("expected verification required got=%v", err)
	}
}

func TestGenerateDownloadURL_OverLimit(t *testing.T) {
	shipID := uuid.New()
	fileID := uuid.New()
	fs := &fakeAccessStore{
		token:         store.AccessToken{ID: uuid.New(), ShipmentID: shipID, TokenType: "download_access", ExpiresAt: time.Now().UTC().Add(1 * time.Hour), MaxUses: 10, Status: "active"},
		shipment:      store.Shipment{ID: shipID, Status: "sent", ShareMode: "url_shared", Title: "件名", ExpiresAt: time.Now().UTC().Add(1 * time.Hour), MaxDownloads: 1},
		filesByShip:   []store.File{{ID: fileID, ShipmentID: shipID}},
		fileByID:      store.File{ID: fileID, ShipmentID: shipID},
		downloadCount: 1,
	}
	svc := &AccessService{Store: fs, ObjectStore: &fakeAccessObjectStore{}}
	_, err := svc.GenerateDownloadURL(context.Background(), DownloadURLInput{Token: "raw-token", FileID: fileID})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 409 {
		t.Fatalf("expected 409 got=%v", err)
	}
}

func TestGenerateDownloadURL_FileMismatch(t *testing.T) {
	shipID := uuid.New()
	fileID := uuid.New()
	fs := &fakeAccessStore{
		token:       store.AccessToken{ID: uuid.New(), ShipmentID: shipID, TokenType: "download_access", ExpiresAt: time.Now().UTC().Add(1 * time.Hour), MaxUses: 10, Status: "active"},
		shipment:    store.Shipment{ID: shipID, Status: "sent", ShareMode: "url_shared", Title: "件名", ExpiresAt: time.Now().UTC().Add(1 * time.Hour), MaxDownloads: 10},
		filesByShip: []store.File{{ID: fileID, ShipmentID: shipID}},
		fileByID:    store.File{ID: fileID, ShipmentID: uuid.New()},
	}
	svc := &AccessService{Store: fs, ObjectStore: &fakeAccessObjectStore{}}
	_, err := svc.GenerateDownloadURL(context.Background(), DownloadURLInput{Token: "raw-token", FileID: fileID})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 404 {
		t.Fatalf("expected 404 got=%v", err)
	}
}

func TestVerifyAccess_BruteForceLocked(t *testing.T) {
	fs := protectedAccessStore(t)
	guard := NewAccessGuard()
	guard.VerifyMaxAttempts = 2
	svc := &AccessService{Store: fs, Guard: guard, AccessGrantSecret: testAccessGrantSecret}
	wrong := "wrong-password"

	for i := 0; i < 2; i++ {
		_, _ = svc.VerifyAccess(context.Background(), VerifyAccessInput{Token: "raw-token", Password: &wrong})
	}
	_, err := svc.VerifyAccess(context.Background(), VerifyAccessInput{Token: "raw-token", Password: &wrong})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 429 {
		t.Fatalf("expected 429 got=%v", err)
	}
}

func TestGenerateDownloadURL_AbuseBlocked(t *testing.T) {
	shipID := uuid.New()
	fileID := uuid.New()
	fs := &fakeAccessStore{
		token:       store.AccessToken{ID: uuid.New(), ShipmentID: shipID, TokenType: "download_access", ExpiresAt: time.Now().UTC().Add(1 * time.Hour), MaxUses: 10, Status: "active"},
		shipment:    store.Shipment{ID: shipID, Status: "sent", ShareMode: "url_shared", Title: "件名", ExpiresAt: time.Now().UTC().Add(1 * time.Hour), MaxDownloads: 10},
		filesByShip: []store.File{{ID: fileID, ShipmentID: shipID}},
		fileByID:    store.File{ID: fileID, ShipmentID: shipID, StorageBucket: "b", StorageKey: "k"},
	}
	guard := NewAccessGuard()
	guard.DownloadLimit = 1
	svc := &AccessService{Store: fs, ObjectStore: &fakeAccessObjectStore{}, Guard: guard}

	_, err := svc.GenerateDownloadURL(context.Background(), DownloadURLInput{Token: "raw-token", FileID: fileID, IPAddress: "127.0.0.1"})
	if err != nil {
		t.Fatalf("unexpected first err: %v", err)
	}
	_, err = svc.GenerateDownloadURL(context.Background(), DownloadURLInput{Token: "raw-token", FileID: fileID, IPAddress: "127.0.0.1"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 429 {
		t.Fatalf("expected 429 got=%v", err)
	}
}

func protectedAccessStore(t *testing.T) *fakeAccessStore {
	t.Helper()
	shipID := uuid.New()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	hash := string(passwordHash)
	return &fakeAccessStore{
		token:    store.AccessToken{ID: uuid.New(), ShipmentID: shipID, TokenType: "download_access", ExpiresAt: time.Now().UTC().Add(1 * time.Hour), MaxUses: 10, Status: "active"},
		shipment: store.Shipment{ID: shipID, Status: "sent", ShareMode: "url_shared", Title: "件名", ExpiresAt: time.Now().UTC().Add(1 * time.Hour), MaxDownloads: 10, PasswordHash: &hash},
	}
}
