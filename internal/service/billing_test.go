package service

import (
	"context"
	"testing"
	"time"

	"github.com/example/vaultsend/internal/store"
	"github.com/google/uuid"
)

type fakeBillingStore struct {
	user             store.User
	sub              store.Subscription
	orgSub           store.Subscription
	subNotFound      bool
	orgSubNotFound   bool
	shipmentCount    int64
	storageBytes     int64
	orgShipmentCount int64
	orgStorageBytes  int64
	memberCount      int64
	memberRole       string
	upserted         store.UpsertSubscriptionParams
	upsertedOrg      store.UpsertSubscriptionParams
}

func (f *fakeBillingStore) GetUserByID(ctx context.Context, id uuid.UUID) (store.User, error) {
	return f.user, nil
}
func (f *fakeBillingStore) GetOrganizationMember(ctx context.Context, orgID uuid.UUID, userID uuid.UUID) (store.OrganizationMember, error) {
	if f.memberRole == "" {
		return store.OrganizationMember{}, store.ErrNotFound
	}
	return store.OrganizationMember{OrganizationID: orgID, UserID: userID, Role: f.memberRole}, nil
}
func (f *fakeBillingStore) GetLatestSubscriptionByUserID(ctx context.Context, userID uuid.UUID) (store.Subscription, error) {
	if f.subNotFound {
		return store.Subscription{}, store.ErrNotFound
	}
	return f.sub, nil
}
func (f *fakeBillingStore) GetLatestSubscriptionByOrgID(ctx context.Context, orgID uuid.UUID) (store.Subscription, error) {
	if f.orgSubNotFound {
		return store.Subscription{}, store.ErrNotFound
	}
	return f.orgSub, nil
}
func (f *fakeBillingStore) UpsertSubscription(ctx context.Context, arg store.UpsertSubscriptionParams) (store.Subscription, error) {
	f.upserted = arg
	return store.Subscription{UserID: arg.UserID, Plan: arg.Plan, Status: arg.Status}, nil
}
func (f *fakeBillingStore) UpsertOrgSubscription(ctx context.Context, arg store.UpsertSubscriptionParams) (store.Subscription, error) {
	f.upsertedOrg = arg
	return store.Subscription{OrganizationID: arg.OrganizationID, Plan: arg.Plan, Status: arg.Status}, nil
}
func (f *fakeBillingStore) CountShipmentsByUserSince(ctx context.Context, ownerUserID uuid.UUID, since time.Time) (int64, error) {
	return f.shipmentCount, nil
}
func (f *fakeBillingStore) CountShipmentsByOrgSince(ctx context.Context, organizationID uuid.UUID, since time.Time) (int64, error) {
	return f.orgShipmentCount, nil
}
func (f *fakeBillingStore) SumStorageBytesByUser(ctx context.Context, ownerUserID uuid.UUID) (int64, error) {
	return f.storageBytes, nil
}
func (f *fakeBillingStore) SumStorageBytesByOrg(ctx context.Context, organizationID uuid.UUID) (int64, error) {
	return f.orgStorageBytes, nil
}
func (f *fakeBillingStore) CountOrganizationMembers(ctx context.Context, orgID uuid.UUID) (int64, error) {
	return f.memberCount, nil
}

type fakeStripeGateway struct {
	checkout         CheckoutSession
	event            WebhookSubscriptionEvent
	updatedSubID     string
	updatedQuantity  int64
	updateShouldFail bool
	invoices         StripeInvoiceList
	invoice          StripeInvoice
}

func (f *fakeStripeGateway) CreateCheckoutSession(ctx context.Context, in CheckoutInput) (CheckoutSession, error) {
	return f.checkout, nil
}
func (f *fakeStripeGateway) ParseSubscriptionWebhook(payload []byte, signature string) (WebhookSubscriptionEvent, error) {
	return f.event, nil
}
func (f *fakeStripeGateway) UpdateSubscriptionQuantity(ctx context.Context, subscriptionID string, quantity int64) error {
	if f.updateShouldFail {
		return context.DeadlineExceeded
	}
	f.updatedSubID = subscriptionID
	f.updatedQuantity = quantity
	return nil
}
func (f *fakeStripeGateway) ListInvoices(ctx context.Context, customerID string, limit int64, startingAfter string) (StripeInvoiceList, error) {
	return f.invoices, nil
}
func (f *fakeStripeGateway) GetInvoice(ctx context.Context, invoiceID string) (StripeInvoice, error) {
	return f.invoice, nil
}

func TestBilling_EnforceUploadLimit_FreeVsPro(t *testing.T) {
	userID := uuid.New()
	svc := &BillingService{Store: &fakeBillingStore{subNotFound: true}}
	if err := svc.EnforceUploadLimit(context.Background(), &userID, nil, 2*1024*1024*1024); err == nil {
		t.Fatal("free should reject >1GB")
	}
	pro := &BillingService{Store: &fakeBillingStore{sub: store.Subscription{Plan: PlanPro, Status: "active"}}}
	if err := pro.EnforceUploadLimit(context.Background(), &userID, nil, 2*1024*1024*1024); err != nil {
		t.Fatalf("pro should allow: %v", err)
	}
}

func TestBilling_GetPlan_OrgPreferred(t *testing.T) {
	userID := uuid.New()
	orgID := uuid.New()
	svc := &BillingService{Store: &fakeBillingStore{
		sub:    store.Subscription{Plan: PlanFree, Status: "active"},
		orgSub: store.Subscription{Plan: PlanPro, Status: "active"},
	}}
	plan, err := svc.GetPlan(context.Background(), &userID, &orgID)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if plan.Name != PlanPro {
		t.Fatalf("expected org plan to win: %+v", plan)
	}
}

func TestBilling_HandleWebhook_UserUpsert(t *testing.T) {
	userID := uuid.New()
	st := &fakeBillingStore{}
	svc := &BillingService{Store: st, Stripe: &fakeStripeGateway{event: WebhookSubscriptionEvent{
		Type:                 "customer.subscription.created",
		StripeSubscriptionID: "sub_123",
		StripeCustomerID:     "cus_123",
		SeatCount:            3,
		Status:               "active",
		Metadata:             map[string]string{"user_id": userID.String(), "plan": "pro"},
	}}}
	if err := svc.HandleWebhook(context.Background(), []byte("{}"), "sig"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if st.upserted.UserID == nil || *st.upserted.UserID != userID || st.upserted.Plan != PlanPro {
		t.Fatalf("unexpected user upsert: %+v", st.upserted)
	}
	if st.upserted.SeatCount != 3 {
		t.Fatalf("unexpected seat count: %+v", st.upserted)
	}
}

func TestBilling_HandleWebhook_OrgUpsert(t *testing.T) {
	orgID := uuid.New()
	st := &fakeBillingStore{}
	svc := &BillingService{Store: st, Stripe: &fakeStripeGateway{event: WebhookSubscriptionEvent{
		Type:                 "customer.subscription.updated",
		StripeSubscriptionID: "sub_org_123",
		StripeCustomerID:     "cus_org_123",
		SeatCount:            7,
		Status:               "active",
		Metadata:             map[string]string{"organization_id": orgID.String(), "plan": "pro"},
	}}}
	if err := svc.HandleWebhook(context.Background(), []byte("{}"), "sig"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if st.upsertedOrg.OrganizationID == nil || *st.upsertedOrg.OrganizationID != orgID {
		t.Fatalf("unexpected org upsert: %+v", st.upsertedOrg)
	}
	if st.upsertedOrg.SeatCount != 7 {
		t.Fatalf("unexpected org seat count: %+v", st.upsertedOrg)
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

func TestBilling_SyncSeatCountWithStripe(t *testing.T) {
	orgID := uuid.New()
	st := &fakeBillingStore{
		orgSub:      store.Subscription{OrganizationID: &orgID, StripeSubscriptionID: "sub_org_123", Plan: PlanPro, Status: "active"},
		memberCount: 4,
	}
	sg := &fakeStripeGateway{}
	svc := &BillingService{Store: st, Stripe: sg}
	if err := svc.SyncSeatCountWithStripe(context.Background(), orgID); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if sg.updatedSubID != "sub_org_123" || sg.updatedQuantity != 4 {
		t.Fatalf("unexpected stripe call: sub=%s qty=%d", sg.updatedSubID, sg.updatedQuantity)
	}
	if st.upsertedOrg.SeatCount != 4 {
		t.Fatalf("unexpected cached seat count: %+v", st.upsertedOrg)
	}
}

func TestBilling_GetOrganizationBilling_OwnerOnly(t *testing.T) {
	orgID := uuid.New()
	ownerID := uuid.New()
	memberID := uuid.New()
	svcOwner := &BillingService{Store: &fakeBillingStore{
		memberRole:       "owner",
		orgSub:           store.Subscription{Plan: PlanPro, Status: "active"},
		orgShipmentCount: 3,
		orgStorageBytes:  1024,
		memberCount:      5,
	}}
	out, err := svcOwner.GetOrganizationBilling(context.Background(), ownerID, orgID)
	if err != nil {
		t.Fatalf("owner should pass: %v", err)
	}
	if out.Plan != PlanPro || out.MembersCount != 5 {
		t.Fatalf("unexpected output: %+v", out)
	}

	svcMember := &BillingService{Store: &fakeBillingStore{memberRole: "member"}}
	if _, err := svcMember.GetOrganizationBilling(context.Background(), memberID, orgID); err == nil {
		t.Fatal("member should be forbidden")
	}
}

func TestBilling_GetPlanDetails(t *testing.T) {
	userID := uuid.New()
	svc := &BillingService{Store: &fakeBillingStore{subNotFound: true, shipmentCount: 12, storageBytes: 2048}}
	out, err := svc.GetPlanDetails(context.Background(), &userID, nil)
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
	err := svc.EnforceUploadLimit(context.Background(), &userID, nil, 2*1024*1024*1024)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("unexpected err: %v", err)
	}
	if apiErr.Error != PlanLimitErrorType || !apiErr.UpgradeRequired || apiErr.RecommendedPlan != RecommendedPlanPro {
		t.Fatalf("unexpected api error: %+v", apiErr)
	}
}

func TestBilling_ListInvoices_OK(t *testing.T) {
	orgID := uuid.New()
	actorID := uuid.New()
	customerID := "cus_123"
	createdAt := time.Now().UTC().Add(-time.Hour)
	paidAt := time.Now().UTC()
	svc := &BillingService{
		Store: &fakeBillingStore{
			memberRole: "admin",
			orgSub: store.Subscription{
				OrganizationID:   &orgID,
				StripeCustomerID: &customerID,
			},
		},
		Stripe: &fakeStripeGateway{
			invoices: StripeInvoiceList{
				Data: []StripeInvoice{{
					ID:               "in_123",
					AmountDue:        1200,
					Currency:         "jpy",
					Status:           "paid",
					HostedInvoiceURL: "https://example.com/invoice",
					InvoicePDF:       "https://example.com/invoice.pdf",
					CreatedAt:        createdAt,
					PaidAt:           &paidAt,
				}},
				HasMore: true,
			},
		},
	}
	out, err := svc.ListInvoices(context.Background(), actorID, orgID, 20, "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out.Invoices) != 1 || out.Invoices[0].InvoiceID != "in_123" {
		t.Fatalf("unexpected invoices: %+v", out)
	}
	if !out.HasMore || out.NextStartingAfter != "in_123" {
		t.Fatalf("unexpected pagination: %+v", out)
	}
}

func TestBilling_GetInvoice_OK(t *testing.T) {
	orgID := uuid.New()
	actorID := uuid.New()
	customerID := "cus_123"
	now := time.Now().UTC()
	svc := &BillingService{
		Store: &fakeBillingStore{
			memberRole: "owner",
			orgSub: store.Subscription{
				OrganizationID:   &orgID,
				StripeCustomerID: &customerID,
			},
		},
		Stripe: &fakeStripeGateway{
			invoice: StripeInvoice{
				ID:               "in_123",
				CustomerID:       customerID,
				AmountDue:        1500,
				Currency:         "jpy",
				Status:           "open",
				PaymentStatus:    "requires_payment_method",
				TaxAmount:        100,
				CreatedAt:        now,
				LineItems:        []InvoiceLineItem{{ID: "il_123", Amount: 1500}},
				PaymentMethod:    "pm_123",
				HostedInvoiceURL: "https://example.com/invoice",
				InvoicePDF:       "https://example.com/invoice.pdf",
			},
		},
	}
	out, err := svc.GetInvoice(context.Background(), actorID, orgID, "in_123")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.InvoiceID != "in_123" || out.Tax.Amount != 100 {
		t.Fatalf("unexpected detail: %+v", out)
	}
}

func TestBilling_ListInvoices_ForbiddenForMember(t *testing.T) {
	orgID := uuid.New()
	actorID := uuid.New()
	customerID := "cus_123"
	svc := &BillingService{
		Store: &fakeBillingStore{
			memberRole: "member",
			orgSub:     store.Subscription{OrganizationID: &orgID, StripeCustomerID: &customerID},
		},
		Stripe: &fakeStripeGateway{},
	}
	if _, err := svc.ListInvoices(context.Background(), actorID, orgID, 20, ""); err == nil {
		t.Fatal("expected forbidden error")
	}
}
