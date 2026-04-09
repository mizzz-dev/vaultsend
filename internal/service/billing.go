package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/example/vaultsend/internal/store"
	"github.com/google/uuid"
)

const (
	PlanFree = "free"
	PlanPro  = "pro"
)

const (
	PlanLimitErrorType = "plan_limit_exceeded"
	RecommendedPlanPro = "pro"
)

type PlanLimits struct {
	MaxFileSizeBytes     int64 `json:"max_file_size"`
	MaxRetentionDays     int   `json:"max_storage_days"`
	MonthlyShipmentLimit int64 `json:"monthly_shipment_limit"`
}

type PlanUsage struct {
	CurrentMonthShipments int64 `json:"current_month_shipments"`
	CurrentStorageBytes   int64 `json:"current_storage_bytes"`
}

type RemainingQuota struct {
	RemainingShipments *int64 `json:"remaining_shipments,omitempty"`
}

type UserPlan struct {
	Name   string     `json:"name"`
	Status string     `json:"status"`
	Limits PlanLimits `json:"limits"`
}

type PlanDetails struct {
	Plan      string         `json:"plan"`
	Status    string         `json:"status"`
	Limits    PlanLimits     `json:"limits"`
	Usage     PlanUsage      `json:"usage"`
	Remaining RemainingQuota `json:"remaining"`
}

type BillingStore interface {
	GetUserByID(ctx context.Context, id uuid.UUID) (store.User, error)
	GetOrganizationMember(ctx context.Context, orgID uuid.UUID, userID uuid.UUID) (store.OrganizationMember, error)
	GetLatestSubscriptionByUserID(ctx context.Context, userID uuid.UUID) (store.Subscription, error)
	GetLatestSubscriptionByOrgID(ctx context.Context, orgID uuid.UUID) (store.Subscription, error)
	UpsertSubscription(ctx context.Context, arg store.UpsertSubscriptionParams) (store.Subscription, error)
	UpsertOrgSubscription(ctx context.Context, arg store.UpsertSubscriptionParams) (store.Subscription, error)
	CountShipmentsByUserSince(ctx context.Context, ownerUserID uuid.UUID, since time.Time) (int64, error)
	CountShipmentsByOrgSince(ctx context.Context, organizationID uuid.UUID, since time.Time) (int64, error)
	SumStorageBytesByUser(ctx context.Context, ownerUserID uuid.UUID) (int64, error)
	SumStorageBytesByOrg(ctx context.Context, organizationID uuid.UUID) (int64, error)
	CountOrganizationMembers(ctx context.Context, orgID uuid.UUID) (int64, error)
}

type CheckoutSession struct {
	ID  string
	URL string
}

type CheckoutInput struct {
	UserID         uuid.UUID
	UserEmail      string
	OrganizationID *uuid.UUID
	SuccessURL     string
	CancelURL      string
}

type CheckoutOutput struct {
	SessionID string `json:"session_id"`
	URL       string `json:"url"`
}

type StripeGateway interface {
	CreateCheckoutSession(ctx context.Context, in CheckoutInput) (CheckoutSession, error)
	ParseSubscriptionWebhook(payload []byte, signature string) (WebhookSubscriptionEvent, error)
	UpdateSubscriptionQuantity(ctx context.Context, subscriptionID string, quantity int64) error
	ListInvoices(ctx context.Context, customerID string, limit int64, startingAfter string) (StripeInvoiceList, error)
	GetInvoice(ctx context.Context, invoiceID string) (StripeInvoice, error)
}

type WebhookSubscriptionEvent struct {
	Type                 string
	StripeSubscriptionID string
	StripeCustomerID     string
	SeatCount            int64
	Status               string
	CurrentPeriodEnd     *time.Time
	Metadata             map[string]string
}

type BillingService struct {
	Store       BillingStore
	Stripe      StripeGateway
	FrontendURL string
}

type StripeInvoiceList struct {
	Data    []StripeInvoice
	HasMore bool
}

type StripeInvoice struct {
	ID               string
	CustomerID       string
	AmountDue        int64
	Currency         string
	Status           string
	HostedInvoiceURL string
	InvoicePDF       string
	CreatedAt        time.Time
	PaidAt           *time.Time
	TaxAmount        int64
	PaymentStatus    string
	PaymentMethod    string
	LineItems        []InvoiceLineItem
}

type InvoiceLineItem struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	Amount      int64      `json:"amount"`
	Currency    string     `json:"currency"`
	Quantity    int64      `json:"quantity"`
	PeriodStart *time.Time `json:"period_start,omitempty"`
	PeriodEnd   *time.Time `json:"period_end,omitempty"`
}

type ListInvoicesOutput struct {
	Invoices          []InvoiceSummary `json:"invoices"`
	HasMore           bool             `json:"has_more"`
	NextStartingAfter string           `json:"next_starting_after,omitempty"`
}

type InvoiceSummary struct {
	InvoiceID        string     `json:"invoice_id"`
	Amount           int64      `json:"amount"`
	Currency         string     `json:"currency"`
	Status           string     `json:"status"`
	HostedInvoiceURL string     `json:"hosted_invoice_url,omitempty"`
	InvoicePDF       string     `json:"invoice_pdf,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	PaidAt           *time.Time `json:"paid_at,omitempty"`
}

type InvoiceDetail struct {
	InvoiceID        string            `json:"invoice_id"`
	Amount           int64             `json:"amount"`
	Currency         string            `json:"currency"`
	Status           string            `json:"status"`
	PaymentStatus    string            `json:"payment_status"`
	PaymentMethod    string            `json:"payment_method,omitempty"`
	Tax              InvoiceTaxSummary `json:"tax"`
	HostedInvoiceURL string            `json:"hosted_invoice_url,omitempty"`
	InvoicePDF       string            `json:"invoice_pdf,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	PaidAt           *time.Time        `json:"paid_at,omitempty"`
	LineItems        []InvoiceLineItem `json:"line_items"`
}

type InvoiceTaxSummary struct {
	Amount int64 `json:"amount"`
}

func (s *BillingService) CreateCheckout(ctx context.Context, userID uuid.UUID) (CheckoutOutput, error) {
	user, err := s.Store.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return CheckoutOutput{}, &APIError{Status: 404, Code: "user_not_found", Message: "user が見つかりません"}
		}
		return CheckoutOutput{}, fmt.Errorf("get user: %w", err)
	}
	if s.Stripe == nil {
		return CheckoutOutput{}, &APIError{Status: 503, Code: "billing_unavailable", Message: "billing が利用できません"}
	}
	base := strings.TrimRight(s.FrontendURL, "/")
	if base == "" {
		base = "http://localhost:3000"
	}
	checkout, err := s.Stripe.CreateCheckoutSession(ctx, CheckoutInput{
		UserID:     userID,
		UserEmail:  user.Email,
		SuccessURL: base + "/settings/billing?checkout=success",
		CancelURL:  base + "/settings/billing?checkout=cancel",
	})
	if err != nil {
		return CheckoutOutput{}, fmt.Errorf("create stripe checkout: %w", err)
	}
	return CheckoutOutput{SessionID: checkout.ID, URL: checkout.URL}, nil
}

func (s *BillingService) CreateCheckoutForOrganization(ctx context.Context, actorID uuid.UUID, organizationID *uuid.UUID) (CheckoutOutput, error) {
	if organizationID == nil {
		return s.CreateCheckout(ctx, actorID)
	}
	member, err := s.Store.GetOrganizationMember(ctx, *organizationID, actorID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return CheckoutOutput{}, &APIError{Status: 403, Code: "forbidden", Message: "organization へのアクセス権がありません"}
		}
		return CheckoutOutput{}, fmt.Errorf("get organization member: %w", err)
	}
	if member.Role != "owner" {
		return CheckoutOutput{}, &APIError{Status: 403, Code: "forbidden", Message: "organization billing は owner のみ操作できます"}
	}
	user, err := s.Store.GetUserByID(ctx, actorID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return CheckoutOutput{}, &APIError{Status: 404, Code: "user_not_found", Message: "user が見つかりません"}
		}
		return CheckoutOutput{}, fmt.Errorf("get user: %w", err)
	}
	base := strings.TrimRight(s.FrontendURL, "/")
	if base == "" {
		base = "http://localhost:3000"
	}
	checkout, err := s.Stripe.CreateCheckoutSession(ctx, CheckoutInput{
		UserID:         actorID,
		UserEmail:      user.Email,
		OrganizationID: organizationID,
		SuccessURL:     base + "/settings/billing?checkout=success",
		CancelURL:      base + "/settings/billing?checkout=cancel",
	})
	if err != nil {
		return CheckoutOutput{}, fmt.Errorf("create stripe checkout: %w", err)
	}
	return CheckoutOutput{SessionID: checkout.ID, URL: checkout.URL}, nil
}

func (s *BillingService) HandleWebhook(ctx context.Context, payload []byte, signature string) error {
	if s.Stripe == nil {
		return &APIError{Status: 503, Code: "billing_unavailable", Message: "billing が利用できません"}
	}
	evt, err := s.Stripe.ParseSubscriptionWebhook(payload, signature)
	if err != nil {
		return &APIError{Status: 400, Code: "invalid_webhook", Message: "webhook 検証に失敗しました"}
	}
	if evt.StripeSubscriptionID == "" {
		return nil
	}
	orgIDRaw := strings.TrimSpace(evt.Metadata["organization_id"])
	userIDRaw := strings.TrimSpace(evt.Metadata["user_id"])
	plan := PlanFree
	if strings.EqualFold(strings.TrimSpace(evt.Metadata["plan"]), PlanPro) {
		plan = PlanPro
	}
	status := strings.TrimSpace(evt.Status)
	if status == "" {
		status = "active"
	}
	if evt.Type == "customer.subscription.deleted" {
		status = "canceled"
	}
	seatCount := evt.SeatCount
	if seatCount < 1 {
		seatCount = 1
	}
	var upsertErr error
	if orgIDRaw != "" {
		orgID, parseErr := uuid.Parse(orgIDRaw)
		if parseErr != nil {
			return nil
		}
		_, upsertErr = s.Store.UpsertOrgSubscription(ctx, store.UpsertSubscriptionParams{
			OrganizationID:       &orgID,
			StripeCustomerID:     ptrOrNil(evt.StripeCustomerID),
			StripeSubscriptionID: evt.StripeSubscriptionID,
			SeatCount:            seatCount,
			Plan:                 plan,
			Status:               status,
			CurrentPeriodEnd:     evt.CurrentPeriodEnd,
		})
	} else {
		if userIDRaw == "" {
			return nil
		}
		userID, parseErr := uuid.Parse(userIDRaw)
		if parseErr != nil {
			return nil
		}
		_, upsertErr = s.Store.UpsertSubscription(ctx, store.UpsertSubscriptionParams{
			UserID:               &userID,
			StripeCustomerID:     ptrOrNil(evt.StripeCustomerID),
			StripeSubscriptionID: evt.StripeSubscriptionID,
			SeatCount:            seatCount,
			Plan:                 plan,
			Status:               status,
			CurrentPeriodEnd:     evt.CurrentPeriodEnd,
		})
	}
	if upsertErr != nil {
		return fmt.Errorf("upsert subscription: %w", upsertErr)
	}
	return nil
}

func (s *BillingService) GetPlan(ctx context.Context, userID, orgID *uuid.UUID) (UserPlan, error) {
	if orgID != nil {
		sub, err := s.Store.GetLatestSubscriptionByOrgID(ctx, *orgID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return UserPlan{Name: PlanFree, Status: "inactive", Limits: limitsForPlan(PlanFree)}, nil
			}
			return UserPlan{}, fmt.Errorf("get latest organization subscription: %w", err)
		}
		if sub.Plan == PlanPro && isPaidActive(sub.Status) {
			return UserPlan{Name: PlanPro, Status: sub.Status, Limits: limitsForPlan(PlanPro)}, nil
		}
		return UserPlan{Name: PlanFree, Status: sub.Status, Limits: limitsForPlan(PlanFree)}, nil
	}
	if userID == nil {
		return UserPlan{Name: PlanFree, Status: "inactive", Limits: limitsForPlan(PlanFree)}, nil
	}
	sub, err := s.Store.GetLatestSubscriptionByUserID(ctx, *userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return UserPlan{Name: PlanFree, Status: "inactive", Limits: limitsForPlan(PlanFree)}, nil
		}
		return UserPlan{}, fmt.Errorf("get latest subscription: %w", err)
	}
	if sub.Plan == PlanPro && isPaidActive(sub.Status) {
		return UserPlan{Name: PlanPro, Status: sub.Status, Limits: limitsForPlan(PlanPro)}, nil
	}
	return UserPlan{Name: PlanFree, Status: sub.Status, Limits: limitsForPlan(PlanFree)}, nil
}

func (s *BillingService) GetUsage(ctx context.Context, userID, orgID *uuid.UUID) (PlanUsage, error) {
	if orgID != nil {
		since := time.Now().UTC().AddDate(0, 0, -30)
		count, err := s.Store.CountShipmentsByOrgSince(ctx, *orgID, since)
		if err != nil {
			return PlanUsage{}, fmt.Errorf("count organization shipments: %w", err)
		}
		storageBytes, err := s.Store.SumStorageBytesByOrg(ctx, *orgID)
		if err != nil {
			return PlanUsage{}, fmt.Errorf("sum organization storage bytes: %w", err)
		}
		return PlanUsage{CurrentMonthShipments: count, CurrentStorageBytes: storageBytes}, nil
	}
	if userID == nil {
		return PlanUsage{}, nil
	}
	since := time.Now().UTC().AddDate(0, 0, -30)
	count, err := s.Store.CountShipmentsByUserSince(ctx, *userID, since)
	if err != nil {
		return PlanUsage{}, fmt.Errorf("count shipments: %w", err)
	}
	storageBytes, err := s.Store.SumStorageBytesByUser(ctx, *userID)
	if err != nil {
		return PlanUsage{}, fmt.Errorf("sum storage bytes: %w", err)
	}
	return PlanUsage{CurrentMonthShipments: count, CurrentStorageBytes: storageBytes}, nil
}

func (s *BillingService) GetRemainingQuota(plan UserPlan, usage PlanUsage) RemainingQuota {
	if plan.Limits.MonthlyShipmentLimit <= 0 {
		return RemainingQuota{}
	}
	remaining := plan.Limits.MonthlyShipmentLimit - usage.CurrentMonthShipments
	if remaining < 0 {
		remaining = 0
	}
	return RemainingQuota{RemainingShipments: &remaining}
}

func (s *BillingService) GetPlanDetails(ctx context.Context, userID, orgID *uuid.UUID) (PlanDetails, error) {
	plan, err := s.GetPlan(ctx, userID, orgID)
	if err != nil {
		return PlanDetails{}, err
	}
	usage, err := s.GetUsage(ctx, userID, orgID)
	if err != nil {
		return PlanDetails{}, err
	}
	return PlanDetails{Plan: plan.Name, Status: plan.Status, Limits: plan.Limits, Usage: usage, Remaining: s.GetRemainingQuota(plan, usage)}, nil
}

func (s *BillingService) EnforceUploadLimit(ctx context.Context, userID, orgID *uuid.UUID, fileSize int64) error {
	plan, err := s.GetPlan(ctx, userID, orgID)
	if err != nil {
		return err
	}
	if fileSize > plan.Limits.MaxFileSizeBytes {
		return newPlanLimitError("FILE_SIZE_LIMIT", "無料プランでは1GBまでです")
	}
	return nil
}

func (s *BillingService) EnforceShipmentLimit(ctx context.Context, userID, orgID *uuid.UUID, expiresAt time.Time) error {
	plan, err := s.GetPlan(ctx, userID, orgID)
	if err != nil {
		return err
	}
	maxExpiry := time.Now().UTC().AddDate(0, 0, plan.Limits.MaxRetentionDays)
	if expiresAt.After(maxExpiry) {
		return newPlanLimitError("STORAGE_DAYS_LIMIT", "現在のプランで設定可能な保存期間を超えています")
	}
	if plan.Limits.MonthlyShipmentLimit > 0 {
		usage, err := s.GetUsage(ctx, userID, orgID)
		if err != nil {
			return err
		}
		if usage.CurrentMonthShipments >= plan.Limits.MonthlyShipmentLimit {
			return newPlanLimitError("MONTHLY_SHIPMENT_LIMIT", "月間送信数の上限に達しました")
		}
	}
	return nil
}

type OrganizationBillingDetails struct {
	Plan             string         `json:"plan"`
	Status           string         `json:"status"`
	Usage            PlanUsage      `json:"usage"`
	MembersCount     int64          `json:"members_count"`
	SeatLimit        int64          `json:"seat_limit"`
	CurrentSeatUsage int64          `json:"current_seat_usage"`
	RemainingSeats   int64          `json:"remaining_seats"`
	NextBillingAt    *time.Time     `json:"next_billing_at,omitempty"`
	RemainingQuota   RemainingQuota `json:"remaining"`
}

func (s *BillingService) GetOrganizationBilling(ctx context.Context, actorID, orgID uuid.UUID) (OrganizationBillingDetails, error) {
	member, err := s.Store.GetOrganizationMember(ctx, orgID, actorID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return OrganizationBillingDetails{}, &APIError{Status: 403, Code: "forbidden", Message: "organization へのアクセス権がありません"}
		}
		return OrganizationBillingDetails{}, fmt.Errorf("get organization member: %w", err)
	}
	if member.Role != "owner" {
		return OrganizationBillingDetails{}, &APIError{Status: 403, Code: "forbidden", Message: "organization billing は owner のみ操作できます"}
	}
	planDetails, err := s.GetPlanDetails(ctx, nil, &orgID)
	if err != nil {
		return OrganizationBillingDetails{}, err
	}
	count, err := s.Store.CountOrganizationMembers(ctx, orgID)
	if err != nil {
		return OrganizationBillingDetails{}, fmt.Errorf("count organization members: %w", err)
	}
	seatLimit, err := s.GetSeatLimit(ctx, orgID)
	if err != nil {
		return OrganizationBillingDetails{}, err
	}
	usage, err := s.GetCurrentSeatUsage(ctx, orgID)
	if err != nil {
		return OrganizationBillingDetails{}, err
	}
	remainingSeats := seatLimit - usage
	if remainingSeats < 0 {
		remainingSeats = 0
	}
	var nextBillingAt *time.Time
	if sub, subErr := s.Store.GetLatestSubscriptionByOrgID(ctx, orgID); subErr == nil {
		nextBillingAt = sub.CurrentPeriodEnd
	}
	return OrganizationBillingDetails{
		Plan:             planDetails.Plan,
		Status:           planDetails.Status,
		Usage:            planDetails.Usage,
		MembersCount:     count,
		SeatLimit:        seatLimit,
		CurrentSeatUsage: usage,
		RemainingSeats:   remainingSeats,
		NextBillingAt:    nextBillingAt,
		RemainingQuota:   planDetails.Remaining,
	}, nil
}

func (s *BillingService) ListInvoices(ctx context.Context, actorID, orgID uuid.UUID, limit int64, startingAfter string) (ListInvoicesOutput, error) {
	if s.Stripe == nil {
		return ListInvoicesOutput{}, &APIError{Status: 503, Code: "billing_unavailable", Message: "billing が利用できません"}
	}
	if err := s.requireOrganizationInvoiceAccess(ctx, actorID, orgID); err != nil {
		return ListInvoicesOutput{}, err
	}
	customerID, err := s.getOrganizationStripeCustomerID(ctx, orgID)
	if err != nil {
		return ListInvoicesOutput{}, err
	}
	if limit <= 0 {
		limit = 20
	}
	invoiceList, err := s.Stripe.ListInvoices(ctx, customerID, limit, startingAfter)
	if err != nil {
		return ListInvoicesOutput{}, fmt.Errorf("list stripe invoices: %w", err)
	}
	out := ListInvoicesOutput{
		Invoices: make([]InvoiceSummary, 0, len(invoiceList.Data)),
		HasMore:  invoiceList.HasMore,
	}
	for _, inv := range invoiceList.Data {
		out.Invoices = append(out.Invoices, InvoiceSummary{
			InvoiceID:        inv.ID,
			Amount:           inv.AmountDue,
			Currency:         strings.ToLower(inv.Currency),
			Status:           normalizeInvoiceStatus(inv.Status),
			HostedInvoiceURL: inv.HostedInvoiceURL,
			InvoicePDF:       inv.InvoicePDF,
			CreatedAt:        inv.CreatedAt,
			PaidAt:           inv.PaidAt,
		})
	}
	if out.HasMore && len(out.Invoices) > 0 {
		out.NextStartingAfter = out.Invoices[len(out.Invoices)-1].InvoiceID
	}
	return out, nil
}

func (s *BillingService) GetInvoice(ctx context.Context, actorID, orgID uuid.UUID, invoiceID string) (InvoiceDetail, error) {
	if s.Stripe == nil {
		return InvoiceDetail{}, &APIError{Status: 503, Code: "billing_unavailable", Message: "billing が利用できません"}
	}
	if strings.TrimSpace(invoiceID) == "" {
		return InvoiceDetail{}, &APIError{Status: 400, Code: "invalid_invoice_id", Message: "invoice id が不正です"}
	}
	if err := s.requireOrganizationInvoiceAccess(ctx, actorID, orgID); err != nil {
		return InvoiceDetail{}, err
	}
	customerID, err := s.getOrganizationStripeCustomerID(ctx, orgID)
	if err != nil {
		return InvoiceDetail{}, err
	}
	inv, err := s.Stripe.GetInvoice(ctx, invoiceID)
	if err != nil {
		return InvoiceDetail{}, fmt.Errorf("get stripe invoice: %w", err)
	}
	if strings.TrimSpace(inv.CustomerID) != customerID {
		return InvoiceDetail{}, &APIError{Status: 404, Code: "invoice_not_found", Message: "invoice が見つかりません"}
	}
	return InvoiceDetail{
		InvoiceID:        inv.ID,
		Amount:           inv.AmountDue,
		Currency:         strings.ToLower(inv.Currency),
		Status:           normalizeInvoiceStatus(inv.Status),
		PaymentStatus:    normalizeInvoiceStatus(inv.PaymentStatus),
		PaymentMethod:    inv.PaymentMethod,
		Tax:              InvoiceTaxSummary{Amount: inv.TaxAmount},
		HostedInvoiceURL: inv.HostedInvoiceURL,
		InvoicePDF:       inv.InvoicePDF,
		CreatedAt:        inv.CreatedAt,
		PaidAt:           inv.PaidAt,
		LineItems:        inv.LineItems,
	}, nil
}

func (s *BillingService) GetSeatLimit(ctx context.Context, orgID uuid.UUID) (int64, error) {
	sub, err := s.Store.GetLatestSubscriptionByOrgID(ctx, orgID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return 1, nil
		}
		return 0, fmt.Errorf("get latest organization subscription: %w", err)
	}
	if !isPaidActive(sub.Status) || sub.Plan != PlanPro {
		return 1, nil
	}
	if sub.SeatCount < 1 {
		return 1, nil
	}
	return sub.SeatCount, nil
}

func (s *BillingService) GetCurrentSeatUsage(ctx context.Context, orgID uuid.UUID) (int64, error) {
	usage, err := s.Store.CountOrganizationMembers(ctx, orgID)
	if err != nil {
		return 0, fmt.Errorf("count organization members: %w", err)
	}
	return usage, nil
}

func (s *BillingService) SyncSeatCountWithStripe(ctx context.Context, orgID uuid.UUID) error {
	if s.Stripe == nil {
		return &APIError{Status: 503, Code: "billing_unavailable", Message: "billing が利用できません"}
	}
	sub, err := s.Store.GetLatestSubscriptionByOrgID(ctx, orgID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("get latest organization subscription: %w", err)
	}
	if sub.StripeSubscriptionID == "" || !isPaidActive(sub.Status) || sub.Plan != PlanPro {
		return nil
	}
	usage, err := s.GetCurrentSeatUsage(ctx, orgID)
	if err != nil {
		return err
	}
	if usage < 1 {
		usage = 1
	}
	if err := s.Stripe.UpdateSubscriptionQuantity(ctx, sub.StripeSubscriptionID, usage); err != nil {
		return fmt.Errorf("update stripe subscription quantity: %w", err)
	}
	if _, err := s.Store.UpsertOrgSubscription(ctx, store.UpsertSubscriptionParams{
		OrganizationID:       &orgID,
		StripeCustomerID:     sub.StripeCustomerID,
		StripeSubscriptionID: sub.StripeSubscriptionID,
		SeatCount:            usage,
		Plan:                 sub.Plan,
		Status:               sub.Status,
		CurrentPeriodEnd:     sub.CurrentPeriodEnd,
	}); err != nil {
		return fmt.Errorf("upsert organization subscription: %w", err)
	}
	return nil
}

func (s *BillingService) MarshalPlan(plan UserPlan) json.RawMessage {
	b, _ := json.Marshal(plan)
	return b
}

func limitsForPlan(plan string) PlanLimits {
	switch plan {
	case PlanPro:
		return PlanLimits{MaxFileSizeBytes: 10 * 1024 * 1024 * 1024, MaxRetentionDays: 7, MonthlyShipmentLimit: 0}
	default:
		return PlanLimits{MaxFileSizeBytes: 1 * 1024 * 1024 * 1024, MaxRetentionDays: 3, MonthlyShipmentLimit: 50}
	}
}

func isPaidActive(status string) bool {
	switch strings.ToLower(status) {
	case "active", "trialing", "past_due":
		return true
	default:
		return false
	}
}

func ptrOrNil(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

func newPlanLimitError(code, message string) *APIError {
	return &APIError{
		Status:          403,
		Error:           PlanLimitErrorType,
		Code:            code,
		Message:         message,
		UpgradeRequired: true,
		UpgradeURL:      "/settings/billing",
		RecommendedPlan: RecommendedPlanPro,
	}
}

func (s *BillingService) requireOrganizationInvoiceAccess(ctx context.Context, actorID, orgID uuid.UUID) error {
	member, err := s.Store.GetOrganizationMember(ctx, orgID, actorID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return &APIError{Status: 403, Code: "forbidden", Message: "organization へのアクセス権がありません"}
		}
		return fmt.Errorf("get organization member: %w", err)
	}
	if member.Role != "owner" && member.Role != "admin" {
		return &APIError{Status: 403, Code: "forbidden", Message: "invoice は owner/admin のみ閲覧できます"}
	}
	return nil
}

func (s *BillingService) getOrganizationStripeCustomerID(ctx context.Context, orgID uuid.UUID) (string, error) {
	sub, err := s.Store.GetLatestSubscriptionByOrgID(ctx, orgID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return "", &APIError{Status: 404, Code: "billing_customer_not_found", Message: "organization の請求先情報が見つかりません"}
		}
		return "", fmt.Errorf("get latest organization subscription: %w", err)
	}
	if sub.StripeCustomerID == nil || strings.TrimSpace(*sub.StripeCustomerID) == "" {
		return "", &APIError{Status: 404, Code: "billing_customer_not_found", Message: "organization の請求先情報が見つかりません"}
	}
	return strings.TrimSpace(*sub.StripeCustomerID), nil
}

func normalizeInvoiceStatus(status string) string {
	s := strings.ToLower(strings.TrimSpace(status))
	switch s {
	case "draft", "open", "paid", "uncollectible", "void":
		return s
	default:
		return s
	}
}
