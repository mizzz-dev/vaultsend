package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/example/vaultsend/internal/service"
	"github.com/google/uuid"
)

type fakePlanner struct{}

func (f *fakePlanner) GetPlanDetails(ctx context.Context, userID, orgID *uuid.UUID) (service.PlanDetails, error) {
	remaining := int64(10)
	return service.PlanDetails{
		Plan:      service.PlanFree,
		Status:    "inactive",
		Limits:    service.PlanLimits{MaxFileSizeBytes: 1, MaxRetentionDays: 3, MonthlyShipmentLimit: 50},
		Usage:     service.PlanUsage{CurrentMonthShipments: 40},
		Remaining: service.RemainingQuota{RemainingShipments: &remaining},
	}, nil
}

func TestOptionalPlanInjectsPlanUsage(t *testing.T) {
	h := OptionalPlan(&fakePlanner{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plan, ok := PlanFromContext(r.Context())
		if !ok {
			t.Fatal("plan missing")
		}
		if plan.Usage.CurrentMonthShipments != 40 {
			t.Fatalf("unexpected usage: %+v", plan.Usage)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/v1/shipments", nil)
	req = req.WithContext(WithAuthUser(req.Context(), service.AuthUser{ID: uuid.New(), Email: "u@example.com"}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d", w.Code)
	}
}
