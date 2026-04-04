package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/example/vaultsend/internal/http/middleware"
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
	ShipmentID *uuid.UUID  `json:"shipment_id"`
	FileIDs    []uuid.UUID `json:"file_ids"`
	// 仮置き: 後方互換のためリクエスト項目は受け取るが、認証済み時はサーバー側のユーザーIDを優先する。
	OwnerUserID *uuid.UUID `json:"owner_user_id"`
	Subject     string     `json:"subject"`
	Message     *string    `json:"message"`
	ShareMode   string     `json:"share_mode"`
	Recipients  []struct {
		Email string `json:"email"`
	} `json:"recipients"`
	ExpiresAt        *string `json:"expires_at"`
	MaxDownloadCount *int32  `json:"max_download_count"`
	Password         *string `json:"password"`
}

type ResendShipmentRequest struct {
	RecipientIDs []uuid.UUID `json:"recipient_ids"`
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

	ownerUserID := req.OwnerUserID
	if user, ok := middleware.AuthUserFromContext(r.Context()); ok {
		ownerUserID = &user.ID
	}

	out, err := h.Service.CreateShipment(r.Context(), service.CreateShipmentInput{
		ShipmentID:       req.ShipmentID,
		FileIDs:          req.FileIDs,
		OwnerUserID:      ownerUserID,
		Subject:          req.Subject,
		Message:          req.Message,
		ShareMode:        req.ShareMode,
		Recipients:       recipients,
		ExpiresAt:        expiresAt,
		MaxDownloadCount: req.MaxDownloadCount,
		Password:         req.Password,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusCreated, out)
}

func (h ShipmentHandler) GetShipment(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.AuthUserFromContext(r.Context())
	if !ok {
		render.Error(w, http.StatusUnauthorized, "unauthorized", "ログインが必要です", chimw.GetReqID(r.Context()))
		return
	}
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid_shipment_id", "shipment id が不正です", chimw.GetReqID(r.Context()))
		return
	}

	out, err := h.Service.GetShipmentDetailByUser(r.Context(), user.ID, id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusOK, out)
}

func (h ShipmentHandler) ListShipmentNotifications(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.AuthUserFromContext(r.Context())
	if !ok {
		render.Error(w, http.StatusUnauthorized, "unauthorized", "ログインが必要です", chimw.GetReqID(r.Context()))
		return
	}
	id, ok := parseShipmentIDFromPath(w, r)
	if !ok {
		return
	}
	limit, offset, ok := parseLimitOffset(w, r)
	if !ok {
		return
	}
	out, err := h.Service.ListShipmentNotificationsByUser(r.Context(), service.ListShipmentNotificationsInput{
		OwnerUserID: user.ID,
		ShipmentID:  id,
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusOK, out)
}

func (h ShipmentHandler) ListShipmentRecipients(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.AuthUserFromContext(r.Context())
	if !ok {
		render.Error(w, http.StatusUnauthorized, "unauthorized", "ログインが必要です", chimw.GetReqID(r.Context()))
		return
	}
	id, ok := parseShipmentIDFromPath(w, r)
	if !ok {
		return
	}
	out, err := h.Service.ListShipmentRecipientsByUser(r.Context(), user.ID, id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h ShipmentHandler) ListShipments(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.AuthUserFromContext(r.Context())
	if !ok {
		render.Error(w, http.StatusUnauthorized, "unauthorized", "ログインが必要です", chimw.GetReqID(r.Context()))
		return
	}
	limit, offset, ok := parseLimitOffset(w, r)
	if !ok {
		return
	}

	out, err := h.Service.ListShipmentsByUser(r.Context(), service.ShipmentListInput{
		OwnerUserID: user.ID,
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusOK, out)
}

func parseShipmentIDFromPath(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid_shipment_id", "shipment id が不正です", chimw.GetReqID(r.Context()))
		return uuid.Nil, false
	}
	return id, true
}

func parseLimitOffset(w http.ResponseWriter, r *http.Request) (int32, int32, bool) {
	limit := int32(20)
	offset := int32(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			render.Error(w, http.StatusBadRequest, "invalid_limit", "limit が不正です", chimw.GetReqID(r.Context()))
			return 0, 0, false
		}
		limit = int32(v)
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			render.Error(w, http.StatusBadRequest, "invalid_offset", "offset が不正です", chimw.GetReqID(r.Context()))
			return 0, 0, false
		}
		offset = int32(v)
	}
	return limit, offset, true
}

func (h ShipmentHandler) DeleteShipment(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.AuthUserFromContext(r.Context())
	if !ok {
		render.Error(w, http.StatusUnauthorized, "unauthorized", "ログインが必要です", chimw.GetReqID(r.Context()))
		return
	}
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid_shipment_id", "shipment id が不正です", chimw.GetReqID(r.Context()))
		return
	}
	if err := h.Service.DeleteShipmentByUser(r.Context(), user.ID, id); err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h ShipmentHandler) ResendShipment(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.AuthUserFromContext(r.Context())
	if !ok {
		render.Error(w, http.StatusUnauthorized, "unauthorized", "ログインが必要です", chimw.GetReqID(r.Context()))
		return
	}
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid_shipment_id", "shipment id が不正です", chimw.GetReqID(r.Context()))
		return
	}

	var req ResendShipmentRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := decodeJSON(w, r, &req); err != nil {
			render.Error(w, http.StatusBadRequest, "invalid_request", "不正なJSONです", chimw.GetReqID(r.Context()))
			return
		}
	}

	out, err := h.Service.ResendShipmentNotification(r.Context(), service.ResendShipmentInput{
		OwnerUserID:  user.ID,
		ShipmentID:   id,
		RecipientIDs: req.RecipientIDs,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusOK, out)
}
