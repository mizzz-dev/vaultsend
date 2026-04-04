package handler

import (
	"errors"
	"net/http"

	"github.com/example/vaultsend/internal/http/render"
	"github.com/example/vaultsend/internal/service"
	chimw "github.com/go-chi/chi/v5/middleware"
)

func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *service.APIError
	if errors.As(err, &apiErr) {
		render.ServiceError(w, apiErr, chimw.GetReqID(r.Context()))
		return
	}
	render.Error(w, http.StatusInternalServerError, "internal_error", "内部エラーが発生しました", chimw.GetReqID(r.Context()))
}
