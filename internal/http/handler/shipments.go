package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/example/vaultsend/internal/http/render"
	"github.com/example/vaultsend/internal/service"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type ShipmentHandler struct {
	Service *service.ShipmentService
}

type CreateShipmentRequest struct {
	ShipmentID  *uuid.UUID  `json:"shipment_id"`
	FileIDs     []uuid.UUID `json:"file_ids"`
	OwnerUserID *uuid.UUID  `json:"owner_user_id"`
	Subject     string      `json:"subject"`
	Message     *string     `json:"message"`
	ShareMode   string      `json:"share_mode"`
	Recipients  []struct {
		Email string `json:"email"`
	} `json:"recipients"`
	ExpiresAt        *string `json:"expires_at"`
	MaxDownloadCount *int32  `json:"max_download_count"`
	Password         *string `json:"password"`
}

func (h ShipmentHandler) CreateShipment(w http.ResponseWriter, r *http.Request) {
	var req CreateShipmentRequest
	if err := decodeJSON(w, r, &req); err != nil {
		render.Error(w, http.StatusBadRequest, "invalid_request", "不正なJSONです", chimw.GetReqID(r.Context()))
		return
	}
	if len(strings.TrimSpace(req.Subject)) > 200 {
		render.Error(w, http.StatusBadRequest, "invalid_subject", "subject が長すぎます", chimw.GetReqID(r.Context()))
		return
	}
	if req.Message != nil && len(strings.TrimSpace(*req.Message)) > 5000 {
		render.Error(w, http.StatusBadRequest, "invalid_message", "message が長すぎます", chimw.GetReqID(r.Context()))
		return
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		parsed, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			render.Error(w, http.StatusBadRequest, "invalid_expires_at", "expires_at は RFC3339 形式で指定してください", chimw.GetReqID(r.Context()))
			return
		}
		expiresAt = &parsed
	}

	recipients := make([]service.ShipmentRecipientInput, 0, len(req.Recipients))
	for _, rc := range req.Recipients {
		if !isValidEmail(rc.Email) {
			render.Error(w, http.StatusBadRequest, "invalid_recipients", "email 形式が不正です", chimw.GetReqID(r.Context()))
			return
		}
		recipients = append(recipients, service.ShipmentRecipientInput{Email: rc.Email})
	}

	out, err := h.Service.CreateShipment(r.Context(), service.CreateShipmentInput{
		ShipmentID:       req.ShipmentID,
		FileIDs:          req.FileIDs,
		OwnerUserID:      req.OwnerUserID,
		Subject:          req.Subject,
		Message:          req.Message,
		ShareMode:        req.ShareMode,
		Recipients:       recipients,
		ExpiresAt:        expiresAt,
		MaxDownloadCount: req.MaxDownloadCount,
		Password:         req.Password,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusCreated, out)
}

func (h ShipmentHandler) GetShipment(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid_shipment_id", "shipment id が不正です", chimw.GetReqID(r.Context()))
		return
	}

	out, err := h.Service.GetShipmentDetail(r.Context(), id)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusOK, out)
}

func (h ShipmentHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *service.APIError
	if errors.As(err, &apiErr) {
		render.Error(w, apiErr.Status, apiErr.Code, apiErr.Message, chimw.GetReqID(r.Context()))
		return
	}
	render.Error(w, http.StatusInternalServerError, "internal_error", "内部エラーが発生しました", chimw.GetReqID(r.Context()))
}
