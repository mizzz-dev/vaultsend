package handler

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/example/vaultsend/internal/http/middleware"
	"github.com/example/vaultsend/internal/http/render"
	"github.com/example/vaultsend/internal/service"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type BillingHandler struct {
	Service *service.BillingService
}

type createCheckoutRequest struct {
	OrganizationID *uuid.UUID `json:"organization_id"`
}

func (h BillingHandler) GetPlan(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.AuthUserFromContext(r.Context())
	if !ok {
		render.Error(w, http.StatusUnauthorized, "unauthorized", "ログインが必要です", chimw.GetReqID(r.Context()))
		return
	}
	var orgID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("organization_id")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			render.Error(w, http.StatusBadRequest, "invalid_org_id", "organization id が不正です", chimw.GetReqID(r.Context()))
			return
		}
		orgID = &parsed
	}
	out, err := h.Service.GetPlanDetails(r.Context(), &user.ID, orgID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusOK, out)
}

func (h BillingHandler) CreateCheckout(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.AuthUserFromContext(r.Context())
	if !ok {
		render.Error(w, http.StatusUnauthorized, "unauthorized", "ログインが必要です", chimw.GetReqID(r.Context()))
		return
	}
	var req createCheckoutRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := decodeJSON(w, r, &req); err != nil {
			render.Error(w, http.StatusBadRequest, "invalid_request", "不正なJSONです", chimw.GetReqID(r.Context()))
			return
		}
	}
	out, err := h.Service.CreateCheckoutForOrganization(r.Context(), user.ID, req.OrganizationID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusCreated, out)
}

func (h BillingHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid_request", "body の読み取りに失敗しました", chimw.GetReqID(r.Context()))
		return
	}
	if err := h.Service.HandleWebhook(r.Context(), payload, r.Header.Get("Stripe-Signature")); err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h BillingHandler) GetOrgBilling(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.AuthUserFromContext(r.Context())
	if !ok {
		render.Error(w, http.StatusUnauthorized, "unauthorized", "ログインが必要です", chimw.GetReqID(r.Context()))
		return
	}
	orgID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid_org_id", "organization id が不正です", chimw.GetReqID(r.Context()))
		return
	}
	out, err := h.Service.GetOrganizationBilling(r.Context(), user.ID, orgID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusOK, out)
}

func (h BillingHandler) ListOrgInvoices(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.AuthUserFromContext(r.Context())
	if !ok {
		render.Error(w, http.StatusUnauthorized, "unauthorized", "ログインが必要です", chimw.GetReqID(r.Context()))
		return
	}
	orgID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid_org_id", "organization id が不正です", chimw.GetReqID(r.Context()))
		return
	}
	limit := int64(20)
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || n < 1 || n > 100 {
			render.Error(w, http.StatusBadRequest, "invalid_limit", "limit は 1-100 の範囲で指定してください", chimw.GetReqID(r.Context()))
			return
		}
		limit = n
	}
	out, err := h.Service.ListInvoices(r.Context(), user.ID, orgID, limit, strings.TrimSpace(r.URL.Query().Get("starting_after")))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusOK, out)
}

func (h BillingHandler) GetOrgInvoice(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.AuthUserFromContext(r.Context())
	if !ok {
		render.Error(w, http.StatusUnauthorized, "unauthorized", "ログインが必要です", chimw.GetReqID(r.Context()))
		return
	}
	orgID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid_org_id", "organization id が不正です", chimw.GetReqID(r.Context()))
		return
	}
	invoiceID := strings.TrimSpace(chi.URLParam(r, "invoice_id"))
	if invoiceID == "" {
		render.Error(w, http.StatusBadRequest, "invalid_invoice_id", "invoice id が不正です", chimw.GetReqID(r.Context()))
		return
	}
	out, err := h.Service.GetInvoice(r.Context(), user.ID, orgID, invoiceID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusOK, out)
}
