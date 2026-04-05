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
}

type WebhookSubscriptionEvent struct {
	Type                 string
	StripeSubscriptionID string
	StripeCustomerID     string
	Status               string
	CurrentPeriodEnd     *time.Time
	Metadata             map[string]string
}

type BillingService struct {
	Store       BillingStore
	Stripe      StripeGateway
	FrontendURL string
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
	Plan           string         `json:"plan"`
	Status         string         `json:"status"`
	Usage          PlanUsage      `json:"usage"`
	MembersCount   int64          `json:"members_count"`
	NextBillingAt  *time.Time     `json:"next_billing_at,omitempty"`
	RemainingQuota RemainingQuota `json:"remaining"`
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
	var nextBillingAt *time.Time
	if sub, subErr := s.Store.GetLatestSubscriptionByOrgID(ctx, orgID); subErr == nil {
		nextBillingAt = sub.CurrentPeriodEnd
	}
	return OrganizationBillingDetails{
		Plan:           planDetails.Plan,
		Status:         planDetails.Status,
		Usage:          planDetails.Usage,
		MembersCount:   count,
		NextBillingAt:  nextBillingAt,
		RemainingQuota: planDetails.Remaining,
	}, nil
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
