package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/vaultsend/internal/service"
	"github.com/example/vaultsend/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type fakeShipmentSvcStore struct{}

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
	return store.ShipmentFinalizeResult{Shipment: store.Shipment{ID: arg.ShipmentID, Status: "sent", ExpiresAt: arg.ExpiresAt, MaxDownloads: arg.MaxDownloads}}, nil
}
func (f *fakeShipmentSvcStore) GetFilesByShipmentID(ctx context.Context, shipmentID uuid.UUID) ([]store.File, error) {
	return []store.File{{ID: uuid.New(), OriginalName: "a.txt", SizeBytes: 1}}, nil
}
func (f *fakeShipmentSvcStore) GetRecipientsByShipmentID(ctx context.Context, shipmentID uuid.UUID) ([]store.Recipient, error) {
	return []store.Recipient{{ID: uuid.New(), Email: "a@example.com", Status: "pending"}}, nil
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
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
