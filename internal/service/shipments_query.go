package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/example/vaultsend/internal/store"
	"github.com/google/uuid"
)

const (
	defaultShipmentListLimit int32 = 20
	maxShipmentListLimit     int32 = 100
)

type ShipmentListInput struct {
	OwnerUserID uuid.UUID
	Limit       int32
	Offset      int32
}

type ShipmentListItem struct {
	ID               uuid.UUID `json:"id"`
	Subject          string    `json:"subject"`
	ShareMode        string    `json:"share_mode"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	ExpiresAt        time.Time `json:"expires_at"`
	DownloadCount    int32     `json:"download_count"`
	MaxDownloadCount int32     `json:"max_download_count"`
	FileCount        int32     `json:"file_count"`
}

type ListShipmentsOutput struct {
	Items  []ShipmentListItem `json:"items"`
	Limit  int32              `json:"limit"`
	Offset int32              `json:"offset"`
	Total  int64              `json:"total"`
}

type ShipmentRecipientDownloadView struct {
	RecipientID    uuid.UUID  `json:"recipient_id"`
	Email          string     `json:"email"`
	DownloadCount  int32      `json:"download_count"`
	LastDownloadAt *time.Time `json:"last_download_at,omitempty"`
}

type ShipmentDetailFileView struct {
	ID       uuid.UUID `json:"id"`
	FileName string    `json:"file_name"`
	Size     int64     `json:"size"`
}

type ShipmentDetailOutput struct {
	ID                      uuid.UUID                       `json:"id"`
	Status                  string                          `json:"status"`
	ShareMode               string                          `json:"share_mode"`
	Subject                 string                          `json:"subject"`
	Message                 *string                         `json:"message,omitempty"`
	ExpiresAt               time.Time                       `json:"expires_at"`
	MaxDownloadCount        int32                           `json:"max_download_count"`
	DownloadCount           int32                           `json:"download_count"`
	LastDownloadAt          *time.Time                      `json:"last_download_at,omitempty"`
	Files                   []ShipmentDetailFileView        `json:"files"`
	Recipients              []CreateShipmentRecipientView   `json:"recipients"`
	RecipientDownloadEvents []ShipmentRecipientDownloadView `json:"recipient_downloads"`
}

func (s *ShipmentService) ListShipmentsByUser(ctx context.Context, in ShipmentListInput) (ListShipmentsOutput, error) {
	if in.OwnerUserID == uuid.Nil {
		return ListShipmentsOutput{}, &APIError{Status: 401, Code: "unauthorized", Message: "ログインが必要です"}
	}
	if in.Limit <= 0 {
		in.Limit = defaultShipmentListLimit
	}
	if in.Limit > maxShipmentListLimit {
		return ListShipmentsOutput{}, &APIError{Status: 400, Code: "invalid_limit", Message: "limit が上限を超えています"}
	}
	if in.Offset < 0 {
		return ListShipmentsOutput{}, &APIError{Status: 400, Code: "invalid_offset", Message: "offset が不正です"}
	}

	rows, err := s.Store.ListShipmentsByUser(ctx, in.OwnerUserID, in.Limit, in.Offset)
	if err != nil {
		return ListShipmentsOutput{}, fmt.Errorf("list shipments by user: %w", err)
	}
	total, err := s.Store.CountShipmentsByUser(ctx, in.OwnerUserID)
	if err != nil {
		return ListShipmentsOutput{}, fmt.Errorf("count shipments by user: %w", err)
	}
	items := make([]ShipmentListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, ShipmentListItem{
			ID:               row.ID,
			Subject:          row.Title,
			ShareMode:        normalizeShareModeForResponse(row.ShareMode),
			Status:           row.Status,
			CreatedAt:        row.CreatedAt,
			ExpiresAt:        row.ExpiresAt,
			DownloadCount:    row.DownloadCount,
			MaxDownloadCount: row.MaxDownloads,
			FileCount:        row.FileCount,
		})
	}
	return ListShipmentsOutput{Items: items, Limit: in.Limit, Offset: in.Offset, Total: total}, nil
}

func (s *ShipmentService) GetShipmentDetailByUser(ctx context.Context, ownerUserID uuid.UUID, shipmentID uuid.UUID) (ShipmentDetailOutput, error) {
	shipment, err := s.Store.GetShipment(ctx, shipmentID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ShipmentDetailOutput{}, &APIError{Status: 404, Code: "shipment_not_found", Message: "shipment が見つかりません"}
		}
		return ShipmentDetailOutput{}, fmt.Errorf("get shipment: %w", err)
	}
	if shipment.OwnerUserID == nil || *shipment.OwnerUserID != ownerUserID {
		return ShipmentDetailOutput{}, &APIError{Status: 403, Code: "forbidden", Message: "他ユーザーの shipment にはアクセスできません"}
	}

	files, err := s.Store.GetFilesByShipmentID(ctx, shipmentID)
	if err != nil {
		return ShipmentDetailOutput{}, fmt.Errorf("get files by shipment: %w", err)
	}
	recipients, err := s.Store.GetRecipientsByShipmentID(ctx, shipmentID)
	if err != nil {
		return ShipmentDetailOutput{}, fmt.Errorf("get recipients by shipment: %w", err)
	}
	downloadCount, err := s.Store.CountDownloadEventsByShipment(ctx, shipmentID)
	if err != nil {
		return ShipmentDetailOutput{}, fmt.Errorf("count download events by shipment: %w", err)
	}
	recipientStats, err := s.Store.GetRecipientDownloadStatsByShipment(ctx, shipmentID)
	if err != nil {
		return ShipmentDetailOutput{}, fmt.Errorf("get recipient download stats by shipment: %w", err)
	}

	out := ShipmentDetailOutput{
		ID:                      shipment.ID,
		Status:                  shipment.Status,
		ShareMode:               normalizeShareModeForResponse(shipment.ShareMode),
		Subject:                 shipment.Title,
		Message:                 shipment.Message,
		ExpiresAt:               shipment.ExpiresAt,
		MaxDownloadCount:        shipment.MaxDownloads,
		DownloadCount:           downloadCount,
		Files:                   make([]ShipmentDetailFileView, 0, len(files)),
		Recipients:              make([]CreateShipmentRecipientView, 0, len(recipients)),
		RecipientDownloadEvents: make([]ShipmentRecipientDownloadView, 0, len(recipientStats)),
	}
	for _, f := range files {
		out.Files = append(out.Files, ShipmentDetailFileView{ID: f.ID, FileName: f.OriginalName, Size: f.SizeBytes})
	}
	for _, r := range recipients {
		out.Recipients = append(out.Recipients, CreateShipmentRecipientView{ID: r.ID, Email: r.Email, Status: r.Status})
	}
	for _, item := range recipientStats {
		out.RecipientDownloadEvents = append(out.RecipientDownloadEvents, ShipmentRecipientDownloadView{
			RecipientID:    item.RecipientID,
			Email:          item.Email,
			DownloadCount:  item.DownloadCount,
			LastDownloadAt: item.LastDownloadAt,
		})
		if item.LastDownloadAt != nil && (out.LastDownloadAt == nil || item.LastDownloadAt.After(*out.LastDownloadAt)) {
			out.LastDownloadAt = item.LastDownloadAt
		}
	}
	return out, nil
}

func (s *ShipmentService) DeleteShipmentByUser(ctx context.Context, ownerUserID uuid.UUID, shipmentID uuid.UUID) error {
	shipment, err := s.Store.GetShipment(ctx, shipmentID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return &APIError{Status: 404, Code: "shipment_not_found", Message: "shipment が見つかりません"}
		}
		return fmt.Errorf("get shipment: %w", err)
	}
	if shipment.OwnerUserID == nil || *shipment.OwnerUserID != ownerUserID {
		return &APIError{Status: 403, Code: "forbidden", Message: "他ユーザーの shipment は削除できません"}
	}
	if shipment.Status == "deleted" || shipment.Status == "revoked" {
		return &APIError{Status: 409, Code: "invalid_shipment_status", Message: "この shipment は削除できない状態です"}
	}
	if err := s.Store.DeleteShipment(ctx, shipmentID); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return &APIError{Status: 409, Code: "invalid_shipment_status", Message: "この shipment は削除できない状態です"}
		}
		return fmt.Errorf("delete shipment: %w", err)
	}
	if err := s.Store.RevokeAccessTokensByShipment(ctx, shipmentID); err != nil {
		return fmt.Errorf("revoke access tokens by shipment: %w", err)
	}
	return nil
}
