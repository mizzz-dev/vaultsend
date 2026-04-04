package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/vaultsend/internal/store"
	"github.com/google/uuid"
)

type fakeShipmentStore struct {
	shipment      store.Shipment
	filesByIDs    []store.FileWithShipment
	finalizeOut   store.ShipmentFinalizeResult
	finalizeArg   store.FinalizeShipmentParams
	finalizeErr   error
	recipientsOut []store.Recipient
	filesOut      []store.File
}

func (f *fakeShipmentStore) GetShipment(ctx context.Context, id uuid.UUID) (store.Shipment, error) {
	if f.shipment.ID == uuid.Nil {
		return store.Shipment{}, store.ErrNotFound
	}
	return f.shipment, nil
}

func (f *fakeShipmentStore) GetFilesByIDs(ctx context.Context, ids []uuid.UUID) ([]store.FileWithShipment, error) {
	return f.filesByIDs, nil
}

func (f *fakeShipmentStore) FinalizeShipment(ctx context.Context, arg store.FinalizeShipmentParams) (store.ShipmentFinalizeResult, error) {
	f.finalizeArg = arg
	if f.finalizeErr != nil {
		return store.ShipmentFinalizeResult{}, f.finalizeErr
	}
	if f.finalizeOut.Shipment.ID == uuid.Nil {
		f.finalizeOut.Shipment = store.Shipment{ID: arg.ShipmentID, Status: arg.Status, ExpiresAt: arg.ExpiresAt, MaxDownloads: arg.MaxDownloads}
	}
	return f.finalizeOut, nil
}

func (f *fakeShipmentStore) GetFilesByShipmentID(ctx context.Context, shipmentID uuid.UUID) ([]store.File, error) {
	return f.filesOut, nil
}

func (f *fakeShipmentStore) GetRecipientsByShipmentID(ctx context.Context, shipmentID uuid.UUID) ([]store.Recipient, error) {
	return f.recipientsOut, nil
}

func TestCreateShipment_URLShared_Success(t *testing.T) {
	shipID := uuid.New()
	fileID := uuid.New()
	fs := &fakeShipmentStore{
		filesByIDs:  []store.FileWithShipment{{File: store.File{ID: fileID, ShipmentID: shipID, OriginalName: "a.txt", SizeBytes: 10, UploadStatus: "completed"}, ShipmentStatus: "ready"}},
		finalizeOut: store.ShipmentFinalizeResult{Files: []store.File{{ID: fileID, OriginalName: "a.txt", SizeBytes: 10}}},
	}
	svc := &ShipmentService{Store: fs}
	out, err := svc.CreateShipment(context.Background(), CreateShipmentInput{FileIDs: []uuid.UUID{fileID}, Subject: "件名", ShareMode: "url_shared"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.AccessURL == nil || out.Status != "sent" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestCreateShipment_RecipientRestricted_Success(t *testing.T) {
	shipID := uuid.New()
	fileID := uuid.New()
	fs := &fakeShipmentStore{
		filesByIDs:  []store.FileWithShipment{{File: store.File{ID: fileID, ShipmentID: shipID, OriginalName: "a.txt", SizeBytes: 10, UploadStatus: "completed"}, ShipmentStatus: "ready"}},
		finalizeOut: store.ShipmentFinalizeResult{Recipients: []store.Recipient{{ID: uuid.New(), Email: "a@example.com", Status: "pending"}}},
	}
	svc := &ShipmentService{Store: fs}
	_, err := svc.CreateShipment(context.Background(), CreateShipmentInput{FileIDs: []uuid.UUID{fileID}, Subject: "件名", ShareMode: "recipient_restricted", Recipients: []ShipmentRecipientInput{{Email: "A@example.com"}, {Email: "a@example.com"}}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(fs.finalizeArg.Recipients) != 1 {
		t.Fatalf("expected dedup recipient got=%d", len(fs.finalizeArg.Recipients))
	}
}

func TestCreateShipment_InvalidRecipients(t *testing.T) {
	shipID := uuid.New()
	fileID := uuid.New()
	svc := &ShipmentService{Store: &fakeShipmentStore{filesByIDs: []store.FileWithShipment{{File: store.File{ID: fileID, ShipmentID: shipID, UploadStatus: "completed"}, ShipmentStatus: "ready"}}}}
	_, err := svc.CreateShipment(context.Background(), CreateShipmentInput{FileIDs: []uuid.UUID{fileID}, Subject: "件名", ShareMode: "recipient_restricted", Recipients: []ShipmentRecipientInput{{Email: "not-email"}}})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 400 {
		t.Fatalf("expected 400 got=%v", err)
	}
}

func TestCreateShipment_FileStatusConflict(t *testing.T) {
	shipID := uuid.New()
	fileID := uuid.New()
	svc := &ShipmentService{Store: &fakeShipmentStore{filesByIDs: []store.FileWithShipment{{File: store.File{ID: fileID, ShipmentID: shipID, UploadStatus: "initiated"}, ShipmentStatus: "uploading"}}}}
	_, err := svc.CreateShipment(context.Background(), CreateShipmentInput{FileIDs: []uuid.UUID{fileID}, Subject: "件名", ShareMode: "url_shared"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 409 {
		t.Fatalf("expected 409 got=%v", err)
	}
}

func TestCreateShipment_StoreConflict(t *testing.T) {
	shipID := uuid.New()
	fileID := uuid.New()
	svc := &ShipmentService{Store: &fakeShipmentStore{
		filesByIDs:  []store.FileWithShipment{{File: store.File{ID: fileID, ShipmentID: shipID, UploadStatus: "completed"}, ShipmentStatus: "ready"}},
		finalizeErr: store.ErrConflict,
	}}
	_, err := svc.CreateShipment(context.Background(), CreateShipmentInput{FileIDs: []uuid.UUID{fileID}, Subject: "件名", ShareMode: "url_shared", ExpiresAt: ptrTime(time.Now().UTC().Add(48 * time.Hour))})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 409 {
		t.Fatalf("expected 409 got=%v", err)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
