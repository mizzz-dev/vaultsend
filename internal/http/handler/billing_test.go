package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/example/vaultsend/internal/http/middleware"
	"github.com/example/vaultsend/internal/service"
	"github.com/example/vaultsend/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type handlerBillingStore struct{}

func (h *handlerBillingStore) GetUserByID(ctx context.Context, id uuid.UUID) (store.User, error) {
	return store.User{ID: id, Email: "u@example.com"}, nil
}
func (h *handlerBillingStore) GetOrganizationMember(ctx context.Context, orgID uuid.UUID, userID uuid.UUID) (store.OrganizationMember, error) {
	return store.OrganizationMember{OrganizationID: orgID, UserID: userID, Role: "owner"}, nil
}
func (h *handlerBillingStore) GetLatestSubscriptionByUserID(ctx context.Context, userID uuid.UUID) (store.Subscription, error) {
	return store.Subscription{}, store.ErrNotFound
}
func (h *handlerBillingStore) GetLatestSubscriptionByOrgID(ctx context.Context, orgID uuid.UUID) (store.Subscription, error) {
	return store.Subscription{}, store.ErrNotFound
}
func (h *handlerBillingStore) UpsertSubscription(ctx context.Context, arg store.UpsertSubscriptionParams) (store.Subscription, error) {
	return store.Subscription{}, nil
}
func (h *handlerBillingStore) UpsertOrgSubscription(ctx context.Context, arg store.UpsertSubscriptionParams) (store.Subscription, error) {
	return store.Subscription{}, nil
}
func (h *handlerBillingStore) CountShipmentsByUserSince(ctx context.Context, ownerUserID uuid.UUID, since time.Time) (int64, error) {
	return 0, nil
}
func (h *handlerBillingStore) CountShipmentsByOrgSince(ctx context.Context, organizationID uuid.UUID, since time.Time) (int64, error) {
	return 0, nil
}
func (h *handlerBillingStore) SumStorageBytesByUser(ctx context.Context, ownerUserID uuid.UUID) (int64, error) {
	return 0, nil
}
func (h *handlerBillingStore) SumStorageBytesByOrg(ctx context.Context, organizationID uuid.UUID) (int64, error) {
	return 0, nil
}
func (h *handlerBillingStore) CountOrganizationMembers(ctx context.Context, orgID uuid.UUID) (int64, error) {
	return 1, nil
}

type handlerBillingStripe struct{}

func (s *handlerBillingStripe) CreateCheckoutSession(ctx context.Context, in service.CheckoutInput) (service.CheckoutSession, error) {
	return service.CheckoutSession{ID: "cs_test", URL: "https://example.com/checkout"}, nil
}
func (s *handlerBillingStripe) ParseSubscriptionWebhook(payload []byte, signature string) (service.WebhookSubscriptionEvent, error) {
	return service.WebhookSubscriptionEvent{}, nil
}
func (s *handlerBillingStripe) UpdateSubscriptionQuantity(ctx context.Context, subscriptionID string, quantity int64) error {
	return nil
}
func (s *handlerBillingStripe) ListInvoices(ctx context.Context, customerID string, limit int64, startingAfter string) (service.StripeInvoiceList, error) {
	now := time.Now().UTC()
	return service.StripeInvoiceList{
		Data: []service.StripeInvoice{{
			ID:         "in_123",
			CustomerID: customerID,
			AmountDue:  1200,
			Currency:   "jpy",
			Status:     "paid",
			CreatedAt:  now,
		}},
	}, nil
}
func (s *handlerBillingStripe) GetInvoice(ctx context.Context, invoiceID string) (service.StripeInvoice, error) {
	now := time.Now().UTC()
	return service.StripeInvoice{
		ID:            invoiceID,
		CustomerID:    "cus_123",
		AmountDue:     1200,
		Currency:      "jpy",
		Status:        "paid",
		CreatedAt:     now,
		LineItems:     []service.InvoiceLineItem{{ID: "il_123", Amount: 1200}},
		TaxAmount:     100,
		PaymentStatus: "succeeded",
	}, nil
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

func TestBillingPlan_OK(t *testing.T) {
	svc := &service.BillingService{Store: &handlerBillingStore{}, Stripe: &handlerBillingStripe{}, FrontendURL: "http://localhost:3000"}
	h := BillingHandler{Service: svc}
	req := httptest.NewRequest(http.MethodGet, "/v1/billing/plan", nil)
	uid := uuid.New()
	req = req.WithContext(middleware.WithAuthUser(req.Context(), service.AuthUser{ID: uid, Email: "u@example.com"}))
	w := httptest.NewRecorder()
	h.GetPlan(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestBillingGetOrgBilling_OK(t *testing.T) {
	svc := &service.BillingService{Store: &handlerBillingStore{}, Stripe: &handlerBillingStripe{}, FrontendURL: "http://localhost:3000"}
	h := BillingHandler{Service: svc}
	orgID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/orgs/"+orgID.String()+"/billing", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", orgID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(middleware.WithAuthUser(req.Context(), service.AuthUser{ID: uuid.New(), Email: "u@example.com"}))
	w := httptest.NewRecorder()
	h.GetOrgBilling(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestBillingListOrgInvoices_OK(t *testing.T) {
	customerID := "cus_123"
	svc := &service.BillingService{
		Store:  &handlerBillingStoreWithCustomer{customerID: customerID},
		Stripe: &handlerBillingStripe{},
	}
	h := BillingHandler{Service: svc}
	orgID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/orgs/"+orgID.String()+"/invoices?limit=10", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", orgID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(middleware.WithAuthUser(req.Context(), service.AuthUser{ID: uuid.New(), Email: "u@example.com"}))
	w := httptest.NewRecorder()
	h.ListOrgInvoices(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestBillingGetOrgInvoice_OK(t *testing.T) {
	customerID := "cus_123"
	svc := &service.BillingService{
		Store:  &handlerBillingStoreWithCustomer{customerID: customerID},
		Stripe: &handlerBillingStripe{},
	}
	h := BillingHandler{Service: svc}
	orgID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/orgs/"+orgID.String()+"/invoices/in_123", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", orgID.String())
	rctx.URLParams.Add("invoice_id", "in_123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(middleware.WithAuthUser(req.Context(), service.AuthUser{ID: uuid.New(), Email: "u@example.com"}))
	w := httptest.NewRecorder()
	h.GetOrgInvoice(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

type handlerBillingStoreWithCustomer struct {
	handlerBillingStore
	customerID string
}

func (h *handlerBillingStoreWithCustomer) GetLatestSubscriptionByOrgID(ctx context.Context, orgID uuid.UUID) (store.Subscription, error) {
	return store.Subscription{OrganizationID: &orgID, StripeCustomerID: &h.customerID}, nil
}

func TestBillingListOrgInvoices_InvalidLimit(t *testing.T) {
	h := BillingHandler{Service: &service.BillingService{}}
	orgID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/orgs/"+orgID.String()+"/invoices?limit=0", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", orgID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(middleware.WithAuthUser(req.Context(), service.AuthUser{ID: uuid.New(), Email: "u@example.com"}))
	w := httptest.NewRecorder()
	h.ListOrgInvoices(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_limit") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
