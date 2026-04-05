package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/example/vaultsend/internal/queue"
	"github.com/example/vaultsend/internal/store"
	"github.com/google/uuid"
)

const maxResendRecipientSelection = 100

type ResendShipmentInput struct {
	OwnerUserID   uuid.UUID
	ShipmentID    uuid.UUID
	RecipientIDs  []uuid.UUID
	RequestAtTime time.Time
}

type ResendShipmentOutput struct {
	ShipmentID            uuid.UUID `json:"shipment_id"`
	ResentRecipientCount  int       `json:"resent_recipient_count"`
	SkippedRecipientCount int       `json:"skipped_recipient_count"`
	SkippedReasons        []string  `json:"skipped_reasons"`
	QueuedAt              time.Time `json:"queued_at"`
}

// ResendShipmentNotification は recipient_restricted shipment の通知を再送する。
// MVP では「既にダウンロード済み recipient でも再送可」とし、将来条件分岐を追加しやすい構造を維持する。
func (s *ShipmentService) ResendShipmentNotification(ctx context.Context, in ResendShipmentInput) (ResendShipmentOutput, error) {
	if in.OwnerUserID == uuid.Nil {
		return ResendShipmentOutput{}, &APIError{Status: 401, Code: "unauthorized", Message: "ログインが必要です"}
	}
	if in.ShipmentID == uuid.Nil {
		return ResendShipmentOutput{}, &APIError{Status: 400, Code: "invalid_shipment_id", Message: "shipment id が不正です"}
	}
	if s.Queue == nil {
		return ResendShipmentOutput{}, fmt.Errorf("mail queue is not configured")
	}

	shipment, err := s.Store.GetShipment(ctx, in.ShipmentID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ResendShipmentOutput{}, &APIError{Status: 404, Code: "shipment_not_found", Message: "shipment が見つかりません"}
		}
		return ResendShipmentOutput{}, fmt.Errorf("get shipment: %w", err)
	}
	if s.Org != nil {
		if err := s.Org.AuthorizeShipmentAction(ctx, in.OwnerUserID, shipment, "resend"); err != nil {
			return ResendShipmentOutput{}, err
		}
	} else if shipment.OwnerUserID == nil || *shipment.OwnerUserID != in.OwnerUserID {
		return ResendShipmentOutput{}, &APIError{Status: 403, Code: "forbidden", Message: "他ユーザーの shipment は再送できません"}
	}
	if shipment.ShareMode != "recipient_restricted" {
		return ResendShipmentOutput{}, &APIError{Status: 409, Code: "resend_not_supported_share_mode", Message: "recipient_restricted のみ再送できます"}
	}
	if shipment.Status != "sent" && shipment.Status != "ready" {
		return ResendShipmentOutput{}, &APIError{Status: 409, Code: "invalid_shipment_status", Message: "この shipment は再送できない状態です"}
	}
	if shipment.DeletedAt != nil || shipment.Status == "deleted" {
		return ResendShipmentOutput{}, &APIError{Status: 409, Code: "shipment_deleted", Message: "削除済み shipment は再送できません"}
	}
	if shipment.RevokedAt != nil || shipment.Status == "revoked" {
		return ResendShipmentOutput{}, &APIError{Status: 409, Code: "shipment_revoked", Message: "取り消し済み shipment は再送できません"}
	}
	now := time.Now().UTC()
	if !in.RequestAtTime.IsZero() {
		now = in.RequestAtTime.UTC()
	}
	if shipment.ExpiresAt.Before(now) {
		return ResendShipmentOutput{}, &APIError{Status: 409, Code: "shipment_expired", Message: "期限切れ shipment は再送できません"}
	}

	targetRecipients, skippedReasons, err := s.resolveResendTargets(ctx, shipment.ID, in.RecipientIDs)
	if err != nil {
		return ResendShipmentOutput{}, err
	}
	if len(targetRecipients) == 0 {
		return ResendShipmentOutput{}, &APIError{Status: 404, Code: "recipient_not_found", Message: "再送対象 recipient が見つかりません"}
	}

	resent := 0
	skipped := 0
	for _, rc := range targetRecipients {
		rawToken := generateRawToken()
		if err := s.Store.CreateAccessToken(ctx, shipment.ID, store.CreateAccessTokenParams{
			RecipientID: &rc.ID,
			TokenType:   "download_access",
			TokenHash:   hashToken(rawToken),
			ExpiresAt:   shipment.ExpiresAt,
			MaxUses:     shipment.MaxDownloads,
			Status:      "active",
		}); err != nil {
			return ResendShipmentOutput{}, fmt.Errorf("create access token for resend: %w", err)
		}
		queuedAt := now
		ev, err := s.Store.CreateNotificationEvent(ctx, store.CreateNotificationEventParams{
			ShipmentID:  shipment.ID,
			RecipientID: rc.ID,
			EventType:   "resend",
			Status:      "queued",
			QueuedAt:    &queuedAt,
		})
		if err != nil {
			return ResendShipmentOutput{}, fmt.Errorf("create notification event: %w", err)
		}

		msg := queue.MailNotification{
			ShipmentID:        shipment.ID,
			RecipientID:       rc.ID,
			NotificationEvent: &ev.ID,
			NotificationType:  "resend",
			Email:             rc.Email,
			Token:             rawToken,
			Subject:           shipment.Title,
			Message:           shipment.Message,
			ExpiresAt:         &shipment.ExpiresAt,
		}
		if err := s.Queue.EnqueueMail(ctx, msg); err != nil {
			return ResendShipmentOutput{}, fmt.Errorf("enqueue resend mail notification: %w", err)
		}
		resent++
	}

	return ResendShipmentOutput{
		ShipmentID:            shipment.ID,
		ResentRecipientCount:  resent,
		SkippedRecipientCount: skipped,
		SkippedReasons:        skippedReasons,
		QueuedAt:              now,
	}, nil
}

func (s *ShipmentService) resolveResendTargets(ctx context.Context, shipmentID uuid.UUID, recipientIDs []uuid.UUID) ([]store.Recipient, []string, error) {
	if len(recipientIDs) == 0 {
		recipients, err := s.Store.GetRecipientsByShipmentID(ctx, shipmentID)
		if err != nil {
			return nil, nil, fmt.Errorf("get recipients by shipment: %w", err)
		}
		return recipients, nil, nil
	}
	if len(recipientIDs) > maxResendRecipientSelection {
		return nil, nil, &APIError{Status: 422, Code: "recipient_ids_limit_exceeded", Message: "recipient_ids が上限を超えています"}
	}

	dedupIDs := dedupeUUIDs(recipientIDs)
	recipients, err := s.Store.ListRecipientsByIDsAndShipmentID(ctx, shipmentID, dedupIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("list recipients by ids and shipment: %w", err)
	}
	if len(recipients) != len(dedupIDs) {
		return nil, nil, &APIError{Status: 404, Code: "recipient_not_found", Message: "shipment に属さない recipient が含まれています"}
	}
	// TODO(将来): ここに「未ダウンロード recipient のみ再送」条件を追加する。
	return recipients, nil, nil
}
