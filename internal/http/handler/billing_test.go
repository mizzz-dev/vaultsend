package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/vaultsend/internal/http/middleware"
	"github.com/example/vaultsend/internal/service"
	"github.com/example/vaultsend/internal/store"
	"github.com/google/uuid"
)

type handlerBillingStore struct{}

func (h *handlerBillingStore) GetUserByID(ctx context.Context, id uuid.UUID) (store.User, error) {
	return store.User{ID: id, Email: "u@example.com"}, nil
}
func (h *handlerBillingStore) GetLatestSubscriptionByUserID(ctx context.Context, userID uuid.UUID) (store.Subscription, error) {
	return store.Subscription{}, store.ErrNotFound
}
func (h *handlerBillingStore) UpsertSubscription(ctx context.Context, arg store.UpsertSubscriptionParams) (store.Subscription, error) {
	return store.Subscription{}, nil
}
func (h *handlerBillingStore) CountShipmentsByUserSince(ctx context.Context, ownerUserID uuid.UUID, since time.Time) (int64, error) {
	return 0, nil
}

type handlerBillingStripe struct{}

func (s *handlerBillingStripe) CreateCheckoutSession(ctx context.Context, in service.CheckoutInput) (service.CheckoutSession, error) {
	return service.CheckoutSession{ID: "cs_test", URL: "https://example.com/checkout"}, nil
}
func (s *handlerBillingStripe) ParseSubscriptionWebhook(payload []byte, signature string) (service.WebhookSubscriptionEvent, error) {
	return service.WebhookSubscriptionEvent{}, nil
}

func TestBillingCheckout_Unauthorized(t *testing.T) {
	h := BillingHandler{Service: &service.BillingService{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/billing/checkout", nil)
	w := httptest.NewRecorder()
	h.CreateCheckout(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestBillingWebhook_OK(t *testing.T) {
	h := BillingHandler{Service: &service.BillingService{Store: &handlerBillingStore{}, Stripe: &handlerBillingStripe{}}}
	req := httptest.NewRequest(http.MethodPost, "/v1/billing/webhook", bytes.NewBufferString("{}"))
	w := httptest.NewRecorder()
	h.Webhook(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestBillingCheckout_Authorized(t *testing.T) {
	svc := &service.BillingService{Store: &handlerBillingStore{}, Stripe: &handlerBillingStripe{}, FrontendURL: "http://localhost:3000"}
	h := BillingHandler{Service: svc}
	req := httptest.NewRequest(http.MethodPost, "/v1/billing/checkout", nil)
	uid := uuid.New()
	req = req.WithContext(middleware.WithAuthUser(req.Context(), service.AuthUser{ID: uid, Email: "u@example.com"}))
	w := httptest.NewRecorder()
	h.CreateCheckout(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
