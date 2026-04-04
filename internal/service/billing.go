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
	GetLatestSubscriptionByUserID(ctx context.Context, userID uuid.UUID) (store.Subscription, error)
	UpsertSubscription(ctx context.Context, arg store.UpsertSubscriptionParams) (store.Subscription, error)
	CountShipmentsByUserSince(ctx context.Context, ownerUserID uuid.UUID, since time.Time) (int64, error)
	SumStorageBytesByUser(ctx context.Context, ownerUserID uuid.UUID) (int64, error)
}

type CheckoutSession struct {
	ID  string
	URL string
}

type CheckoutInput struct {
	UserID     uuid.UUID
	UserEmail  string
	SuccessURL string
	CancelURL  string
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
	userIDRaw := strings.TrimSpace(evt.Metadata["user_id"])
	if userIDRaw == "" {
		return nil
	}
	userID, err := uuid.Parse(userIDRaw)
	if err != nil {
		return nil
	}
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
	_, err = s.Store.UpsertSubscription(ctx, store.UpsertSubscriptionParams{
		UserID:               userID,
		StripeCustomerID:     ptrOrNil(evt.StripeCustomerID),
		StripeSubscriptionID: evt.StripeSubscriptionID,
		Plan:                 plan,
		Status:               status,
		CurrentPeriodEnd:     evt.CurrentPeriodEnd,
	})
	if err != nil {
		return fmt.Errorf("upsert subscription: %w", err)
	}
	return nil
}

func (s *BillingService) GetUserPlan(ctx context.Context, userID *uuid.UUID) (UserPlan, error) {
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

func (s *BillingService) GetUsage(ctx context.Context, userID *uuid.UUID) (PlanUsage, error) {
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

func (s *BillingService) GetPlanDetails(ctx context.Context, userID *uuid.UUID) (PlanDetails, error) {
	plan, err := s.GetUserPlan(ctx, userID)
	if err != nil {
		return PlanDetails{}, err
	}
	usage, err := s.GetUsage(ctx, userID)
	if err != nil {
		return PlanDetails{}, err
	}
	return PlanDetails{Plan: plan.Name, Status: plan.Status, Limits: plan.Limits, Usage: usage, Remaining: s.GetRemainingQuota(plan, usage)}, nil
}

func (s *BillingService) EnforceUploadLimit(ctx context.Context, userID *uuid.UUID, fileSize int64) error {
	plan, err := s.GetUserPlan(ctx, userID)
	if err != nil {
		return err
	}
	if fileSize > plan.Limits.MaxFileSizeBytes {
		return newPlanLimitError("FILE_SIZE_LIMIT", "無料プランでは1GBまでです")
	}
	return nil
}

func (s *BillingService) EnforceShipmentLimit(ctx context.Context, userID *uuid.UUID, expiresAt time.Time) error {
	plan, err := s.GetUserPlan(ctx, userID)
	if err != nil {
		return err
	}
	maxExpiry := time.Now().UTC().AddDate(0, 0, plan.Limits.MaxRetentionDays)
	if expiresAt.After(maxExpiry) {
		return newPlanLimitError("STORAGE_DAYS_LIMIT", "現在のプランで設定可能な保存期間を超えています")
	}
	if userID != nil && plan.Limits.MonthlyShipmentLimit > 0 {
		usage, err := s.GetUsage(ctx, userID)
		if err != nil {
			return err
		}
		if usage.CurrentMonthShipments >= plan.Limits.MonthlyShipmentLimit {
			return newPlanLimitError("MONTHLY_SHIPMENT_LIMIT", "月間送信数の上限に達しました")
		}
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
