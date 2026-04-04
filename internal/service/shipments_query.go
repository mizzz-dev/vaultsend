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
	RecipientID     uuid.UUID  `json:"recipient_id"`
	Email           string     `json:"email"`
	DownloadCount   int32      `json:"download_count"`
	FirstDownloadAt *time.Time `json:"first_download_at,omitempty"`
	LastDownloadAt  *time.Time `json:"last_download_at,omitempty"`
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
	NotificationSummary     ShipmentNotificationSummaryView `json:"notification_summary"`
	RecipientSummaries      []ShipmentRecipientSummaryView  `json:"recipient_summaries"`
}

type ShipmentNotificationSummaryView struct {
	TotalNotifications int64      `json:"total_notifications"`
	QueuedCount        int32      `json:"queued_count"`
	SentCount          int32      `json:"sent_count"`
	FailedCount        int32      `json:"failed_count"`
	LastNotificationAt *time.Time `json:"last_notification_at,omitempty"`
}

type ShipmentRecipientSummaryView struct {
	RecipientID            uuid.UUID  `json:"recipient_id"`
	Email                  string     `json:"email"`
	RecipientStatus        string     `json:"recipient_status"`
	NotificationCount      int32      `json:"notification_count"`
	LastNotificationStatus *string    `json:"last_notification_status,omitempty"`
	LastNotificationType   *string    `json:"last_notification_type,omitempty"`
	LastNotifiedAt         *time.Time `json:"last_notified_at,omitempty"`
	FirstDownloadAt        *time.Time `json:"first_download_at,omitempty"`
	LastDownloadAt         *time.Time `json:"last_download_at,omitempty"`
	DownloadCount          int32      `json:"download_count"`
	HasDownloaded          bool       `json:"has_downloaded"`
}

type ListShipmentNotificationsInput struct {
	OwnerUserID uuid.UUID
	ShipmentID  uuid.UUID
	Limit       int32
	Offset      int32
}

type ShipmentNotificationEventView struct {
	NotificationEventID int64      `json:"notification_event_id"`
	RecipientID         uuid.UUID  `json:"recipient_id"`
	RecipientEmail      string     `json:"recipient_email"`
	EventType           string     `json:"event_type"`
	Status              string     `json:"status"`
	ErrorMessage        *string    `json:"error_message,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	QueuedAt            *time.Time `json:"queued_at,omitempty"`
	SentAt              *time.Time `json:"sent_at,omitempty"`
	FailedAt            *time.Time `json:"failed_at,omitempty"`
}

type ListShipmentNotificationsOutput struct {
	Items  []ShipmentNotificationEventView `json:"items"`
	Limit  int32                           `json:"limit"`
	Offset int32                           `json:"offset"`
	Total  int64                           `json:"total"`
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
	notificationEvents, err := s.Store.GetNotificationEventsByShipmentID(ctx, shipmentID)
	if err != nil {
		return ShipmentDetailOutput{}, fmt.Errorf("get notification events by shipment: %w", err)
	}
	recipientNotificationStats, err := s.Store.GetRecipientNotificationStatsByShipmentID(ctx, shipmentID)
	if err != nil {
		return ShipmentDetailOutput{}, fmt.Errorf("get recipient notification stats by shipment: %w", err)
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
		RecipientSummaries:      make([]ShipmentRecipientSummaryView, 0, len(recipients)),
	}
	for _, f := range files {
		out.Files = append(out.Files, ShipmentDetailFileView{ID: f.ID, FileName: f.OriginalName, Size: f.SizeBytes})
	}
	for _, r := range recipients {
		out.Recipients = append(out.Recipients, CreateShipmentRecipientView{ID: r.ID, Email: r.Email, Status: r.Status})
	}
	for _, item := range recipientStats {
		out.RecipientDownloadEvents = append(out.RecipientDownloadEvents, ShipmentRecipientDownloadView{
			RecipientID:     item.RecipientID,
			Email:           item.Email,
			DownloadCount:   item.DownloadCount,
			FirstDownloadAt: item.FirstDownloadAt,
			LastDownloadAt:  item.LastDownloadAt,
		})
		if item.LastDownloadAt != nil && (out.LastDownloadAt == nil || item.LastDownloadAt.After(*out.LastDownloadAt)) {
			out.LastDownloadAt = item.LastDownloadAt
		}
	}
	// 通知サマリはイベント履歴をそのまま集計する（仮置き: 再送判断しやすさを優先）。
	for _, ev := range notificationEvents {
		switch ev.Status {
		case "queued":
			out.NotificationSummary.QueuedCount++
		case "sent":
			out.NotificationSummary.SentCount++
		case "failed":
			out.NotificationSummary.FailedCount++
		}
		if out.NotificationSummary.LastNotificationAt == nil || ev.CreatedAt.After(*out.NotificationSummary.LastNotificationAt) {
			ts := ev.CreatedAt
			out.NotificationSummary.LastNotificationAt = &ts
		}
	}
	out.NotificationSummary.TotalNotifications = int64(len(notificationEvents))

	// recipient単位で通知/受領を突き合わせるため、まずは map 化して O(1) で引けるようにする。
	downloadStatByRecipient := make(map[uuid.UUID]store.RecipientDownloadStat, len(recipientStats))
	for _, st := range recipientStats {
		downloadStatByRecipient[st.RecipientID] = st
	}
	notificationStatByRecipient := make(map[uuid.UUID]store.RecipientNotificationStat, len(recipientNotificationStats))
	for _, st := range recipientNotificationStats {
		notificationStatByRecipient[st.RecipientID] = st
	}
	for _, rc := range recipients {
		// 仮置き: recipient.status は DB値をそのまま返し、受領有無は has_downloaded で補完する。
		downloadStat := downloadStatByRecipient[rc.ID]
		notificationStat := notificationStatByRecipient[rc.ID]
		out.RecipientSummaries = append(out.RecipientSummaries, ShipmentRecipientSummaryView{
			RecipientID:            rc.ID,
			Email:                  rc.Email,
			RecipientStatus:        rc.Status,
			NotificationCount:      notificationStat.NotificationCount,
			LastNotificationStatus: notificationStat.LastNotificationStatus,
			LastNotificationType:   notificationStat.LastNotificationType,
			LastNotifiedAt:         notificationStat.LastNotificationAt,
			FirstDownloadAt:        downloadStat.FirstDownloadAt,
			LastDownloadAt:         downloadStat.LastDownloadAt,
			DownloadCount:          downloadStat.DownloadCount,
			HasDownloaded:          downloadStat.DownloadCount > 0,
		})
	}
	return out, nil
}

func (s *ShipmentService) ListShipmentNotificationsByUser(ctx context.Context, in ListShipmentNotificationsInput) (ListShipmentNotificationsOutput, error) {
	if in.OwnerUserID == uuid.Nil {
		return ListShipmentNotificationsOutput{}, &APIError{Status: 401, Code: "unauthorized", Message: "ログインが必要です"}
	}
	if in.Limit <= 0 {
		in.Limit = defaultShipmentListLimit
	}
	if in.Limit > maxShipmentListLimit {
		return ListShipmentNotificationsOutput{}, &APIError{Status: 400, Code: "invalid_limit", Message: "limit が上限を超えています"}
	}
	if in.Offset < 0 {
		return ListShipmentNotificationsOutput{}, &APIError{Status: 400, Code: "invalid_offset", Message: "offset が不正です"}
	}
	shipment, err := s.Store.GetShipment(ctx, in.ShipmentID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ListShipmentNotificationsOutput{}, &APIError{Status: 404, Code: "shipment_not_found", Message: "shipment が見つかりません"}
		}
		return ListShipmentNotificationsOutput{}, fmt.Errorf("get shipment: %w", err)
	}
	if shipment.OwnerUserID == nil || *shipment.OwnerUserID != in.OwnerUserID {
		return ListShipmentNotificationsOutput{}, &APIError{Status: 403, Code: "forbidden", Message: "他ユーザーの shipment にはアクセスできません"}
	}
	rows, err := s.Store.ListNotificationEventsByShipmentID(ctx, in.ShipmentID, in.Limit, in.Offset)
	if err != nil {
		return ListShipmentNotificationsOutput{}, fmt.Errorf("list notification events by shipment id: %w", err)
	}
	total, err := s.Store.CountNotificationEventsByShipmentID(ctx, in.ShipmentID)
	if err != nil {
		return ListShipmentNotificationsOutput{}, fmt.Errorf("count notification events by shipment id: %w", err)
	}
	out := ListShipmentNotificationsOutput{
		Items:  make([]ShipmentNotificationEventView, 0, len(rows)),
		Limit:  in.Limit,
		Offset: in.Offset,
		Total:  total,
	}
	for _, row := range rows {
		out.Items = append(out.Items, ShipmentNotificationEventView{
			NotificationEventID: row.ID,
			RecipientID:         row.RecipientID,
			RecipientEmail:      row.RecipientEmail,
			EventType:           row.EventType,
			Status:              row.Status,
			ErrorMessage:        row.ErrorMessage,
			CreatedAt:           row.CreatedAt,
			QueuedAt:            row.QueuedAt,
			SentAt:              row.SentAt,
			FailedAt:            row.FailedAt,
		})
	}
	return out, nil
}

func (s *ShipmentService) ListShipmentRecipientsByUser(ctx context.Context, ownerUserID uuid.UUID, shipmentID uuid.UUID) ([]ShipmentRecipientSummaryView, error) {
	detail, err := s.GetShipmentDetailByUser(ctx, ownerUserID, shipmentID)
	if err != nil {
		return nil, err
	}
	return detail.RecipientSummaries, nil
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
