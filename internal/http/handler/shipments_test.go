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
	"github.com/example/vaultsend/internal/queue"
	"github.com/example/vaultsend/internal/service"
	"github.com/example/vaultsend/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type fakeShipmentSvcStore struct {
	lastOwnerUserID *uuid.UUID
	shipment        store.Shipment
	shipmentErr     error
}

type fakeQueue struct{}

func (f *fakeQueue) EnqueueMail(ctx context.Context, msg queue.MailNotification) error { return nil }

func (f *fakeShipmentSvcStore) GetShipment(ctx context.Context, id uuid.UUID) (store.Shipment, error) {
	if f.shipmentErr != nil {
		return store.Shipment{}, f.shipmentErr
	}
	if f.shipment.ID != uuid.Nil {
		return f.shipment, nil
	}
	return store.Shipment{ID: id, Status: "sent", ShareMode: "url_shared", Title: "件名", ExpiresAt: time.Now().UTC(), MaxDownloads: 10}, nil
}
func (f *fakeShipmentSvcStore) GetFilesByIDs(ctx context.Context, ids []uuid.UUID) ([]store.FileWithShipment, error) {
	out := make([]store.FileWithShipment, 0, len(ids))
	for _, id := range ids {
		out = append(out, store.FileWithShipment{File: store.File{ID: id, ShipmentID: uuid.New(), UploadStatus: "completed"}, ShipmentStatus: "ready"})
	}
	return out, nil
}
func (f *fakeShipmentSvcStore) FinalizeShipment(ctx context.Context, arg store.FinalizeShipmentParams) (store.ShipmentFinalizeResult, error) {
	f.lastOwnerUserID = arg.OwnerUserID
	return store.ShipmentFinalizeResult{Shipment: store.Shipment{ID: arg.ShipmentID, Status: "sent", ExpiresAt: arg.ExpiresAt, MaxDownloads: arg.MaxDownloads}}, nil
}
func (f *fakeShipmentSvcStore) GetFilesByShipmentID(ctx context.Context, shipmentID uuid.UUID) ([]store.File, error) {
	return []store.File{{ID: uuid.New(), OriginalName: "a.txt", SizeBytes: 1}}, nil
}
func (f *fakeShipmentSvcStore) GetRecipientsByShipmentID(ctx context.Context, shipmentID uuid.UUID) ([]store.Recipient, error) {
	return []store.Recipient{{ID: uuid.New(), Email: "a@example.com", Status: "pending"}}, nil
}
func (f *fakeShipmentSvcStore) ListRecipientsByIDsAndShipmentID(ctx context.Context, shipmentID uuid.UUID, recipientIDs []uuid.UUID) ([]store.Recipient, error) {
	out := make([]store.Recipient, 0, len(recipientIDs))
	for _, id := range recipientIDs {
		out = append(out, store.Recipient{ID: id, ShipmentID: shipmentID, Email: "a@example.com"})
	}
	return out, nil
}
func (f *fakeShipmentSvcStore) CreateAccessToken(ctx context.Context, shipmentID uuid.UUID, arg store.CreateAccessTokenParams) error {
	return nil
}
func (f *fakeShipmentSvcStore) CreateNotificationEvent(ctx context.Context, arg store.CreateNotificationEventParams) (store.NotificationEvent, error) {
	return store.NotificationEvent{ID: 1, ShipmentID: arg.ShipmentID, RecipientID: arg.RecipientID, EventType: arg.EventType, Status: arg.Status}, nil
}
func (f *fakeShipmentSvcStore) ListShipmentsByUser(ctx context.Context, ownerUserID uuid.UUID, limit int32, offset int32) ([]store.ShipmentListItem, error) {
	return []store.ShipmentListItem{{ID: uuid.New(), Title: "件名", ShareMode: "url_shared", Status: "sent", MaxDownloads: 10, FileCount: 1}}, nil
}
func (f *fakeShipmentSvcStore) CountShipmentsByUser(ctx context.Context, ownerUserID uuid.UUID) (int64, error) {
	return 1, nil
}
func (f *fakeShipmentSvcStore) ListShipmentsAccessibleByUser(ctx context.Context, userID uuid.UUID, limit int32, offset int32) ([]store.ShipmentListItem, error) {
	return f.ListShipmentsByUser(ctx, userID, limit, offset)
}
func (f *fakeShipmentSvcStore) CountShipmentsAccessibleByUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	return 1, nil
}
func (f *fakeShipmentSvcStore) GetRecipientDownloadStatsByShipment(ctx context.Context, shipmentID uuid.UUID) ([]store.RecipientDownloadStat, error) {
	return []store.RecipientDownloadStat{{RecipientID: uuid.New(), Email: "a@example.com", DownloadCount: 0}}, nil
}
func (f *fakeShipmentSvcStore) GetNotificationEventsByShipmentID(ctx context.Context, shipmentID uuid.UUID) ([]store.NotificationEvent, error) {
	return []store.NotificationEvent{{ID: 1, ShipmentID: shipmentID, RecipientID: uuid.New(), Status: "queued", EventType: "initial_send", CreatedAt: time.Now().UTC()}}, nil
}
func (f *fakeShipmentSvcStore) ListNotificationEventsByShipmentID(ctx context.Context, shipmentID uuid.UUID, limit int32, offset int32) ([]store.NotificationEventListItem, error) {
	return []store.NotificationEventListItem{
		{
			NotificationEvent: store.NotificationEvent{ID: 1, ShipmentID: shipmentID, RecipientID: uuid.New(), EventType: "initial_send", Status: "sent", CreatedAt: time.Now().UTC()},
			RecipientEmail:    "a@example.com",
		},
	}, nil
}
func (f *fakeShipmentSvcStore) CountNotificationEventsByShipmentID(ctx context.Context, shipmentID uuid.UUID) (int64, error) {
	return 1, nil
}
func (f *fakeShipmentSvcStore) GetRecipientNotificationStatsByShipmentID(ctx context.Context, shipmentID uuid.UUID) ([]store.RecipientNotificationStat, error) {
	return []store.RecipientNotificationStat{{RecipientID: uuid.New(), Email: "a@example.com", NotificationCount: 1}}, nil
}
func (f *fakeShipmentSvcStore) CountDownloadEventsByShipment(ctx context.Context, shipmentID uuid.UUID) (int32, error) {
	return 0, nil
}
func (f *fakeShipmentSvcStore) DeleteShipment(ctx context.Context, shipmentID uuid.UUID) error {
	return nil
}
func (f *fakeShipmentSvcStore) RevokeAccessTokensByShipment(ctx context.Context, shipmentID uuid.UUID) error {
	return nil
}

func TestCreateShipmentHandler_Success(t *testing.T) {
	svc := &service.ShipmentService{Store: &fakeShipmentSvcStore{}}
	h := ShipmentHandler{Service: svc}
	payload, _ := json.Marshal(map[string]any{
		"file_ids":   []string{uuid.NewString()},
		"subject":    "請求書",
		"share_mode": "url_shared",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/shipments", bytes.NewReader(payload))
	w := httptest.NewRecorder()
	h.CreateShipment(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateShipmentHandler_BadExpiresAt(t *testing.T) {
	svc := &service.ShipmentService{Store: &fakeShipmentSvcStore{}}
	h := ShipmentHandler{Service: svc}
	payload, _ := json.Marshal(map[string]any{
		"file_ids":   []string{uuid.NewString()},
		"subject":    "請求書",
		"share_mode": "url_shared",
		"expires_at": "bad",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/shipments", bytes.NewReader(payload))
	w := httptest.NewRecorder()
	h.CreateShipment(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestGetShipmentHandler_Success(t *testing.T) {
	svc := &service.ShipmentService{Store: &fakeShipmentSvcStore{}}
	h := ShipmentHandler{Service: svc}
	r := chi.NewRouter()
	r.Get("/v1/shipments/{id}", h.GetShipment)
	req := httptest.NewRequest(http.MethodGet, "/v1/shipments/"+uuid.NewString(), nil)
	req = req.WithContext(middleware.WithAuthUser(req.Context(), service.AuthUser{ID: uuid.New(), Email: "a@example.com"}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListShipmentsHandler_Pagination(t *testing.T) {
	svc := &service.ShipmentService{Store: &fakeShipmentSvcStore{}}
	h := ShipmentHandler{Service: svc}
	req := httptest.NewRequest(http.MethodGet, "/v1/shipments?limit=10&offset=0", nil)
	req = req.WithContext(middleware.WithAuthUser(req.Context(), service.AuthUser{ID: uuid.New(), Email: "a@example.com"}))
	w := httptest.NewRecorder()
	h.ListShipments(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListShipmentNotificationsHandler_Success(t *testing.T) {
	svc := &service.ShipmentService{Store: &fakeShipmentSvcStore{}}
	h := ShipmentHandler{Service: svc}
	r := chi.NewRouter()
	r.Get("/v1/shipments/{id}/notifications", h.ListShipmentNotifications)
	req := httptest.NewRequest(http.MethodGet, "/v1/shipments/"+uuid.NewString()+"/notifications?limit=10&offset=0", nil)
	req = req.WithContext(middleware.WithAuthUser(req.Context(), service.AuthUser{ID: uuid.New(), Email: "a@example.com"}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListShipmentNotificationsHandler_InvalidPagination(t *testing.T) {
	svc := &service.ShipmentService{Store: &fakeShipmentSvcStore{}}
	h := ShipmentHandler{Service: svc}
	r := chi.NewRouter()
	r.Get("/v1/shipments/{id}/notifications", h.ListShipmentNotifications)
	req := httptest.NewRequest(http.MethodGet, "/v1/shipments/"+uuid.NewString()+"/notifications?limit=bad", nil)
	req = req.WithContext(middleware.WithAuthUser(req.Context(), service.AuthUser{ID: uuid.New(), Email: "a@example.com"}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateShipmentHandler_UsesAuthOwnerUserID(t *testing.T) {
	st := &fakeShipmentSvcStore{}
	svc := &service.ShipmentService{Store: st}
	h := ShipmentHandler{Service: svc}
	payload, _ := json.Marshal(map[string]any{
		"file_ids":   []string{uuid.NewString()},
		"subject":    "請求書",
		"share_mode": "url_shared",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/shipments", bytes.NewReader(payload))
	userID := uuid.New()
	req = req.WithContext(middleware.WithAuthUser(req.Context(), service.AuthUser{ID: userID, Email: "a@example.com"}))
	w := httptest.NewRecorder()
	h.CreateShipment(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if st.lastOwnerUserID == nil || *st.lastOwnerUserID != userID {
		t.Fatal("owner_user_id should be set from auth")
	}
}

func TestResendShipmentHandler_Statuses(t *testing.T) {
	ownerID := uuid.New()
	shipID := uuid.New()
	svc := &service.ShipmentService{Store: &fakeShipmentSvcStore{
		shipment: store.Shipment{ID: shipID, OwnerUserID: &ownerID, ShareMode: "recipient_restricted", Status: "sent", ExpiresAt: time.Now().UTC().Add(24 * time.Hour), Title: "件名"},
	}, Queue: &fakeQueue{}}
	h := ShipmentHandler{Service: svc}
	r := chi.NewRouter()
	r.Post("/v1/shipments/{id}/resend", h.ResendShipment)

	req := httptest.NewRequest(http.MethodPost, "/v1/shipments/"+shipID.String()+"/resend", bytes.NewBufferString(`{}`))
	req = req.WithContext(middleware.WithAuthUser(req.Context(), service.AuthUser{ID: ownerID, Email: "a@example.com"}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got=%d body=%s", w.Code, w.Body.String())
	}

	reqForbidden := httptest.NewRequest(http.MethodPost, "/v1/shipments/"+shipID.String()+"/resend", bytes.NewBufferString(`{}`))
	reqForbidden = reqForbidden.WithContext(middleware.WithAuthUser(reqForbidden.Context(), service.AuthUser{ID: uuid.New(), Email: "b@example.com"}))
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, reqForbidden)
	if w2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 got=%d body=%s", w2.Code, w2.Body.String())
	}

	svcNotFound := &service.ShipmentService{Store: &fakeShipmentSvcStore{shipmentErr: store.ErrNotFound}, Queue: &fakeQueue{}}
	hNotFound := ShipmentHandler{Service: svcNotFound}
	r2 := chi.NewRouter()
	r2.Post("/v1/shipments/{id}/resend", hNotFound.ResendShipment)
	reqNotFound := httptest.NewRequest(http.MethodPost, "/v1/shipments/"+shipID.String()+"/resend", bytes.NewBufferString(`{}`))
	reqNotFound = reqNotFound.WithContext(middleware.WithAuthUser(reqNotFound.Context(), service.AuthUser{ID: ownerID, Email: "a@example.com"}))
	w3 := httptest.NewRecorder()
	r2.ServeHTTP(w3, reqNotFound)
	if w3.Code != http.StatusNotFound {
		t.Fatalf("expected 404 got=%d body=%s", w3.Code, w3.Body.String())
	}

	svcConflict := &service.ShipmentService{Store: &fakeShipmentSvcStore{
		shipment: store.Shipment{ID: shipID, OwnerUserID: &ownerID, ShareMode: "url_shared", Status: "sent", ExpiresAt: time.Now().UTC().Add(24 * time.Hour), Title: "件名"},
	}, Queue: &fakeQueue{}}
	hConflict := ShipmentHandler{Service: svcConflict}
	r3 := chi.NewRouter()
	r3.Post("/v1/shipments/{id}/resend", hConflict.ResendShipment)
	reqConflict := httptest.NewRequest(http.MethodPost, "/v1/shipments/"+shipID.String()+"/resend", bytes.NewBufferString(`{}`))
	reqConflict = reqConflict.WithContext(middleware.WithAuthUser(reqConflict.Context(), service.AuthUser{ID: ownerID, Email: "a@example.com"}))
	w4 := httptest.NewRecorder()
	r3.ServeHTTP(w4, reqConflict)
	if w4.Code != http.StatusConflict {
		t.Fatalf("expected 409 got=%d body=%s", w4.Code, w4.Body.String())
	}
}
