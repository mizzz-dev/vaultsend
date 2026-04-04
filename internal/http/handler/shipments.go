package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/example/vaultsend/internal/http/render"
	"github.com/example/vaultsend/internal/store"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type ShipmentHandler struct {
	Queries *store.Queries
}

type CreateShipmentRequest struct {
	ShipmentID      uuid.UUID `json:"shipment_id"`
	Title           string    `json:"title"`
	Message         string    `json:"message"`
	ShareMode       string    `json:"share_mode"`
	RecipientEmails []string  `json:"recipient_emails"`
	ExpiresInDays   int       `json:"expires_in_days"`
	MaxDownloads    int32     `json:"max_downloads"`
}

func (h ShipmentHandler) CreateShipment(w http.ResponseWriter, r *http.Request) {
	var req CreateShipmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Error(w, http.StatusBadRequest, "invalid_request", "不正なJSONです", chimw.GetReqID(r.Context()))
		return
	}

	// TODO: 次PRで ready 状態検証、recipients 登録、SQS通知投入をトランザクションで実装する。
	shipment, err := h.Queries.GetShipment(r.Context(), req.ShipmentID)
	if err != nil {
		render.Error(w, http.StatusNotFound, "shipment_not_found", "shipment が見つかりません", chimw.GetReqID(r.Context()))
		return
	}

	// 仮置き: sentへの状態更新は未実装。レスポンス形のみ先行で固定。
	render.JSON(w, http.StatusCreated, map[string]any{
		"id":         shipment.ID,
		"status":     "sent",
		"access_url": "https://app.example.com/r/TODO_TOKEN",
		"expires_at": time.Now().UTC().AddDate(0, 0, 7),
	})
}

func (h ShipmentHandler) GetShipment(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid_shipment_id", "shipment id が不正です", chimw.GetReqID(r.Context()))
		return
	}

	shipment, err := h.Queries.GetShipment(r.Context(), id)
	if err != nil {
		render.Error(w, http.StatusNotFound, "not_found", "shipment が見つかりません", chimw.GetReqID(r.Context()))
		return
	}

	// TODO: 次PRで files / recipients をjoinして詳細レスポンスを返却する。
	render.JSON(w, http.StatusOK, map[string]any{
		"id":         shipment.ID,
		"status":     shipment.Status,
		"files":      []any{},
		"recipients": []any{},
	})
}
