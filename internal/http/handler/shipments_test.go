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
	"github.com/example/vaultsend/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type fakeShipmentSvcStore struct {
	lastOwnerUserID *uuid.UUID
}

func (f *fakeShipmentSvcStore) GetShipment(ctx context.Context, id uuid.UUID) (store.Shipment, error) {
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
func (f *fakeShipmentSvcStore) ListShipmentsByUser(ctx context.Context, ownerUserID uuid.UUID, limit int32, offset int32) ([]store.ShipmentListItem, error) {
	return []store.ShipmentListItem{{ID: uuid.New(), Title: "件名", ShareMode: "url_shared", Status: "sent", MaxDownloads: 10, FileCount: 1}}, nil
}
func (f *fakeShipmentSvcStore) CountShipmentsByUser(ctx context.Context, ownerUserID uuid.UUID) (int64, error) {
	return 1, nil
}
func (f *fakeShipmentSvcStore) GetRecipientDownloadStatsByShipment(ctx context.Context, shipmentID uuid.UUID) ([]store.RecipientDownloadStat, error) {
	return nil, nil
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
