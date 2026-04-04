package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/vaultsend/internal/queue"
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

type fakeMailQueue struct {
	events []queue.MailNotification
	err    error
}

func (q *fakeMailQueue) EnqueueMail(ctx context.Context, msg queue.MailNotification) error {
	if q.err != nil {
		return q.err
	}
	q.events = append(q.events, msg)
	return nil
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
		f.finalizeOut.Shipment = store.Shipment{ID: arg.ShipmentID, Status: arg.Status, ExpiresAt: arg.ExpiresAt, MaxDownloads: arg.MaxDownloads, Title: arg.Title, Message: arg.Message}
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
	svc := &ShipmentService{Store: fs, FrontendURL: "https://frontend.example.com"}
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
	recipientID := uuid.New()
	queueMock := &fakeMailQueue{}
	fs := &fakeShipmentStore{
		filesByIDs: []store.FileWithShipment{{File: store.File{ID: fileID, ShipmentID: shipID, OriginalName: "a.txt", SizeBytes: 10, UploadStatus: "completed"}, ShipmentStatus: "ready"}},
		finalizeOut: store.ShipmentFinalizeResult{
			Shipment:   store.Shipment{ID: shipID, Status: "sent", Title: "件名", ExpiresAt: time.Now().UTC().Add(24 * time.Hour), MaxDownloads: 10},
			Recipients: []store.Recipient{{ID: recipientID, Email: "a@example.com", EmailNormalized: "a@example.com", Status: "pending"}},
		},
	}
	svc := &ShipmentService{Store: fs, Queue: queueMock, FrontendURL: "https://frontend.example.com"}
	_, err := svc.CreateShipment(context.Background(), CreateShipmentInput{FileIDs: []uuid.UUID{fileID}, Subject: "件名", ShareMode: "recipient_restricted", Recipients: []ShipmentRecipientInput{{Email: "A@example.com"}, {Email: "a@example.com"}}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(fs.finalizeArg.Recipients) != 1 {
		t.Fatalf("expected dedup recipient got=%d", len(fs.finalizeArg.Recipients))
	}
	if len(queueMock.events) != 1 {
		t.Fatalf("expected enqueue once got=%d", len(queueMock.events))
	}
	if queueMock.events[0].RecipientID != recipientID {
		t.Fatalf("unexpected recipient id: %s", queueMock.events[0].RecipientID)
	}
}

func TestCreateShipment_RecipientRestricted_EnqueueError(t *testing.T) {
	shipID := uuid.New()
	fileID := uuid.New()
	recipientID := uuid.New()
	fs := &fakeShipmentStore{
		filesByIDs: []store.FileWithShipment{{File: store.File{ID: fileID, ShipmentID: shipID, UploadStatus: "completed"}, ShipmentStatus: "ready"}},
		finalizeOut: store.ShipmentFinalizeResult{
			Shipment:   store.Shipment{ID: shipID, Status: "sent", Title: "件名", ExpiresAt: time.Now().UTC().Add(24 * time.Hour), MaxDownloads: 10},
			Recipients: []store.Recipient{{ID: recipientID, Email: "a@example.com", EmailNormalized: "a@example.com", Status: "pending"}},
		},
	}
	svc := &ShipmentService{Store: fs, Queue: &fakeMailQueue{err: errors.New("queue down")}}
	_, err := svc.CreateShipment(context.Background(), CreateShipmentInput{FileIDs: []uuid.UUID{fileID}, Subject: "件名", ShareMode: "recipient_restricted", Recipients: []ShipmentRecipientInput{{Email: "a@example.com"}}})
	if err == nil {
		t.Fatal("expected enqueue error")
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
