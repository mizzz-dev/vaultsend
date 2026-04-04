package middleware

import (
	"context"
	"net/http"

	"github.com/example/vaultsend/internal/service"
	"github.com/google/uuid"
)

const planKey ctxKey = "billing_plan"

type BillingPlanContext struct {
	Plan      service.UserPlan       `json:"plan"`
	Usage     service.PlanUsage      `json:"usage"`
	Remaining service.RemainingQuota `json:"remaining"`
}

type BillingPlanner interface {
	GetPlanDetails(ctx context.Context, userID *uuid.UUID) (service.PlanDetails, error)
}

func OptionalPlan(planner BillingPlanner) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			resolved := BillingPlanContext{Plan: service.UserPlan{Name: service.PlanFree, Status: "inactive", Limits: service.PlanLimits{MaxFileSizeBytes: 1 * 1024 * 1024 * 1024, MaxRetentionDays: 3, MonthlyShipmentLimit: 50}}}
			if planner != nil {
				var uid *uuid.UUID
				if user, ok := AuthUserFromContext(ctx); ok {
					uid = &user.ID
				}
				if details, err := planner.GetPlanDetails(ctx, uid); err == nil {
					resolved = BillingPlanContext{
						Plan:      service.UserPlan{Name: details.Plan, Status: details.Status, Limits: details.Limits},
						Usage:     details.Usage,
						Remaining: details.Remaining,
					}
				}
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, planKey, resolved)))
		})
	}
}

func PlanFromContext(ctx context.Context) (BillingPlanContext, bool) {
	v := ctx.Value(planKey)
	if v == nil {
		return BillingPlanContext{}, false
	}
	plan, ok := v.(BillingPlanContext)
	return plan, ok
}
