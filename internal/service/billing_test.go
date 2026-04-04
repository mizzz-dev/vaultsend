package service

import (
	"context"
	"testing"
	"time"

	"github.com/example/vaultsend/internal/store"
	"github.com/google/uuid"
)

type fakeBillingStore struct {
	user          store.User
	sub           store.Subscription
	subNotFound   bool
	shipmentCount int64
	storageBytes  int64
	upserted      store.UpsertSubscriptionParams
}

func (f *fakeBillingStore) GetUserByID(ctx context.Context, id uuid.UUID) (store.User, error) {
	return f.user, nil
}
func (f *fakeBillingStore) GetLatestSubscriptionByUserID(ctx context.Context, userID uuid.UUID) (store.Subscription, error) {
	if f.subNotFound {
		return store.Subscription{}, store.ErrNotFound
	}
	return f.sub, nil
}
func (f *fakeBillingStore) UpsertSubscription(ctx context.Context, arg store.UpsertSubscriptionParams) (store.Subscription, error) {
	f.upserted = arg
	return store.Subscription{UserID: arg.UserID, Plan: arg.Plan, Status: arg.Status}, nil
}
func (f *fakeBillingStore) CountShipmentsByUserSince(ctx context.Context, ownerUserID uuid.UUID, since time.Time) (int64, error) {
	return f.shipmentCount, nil
}
func (f *fakeBillingStore) SumStorageBytesByUser(ctx context.Context, ownerUserID uuid.UUID) (int64, error) {
	return f.storageBytes, nil
}

type fakeStripeGateway struct {
	checkout CheckoutSession
	event    WebhookSubscriptionEvent
}

func (f *fakeStripeGateway) CreateCheckoutSession(ctx context.Context, in CheckoutInput) (CheckoutSession, error) {
	return f.checkout, nil
}
func (f *fakeStripeGateway) ParseSubscriptionWebhook(payload []byte, signature string) (WebhookSubscriptionEvent, error) {
	return f.event, nil
}

func TestBilling_EnforceUploadLimit_FreeVsPro(t *testing.T) {
	userID := uuid.New()
	svc := &BillingService{Store: &fakeBillingStore{subNotFound: true}}
	if err := svc.EnforceUploadLimit(context.Background(), &userID, 2*1024*1024*1024); err == nil {
		t.Fatal("free should reject >1GB")
	}
	pro := &BillingService{Store: &fakeBillingStore{sub: store.Subscription{Plan: PlanPro, Status: "active"}}}
	if err := pro.EnforceUploadLimit(context.Background(), &userID, 2*1024*1024*1024); err != nil {
		t.Fatalf("pro should allow: %v", err)
	}
}

func TestBilling_HandleWebhook_Upsert(t *testing.T) {
	userID := uuid.New()
	st := &fakeBillingStore{}
	svc := &BillingService{Store: st, Stripe: &fakeStripeGateway{event: WebhookSubscriptionEvent{
		Type:                 "customer.subscription.created",
		StripeSubscriptionID: "sub_123",
		StripeCustomerID:     "cus_123",
		Status:               "active",
		Metadata:             map[string]string{"user_id": userID.String(), "plan": "pro"},
	}}}
	if err := svc.HandleWebhook(context.Background(), []byte("{}"), "sig"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if st.upserted.UserID != userID || st.upserted.Plan != PlanPro {
		t.Fatalf("unexpected upsert: %+v", st.upserted)
	}
}

func TestBilling_CreateCheckout(t *testing.T) {
	userID := uuid.New()
	svc := &BillingService{
		Store:       &fakeBillingStore{user: store.User{ID: userID, Email: "u@example.com"}},
		Stripe:      &fakeStripeGateway{checkout: CheckoutSession{ID: "cs_123", URL: "https://checkout"}},
		FrontendURL: "http://localhost:3000",
	}
	out, err := svc.CreateCheckout(context.Background(), userID)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.SessionID == "" || out.URL == "" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestBilling_GetPlanDetails(t *testing.T) {
	userID := uuid.New()
	svc := &BillingService{Store: &fakeBillingStore{subNotFound: true, shipmentCount: 12, storageBytes: 2048}}
	out, err := svc.GetPlanDetails(context.Background(), &userID)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.Plan != PlanFree || out.Usage.CurrentMonthShipments != 12 {
		t.Fatalf("unexpected plan details: %+v", out)
	}
	if out.Remaining.RemainingShipments == nil || *out.Remaining.RemainingShipments != 38 {
		t.Fatalf("unexpected remaining: %+v", out.Remaining)
	}
}

func TestBilling_PlanLimitErrorFormat(t *testing.T) {
	userID := uuid.New()
	svc := &BillingService{Store: &fakeBillingStore{subNotFound: true}}
	err := svc.EnforceUploadLimit(context.Background(), &userID, 2*1024*1024*1024)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("unexpected err: %v", err)
	}
	if apiErr.Error != PlanLimitErrorType || !apiErr.UpgradeRequired || apiErr.RecommendedPlan != RecommendedPlanPro {
		t.Fatalf("unexpected api error: %+v", apiErr)
	}
}
