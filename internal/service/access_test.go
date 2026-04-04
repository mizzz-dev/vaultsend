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
	shipID := uuid.New()
	b, _ := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	h := string(b)
	fs := &fakeAccessStore{
		token:    store.AccessToken{ID: uuid.New(), ShipmentID: shipID, TokenType: "download_access", ExpiresAt: time.Now().UTC().Add(1 * time.Hour), MaxUses: 10, Status: "active"},
		shipment: store.Shipment{ID: shipID, Status: "sent", ShareMode: "url_shared", Title: "件名", ExpiresAt: time.Now().UTC().Add(1 * time.Hour), MaxDownloads: 10, PasswordHash: &h},
	}
	svc := &AccessService{Store: fs}
	wrong := "wrong-password"
	err := svc.VerifyAccess(context.Background(), VerifyAccessInput{Token: "raw-token", Password: &wrong})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 401 {
		t.Fatalf("expected 401 got=%v", err)
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
