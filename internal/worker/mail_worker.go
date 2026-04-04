package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/example/vaultsend/internal/mail"
	"github.com/example/vaultsend/internal/queue"
)

// MailWorker は SQS から通知イベントを読み出して SES 送信する。
type MailWorker struct {
	Queue       queue.Consumer
	Mailer      mail.Sender
	FrontendURL string
	MaxMessages int32
	WaitSeconds int32
}

func (w *MailWorker) Run(ctx context.Context) error {
	if w.MaxMessages <= 0 {
		w.MaxMessages = 5
	}
	if w.WaitSeconds <= 0 {
		w.WaitSeconds = 20
	}

	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		messages, err := w.Queue.Receive(ctx, w.MaxMessages, w.WaitSeconds)
		if err != nil {
			return fmt.Errorf("poll mail queue: %w", err)
		}
		for _, m := range messages {
			if err := w.handleMessage(ctx, m); err != nil {
				log.Printf("mail worker: handle message failed id=%s err=%v", m.MessageID, err)
			}
		}
	}
}

func (w *MailWorker) handleMessage(ctx context.Context, m queue.ReceivedMessage) error {
	var payload queue.MailNotification
	if err := json.Unmarshal([]byte(m.Body), &payload); err != nil {
		if delErr := w.Queue.Delete(ctx, m.ReceiptHandle); delErr != nil {
			return fmt.Errorf("decode message: %w; delete poison message: %v", err, delErr)
		}
		return fmt.Errorf("decode message: %w", err)
	}

	body, err := mail.BuildShipmentNotification(w.FrontendURL, payload)
	if err != nil {
		return fmt.Errorf("build mail body: %w", err)
	}
	if err := w.Mailer.SendEmail(ctx, payload.Email, payload.Subject, body); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	if err := w.Queue.Delete(ctx, m.ReceiptHandle); err != nil {
		return fmt.Errorf("delete message: %w", err)
	}
	return nil
}
