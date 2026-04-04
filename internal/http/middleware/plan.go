package middleware

import (
	"context"
	"net/http"

	"github.com/example/vaultsend/internal/service"
	"github.com/google/uuid"
)

const planKey ctxKey = "billing_plan"

type BillingPlanner interface {
	GetUserPlan(ctx context.Context, userID *uuid.UUID) (service.UserPlan, error)
}

func OptionalPlan(planner BillingPlanner) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			resolvedPlan := service.UserPlan{Name: service.PlanFree, Status: "inactive", Limits: service.PlanLimits{MaxFileSizeBytes: 1 * 1024 * 1024 * 1024, MaxRetentionDays: 3, MonthlyShipmentLimit: 50}}
			if planner != nil {
				if user, ok := AuthUserFromContext(ctx); ok {
					if plan, err := planner.GetUserPlan(ctx, &user.ID); err == nil {
						resolvedPlan = plan
					}
				}
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, planKey, resolvedPlan)))
		})
	}
}

func PlanFromContext(ctx context.Context) (service.UserPlan, bool) {
	v := ctx.Value(planKey)
	if v == nil {
		return service.UserPlan{}, false
	}
	plan, ok := v.(service.UserPlan)
	return plan, ok
}
