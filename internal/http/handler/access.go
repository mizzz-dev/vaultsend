package handler

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/example/vaultsend/internal/http/render"
	"github.com/example/vaultsend/internal/service"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type AccessHandler struct {
	Service *service.AccessService
}

func (h AccessHandler) InspectAccess(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if len(token) > 256 || strings.TrimSpace(token) == "" {
		render.Error(w, http.StatusBadRequest, "invalid_token", "token が不正です", chimw.GetReqID(r.Context()))
		return
	}
	out, err := h.Service.InspectAccess(r.Context(), token)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusOK, out)
}

func (h AccessHandler) VerifyAccess(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	var req struct {
		Password *string `json:"password"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		render.Error(w, http.StatusBadRequest, "invalid_request", "不正なJSONです", chimw.GetReqID(r.Context()))
		return
	}
	if req.Password != nil && len(*req.Password) > 256 {
		render.Error(w, http.StatusBadRequest, "invalid_password", "password が長すぎます", chimw.GetReqID(r.Context()))
		return
	}
	if err := h.Service.VerifyAccess(r.Context(), service.VerifyAccessInput{Token: token, Password: req.Password}); err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusOK, map[string]any{"granted": true})
}

func (h AccessHandler) GenerateDownloadURL(w http.ResponseWriter, r *http.Request) {
	fileID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid_file_id", "file id が不正です", chimw.GetReqID(r.Context()))
		return
	}
	token := r.URL.Query().Get("access_token")
	if len(token) > 256 || strings.TrimSpace(token) == "" {
		render.Error(w, http.StatusBadRequest, "invalid_token", "access_token が不正です", chimw.GetReqID(r.Context()))
		return
	}
	out, err := h.Service.GenerateDownloadURL(r.Context(), service.DownloadURLInput{
		Token:     token,
		FileID:    fileID,
		IPAddress: clientIP(r),
		UserAgent: r.UserAgent(),
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusOK, out)
}

func (h AccessHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *service.APIError
	if errors.As(err, &apiErr) {
		render.Error(w, apiErr.Status, apiErr.Code, apiErr.Message, chimw.GetReqID(r.Context()))
		return
	}
	render.Error(w, http.StatusInternalServerError, "internal_error", "内部エラーが発生しました", chimw.GetReqID(r.Context()))
}

func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		return v
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
