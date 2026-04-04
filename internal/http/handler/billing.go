package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/example/vaultsend/internal/http/middleware"
	"github.com/example/vaultsend/internal/http/render"
	"github.com/example/vaultsend/internal/service"
	chimw "github.com/go-chi/chi/v5/middleware"
)

type BillingHandler struct {
	Service *service.BillingService
}

func (h BillingHandler) CreateCheckout(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.AuthUserFromContext(r.Context())
	if !ok {
		render.Error(w, http.StatusUnauthorized, "unauthorized", "ログインが必要です", chimw.GetReqID(r.Context()))
		return
	}
	out, err := h.Service.CreateCheckout(r.Context(), user.ID)
	if err != nil {
		h.writeServiceError(w, r, err)
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
		h.writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h BillingHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *service.APIError
	if errors.As(err, &apiErr) {
		render.Error(w, apiErr.Status, apiErr.Code, apiErr.Message, chimw.GetReqID(r.Context()))
		return
	}
	render.Error(w, http.StatusInternalServerError, "internal_error", "内部エラーが発生しました", chimw.GetReqID(r.Context()))
}
