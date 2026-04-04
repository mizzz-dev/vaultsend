package queue

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// MailNotification は受信者通知メールを非同期送信するためのキューイベント。
type MailNotification struct {
	ShipmentID        uuid.UUID  `json:"shipment_id"`
	RecipientID       uuid.UUID  `json:"recipient_id"`
	NotificationEvent *int64     `json:"notification_event_id,omitempty"`
	NotificationType  string     `json:"notification_type,omitempty"`
	Email             string     `json:"email"`
	Token             string     `json:"token"`
	Subject           string     `json:"subject"`
	Message           *string    `json:"message,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
}

// Enqueuer はメール送信キュー投入の抽象。
type Enqueuer interface {
	EnqueueMail(ctx context.Context, msg MailNotification) error
}

// Consumer はワーカーが利用するメッセージ取得/ACKの抽象。
type Consumer interface {
	Receive(ctx context.Context, maxMessages int32, waitSeconds int32) ([]ReceivedMessage, error)
	Delete(ctx context.Context, receiptHandle string) error
}

// ReceivedMessage はキューから取得した1件のメッセージ。
type ReceivedMessage struct {
	MessageID     string
	Body          string
	ReceiptHandle string
}
