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
	shipment           store.Shipment
	filesByIDs         []store.FileWithShipment
	finalizeOut        store.ShipmentFinalizeResult
	finalizeArg        store.FinalizeShipmentParams
	finalizeErr        error
	recipientsOut      []store.Recipient
	filesOut           []store.File
	listOut            []store.ShipmentListItem
	totalOut           int64
	deleteErr          error
	revokeErr          error
	recipientStat      []store.RecipientDownloadStat
	notificationStat   []store.RecipientNotificationStat
	notificationEvents []store.NotificationEvent
	notificationList   []store.NotificationEventListItem
	notificationCount  int64
	eventCreates       []store.CreateNotificationEventParams
	tokenCreates       []store.CreateAccessTokenParams
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
func (f *fakeShipmentStore) ListRecipientsByIDsAndShipmentID(ctx context.Context, shipmentID uuid.UUID, recipientIDs []uuid.UUID) ([]store.Recipient, error) {
	idSet := map[uuid.UUID]struct{}{}
	for _, id := range recipientIDs {
		idSet[id] = struct{}{}
	}
	out := make([]store.Recipient, 0, len(recipientIDs))
	for _, rc := range f.recipientsOut {
		if _, ok := idSet[rc.ID]; ok {
			out = append(out, rc)
		}
	}
	return out, nil
}
func (f *fakeShipmentStore) CreateAccessToken(ctx context.Context, shipmentID uuid.UUID, arg store.CreateAccessTokenParams) error {
	f.tokenCreates = append(f.tokenCreates, arg)
	return nil
}
func (f *fakeShipmentStore) CreateNotificationEvent(ctx context.Context, arg store.CreateNotificationEventParams) (store.NotificationEvent, error) {
	f.eventCreates = append(f.eventCreates, arg)
	return store.NotificationEvent{ID: int64(len(f.eventCreates)), ShipmentID: arg.ShipmentID, RecipientID: arg.RecipientID, EventType: arg.EventType, Status: arg.Status}, nil
}
func (f *fakeShipmentStore) ListShipmentsByUser(ctx context.Context, ownerUserID uuid.UUID, limit int32, offset int32) ([]store.ShipmentListItem, error) {
	return f.listOut, nil
}
func (f *fakeShipmentStore) CountShipmentsByUser(ctx context.Context, ownerUserID uuid.UUID) (int64, error) {
	return f.totalOut, nil
}
func (f *fakeShipmentStore) GetRecipientDownloadStatsByShipment(ctx context.Context, shipmentID uuid.UUID) ([]store.RecipientDownloadStat, error) {
	return f.recipientStat, nil
}
func (f *fakeShipmentStore) GetNotificationEventsByShipmentID(ctx context.Context, shipmentID uuid.UUID) ([]store.NotificationEvent, error) {
	return f.notificationEvents, nil
}
func (f *fakeShipmentStore) ListNotificationEventsByShipmentID(ctx context.Context, shipmentID uuid.UUID, limit int32, offset int32) ([]store.NotificationEventListItem, error) {
	return f.notificationList, nil
}
func (f *fakeShipmentStore) CountNotificationEventsByShipmentID(ctx context.Context, shipmentID uuid.UUID) (int64, error) {
	return f.notificationCount, nil
}
func (f *fakeShipmentStore) GetRecipientNotificationStatsByShipmentID(ctx context.Context, shipmentID uuid.UUID) ([]store.RecipientNotificationStat, error) {
	return f.notificationStat, nil
}
func (f *fakeShipmentStore) CountDownloadEventsByShipment(ctx context.Context, shipmentID uuid.UUID) (int32, error) {
	return 0, nil
}
func (f *fakeShipmentStore) DeleteShipment(ctx context.Context, shipmentID uuid.UUID) error {
	return f.deleteErr
}
func (f *fakeShipmentStore) RevokeAccessTokensByShipment(ctx context.Context, shipmentID uuid.UUID) error {
	return f.revokeErr
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
	if len(fs.eventCreates) != 1 || fs.eventCreates[0].EventType != "initial_send" {
		t.Fatalf("expected initial_send event create got=%+v", fs.eventCreates)
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

func TestListShipmentsByUser_Success(t *testing.T) {
	userID := uuid.New()
	svc := &ShipmentService{Store: &fakeShipmentStore{
		listOut:  []store.ShipmentListItem{{ID: uuid.New(), Title: "件名", ShareMode: "url_shared", Status: "sent", MaxDownloads: 10, FileCount: 2, DownloadCount: 1}},
		totalOut: 1,
	}}
	out, err := svc.ListShipmentsByUser(context.Background(), ShipmentListInput{OwnerUserID: userID, Limit: 20, Offset: 0})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out.Items) != 1 || out.Items[0].Subject != "件名" || out.Total != 1 {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestGetShipmentDetailByUser_Forbidden(t *testing.T) {
	ownerID := uuid.New()
	shipID := uuid.New()
	st := &fakeShipmentStore{
		shipment: store.Shipment{ID: uuid.New(), OwnerUserID: &ownerID, ShareMode: "url_shared", Status: "sent"},
	}
	st.shipment.ID = shipID
	svc := &ShipmentService{Store: st}
	_, err := svc.GetShipmentDetailByUser(context.Background(), uuid.New(), shipID)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 403 {
		t.Fatalf("expected 403 got=%v", err)
	}
}

func TestGetShipmentDetailByUser_WithNotificationAndRecipientSummary(t *testing.T) {
	ownerID := uuid.New()
	shipID := uuid.New()
	recipientID := uuid.New()
	now := time.Now().UTC()
	lastSent := now.Add(-1 * time.Hour)
	firstDL := now.Add(-30 * time.Minute)
	lastDL := now.Add(-10 * time.Minute)
	statusSent := "sent"
	eventType := "resend"
	st := &fakeShipmentStore{
		shipment: store.Shipment{ID: shipID, OwnerUserID: &ownerID, ShareMode: "recipient_restricted", Status: "sent", Title: "件名", ExpiresAt: now.Add(24 * time.Hour), MaxDownloads: 10},
		recipientsOut: []store.Recipient{
			{ID: recipientID, ShipmentID: shipID, Email: "a@example.com", Status: "pending"},
		},
		recipientStat: []store.RecipientDownloadStat{
			{RecipientID: recipientID, Email: "a@example.com", DownloadCount: 2, FirstDownloadAt: &firstDL, LastDownloadAt: &lastDL},
		},
		notificationEvents: []store.NotificationEvent{
			{ID: 1, ShipmentID: shipID, RecipientID: recipientID, Status: "queued", CreatedAt: now.Add(-2 * time.Hour)},
			{ID: 2, ShipmentID: shipID, RecipientID: recipientID, Status: "sent", CreatedAt: lastSent},
			{ID: 3, ShipmentID: shipID, RecipientID: recipientID, Status: "failed", CreatedAt: now.Add(-90 * time.Minute)},
		},
		notificationStat: []store.RecipientNotificationStat{
			{RecipientID: recipientID, Email: "a@example.com", NotificationCount: 3, LastNotificationStatus: &statusSent, LastNotificationType: &eventType, LastNotificationAt: &lastSent},
		},
	}
	svc := &ShipmentService{Store: st}
	out, err := svc.GetShipmentDetailByUser(context.Background(), ownerID, shipID)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.NotificationSummary.TotalNotifications != 3 || out.NotificationSummary.QueuedCount != 1 || out.NotificationSummary.SentCount != 1 || out.NotificationSummary.FailedCount != 1 {
		t.Fatalf("unexpected notification summary: %+v", out.NotificationSummary)
	}
	if len(out.RecipientSummaries) != 1 || !out.RecipientSummaries[0].HasDownloaded || out.RecipientSummaries[0].DownloadCount != 2 {
		t.Fatalf("unexpected recipient summary: %+v", out.RecipientSummaries)
	}
}

func TestListShipmentNotificationsByUser_Success(t *testing.T) {
	ownerID := uuid.New()
	shipID := uuid.New()
	recipientID := uuid.New()
	st := &fakeShipmentStore{
		shipment: store.Shipment{ID: shipID, OwnerUserID: &ownerID, ShareMode: "recipient_restricted", Status: "sent"},
		notificationList: []store.NotificationEventListItem{
			{
				NotificationEvent: store.NotificationEvent{
					ID:          10,
					ShipmentID:  shipID,
					RecipientID: recipientID,
					EventType:   "initial_send",
					Status:      "sent",
					CreatedAt:   time.Now().UTC(),
				},
				RecipientEmail: "a@example.com",
			},
		},
		notificationCount: 1,
	}
	svc := &ShipmentService{Store: st}
	out, err := svc.ListShipmentNotificationsByUser(context.Background(), ListShipmentNotificationsInput{
		OwnerUserID: ownerID,
		ShipmentID:  shipID,
		Limit:       20,
		Offset:      0,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.Total != 1 || len(out.Items) != 1 || out.Items[0].RecipientEmail != "a@example.com" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestListShipmentNotificationsByUser_ForbiddenAndNotFound(t *testing.T) {
	ownerID := uuid.New()
	otherID := uuid.New()
	shipID := uuid.New()
	svcForbidden := &ShipmentService{Store: &fakeShipmentStore{
		shipment: store.Shipment{ID: shipID, OwnerUserID: &otherID, ShareMode: "recipient_restricted", Status: "sent"},
	}}
	_, err := svcForbidden.ListShipmentNotificationsByUser(context.Background(), ListShipmentNotificationsInput{
		OwnerUserID: ownerID,
		ShipmentID:  shipID,
		Limit:       20,
		Offset:      0,
	})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 403 {
		t.Fatalf("expected 403 got=%v", err)
	}

	svcNotFound := &ShipmentService{Store: &fakeShipmentStore{}}
	_, err = svcNotFound.ListShipmentNotificationsByUser(context.Background(), ListShipmentNotificationsInput{
		OwnerUserID: ownerID,
		ShipmentID:  shipID,
		Limit:       20,
		Offset:      0,
	})
	if !errors.As(err, &apiErr) || apiErr.Status != 404 {
		t.Fatalf("expected 404 got=%v", err)
	}
}

func TestGetShipmentDetailByUser_NoNotifications(t *testing.T) {
	ownerID := uuid.New()
	shipID := uuid.New()
	recipientID := uuid.New()
	svc := &ShipmentService{Store: &fakeShipmentStore{
		shipment:      store.Shipment{ID: shipID, OwnerUserID: &ownerID, ShareMode: "recipient_restricted", Status: "sent", ExpiresAt: time.Now().UTC().Add(24 * time.Hour)},
		recipientsOut: []store.Recipient{{ID: recipientID, ShipmentID: shipID, Email: "none@example.com", Status: "pending"}},
		recipientStat: []store.RecipientDownloadStat{{RecipientID: recipientID, Email: "none@example.com", DownloadCount: 0}},
	}}
	out, err := svc.GetShipmentDetailByUser(context.Background(), ownerID, shipID)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.NotificationSummary.TotalNotifications != 0 || out.NotificationSummary.LastNotificationAt != nil {
		t.Fatalf("expected empty notification summary got=%+v", out.NotificationSummary)
	}
	if len(out.RecipientSummaries) != 1 || out.RecipientSummaries[0].HasDownloaded {
		t.Fatalf("expected not downloaded recipient got=%+v", out.RecipientSummaries)
	}
}

func TestDeleteShipmentByUser_Success(t *testing.T) {
	ownerID := uuid.New()
	shipID := uuid.New()
	svc := &ShipmentService{Store: &fakeShipmentStore{
		shipment: store.Shipment{ID: shipID, OwnerUserID: &ownerID, Status: "sent"},
	}}
	if err := svc.DeleteShipmentByUser(context.Background(), ownerID, shipID); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestResendShipmentNotification_Success(t *testing.T) {
	ownerID := uuid.New()
	shipID := uuid.New()
	recipientID := uuid.New()
	fs := &fakeShipmentStore{
		shipment:      store.Shipment{ID: shipID, OwnerUserID: &ownerID, ShareMode: "recipient_restricted", Status: "sent", Title: "件名", ExpiresAt: time.Now().UTC().Add(24 * time.Hour)},
		recipientsOut: []store.Recipient{{ID: recipientID, ShipmentID: shipID, Email: "a@example.com"}},
	}
	queueMock := &fakeMailQueue{}
	svc := &ShipmentService{Store: fs, Queue: queueMock}
	out, err := svc.ResendShipmentNotification(context.Background(), ResendShipmentInput{OwnerUserID: ownerID, ShipmentID: shipID})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.ResentRecipientCount != 1 || len(queueMock.events) != 1 {
		t.Fatalf("unexpected output=%+v queued=%d", out, len(queueMock.events))
	}
	if len(fs.eventCreates) != 1 || fs.eventCreates[0].EventType != "resend" {
		t.Fatalf("expected resend event create got=%+v", fs.eventCreates)
	}
	if len(fs.tokenCreates) != 1 || fs.tokenCreates[0].RecipientID == nil || *fs.tokenCreates[0].RecipientID != recipientID {
		t.Fatalf("expected token create for recipient got=%+v", fs.tokenCreates)
	}
}

func TestResendShipmentNotification_ForbidOrConflict(t *testing.T) {
	ownerID := uuid.New()
	shipID := uuid.New()
	cases := []struct {
		name     string
		shipment store.Shipment
		userID   uuid.UUID
		status   int
	}{
		{name: "owner mismatch", shipment: store.Shipment{ID: shipID, OwnerUserID: ptrUUID(uuid.New()), ShareMode: "recipient_restricted", Status: "sent", ExpiresAt: time.Now().UTC().Add(24 * time.Hour)}, userID: ownerID, status: 403},
		{name: "url_shared", shipment: store.Shipment{ID: shipID, OwnerUserID: &ownerID, ShareMode: "url_shared", Status: "sent", ExpiresAt: time.Now().UTC().Add(24 * time.Hour)}, userID: ownerID, status: 409},
		{name: "expired", shipment: store.Shipment{ID: shipID, OwnerUserID: &ownerID, ShareMode: "recipient_restricted", Status: "sent", ExpiresAt: time.Now().UTC().Add(-1 * time.Hour)}, userID: ownerID, status: 409},
		{name: "deleted", shipment: store.Shipment{ID: shipID, OwnerUserID: &ownerID, ShareMode: "recipient_restricted", Status: "deleted", ExpiresAt: time.Now().UTC().Add(24 * time.Hour), DeletedAt: ptrTime(time.Now().UTC())}, userID: ownerID, status: 409},
		{name: "revoked", shipment: store.Shipment{ID: shipID, OwnerUserID: &ownerID, ShareMode: "recipient_restricted", Status: "revoked", ExpiresAt: time.Now().UTC().Add(24 * time.Hour), RevokedAt: ptrTime(time.Now().UTC())}, userID: ownerID, status: 409},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &ShipmentService{Store: &fakeShipmentStore{shipment: tc.shipment}, Queue: &fakeMailQueue{}}
			_, err := svc.ResendShipmentNotification(context.Background(), ResendShipmentInput{OwnerUserID: tc.userID, ShipmentID: shipID})
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.Status != tc.status {
				t.Fatalf("expected status=%d err=%v", tc.status, err)
			}
		})
	}
}

func ptrUUID(id uuid.UUID) *uuid.UUID { return &id }
