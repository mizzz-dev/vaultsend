package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/example/vaultsend/internal/mail"
	"github.com/example/vaultsend/internal/queue"
	"github.com/example/vaultsend/internal/store"
	"github.com/google/uuid"
)

type fakeConsumer struct {
	messages []queue.ReceivedMessage
	deleted  []string
}

func (f *fakeConsumer) Receive(ctx context.Context, maxMessages int32, waitSeconds int32) ([]queue.ReceivedMessage, error) {
	if len(f.messages) == 0 {
		return nil, ctx.Err()
	}
	msgs := f.messages
	f.messages = nil
	return msgs, nil
}

func (f *fakeConsumer) Delete(ctx context.Context, receiptHandle string) error {
	f.deleted = append(f.deleted, receiptHandle)
	return nil
}

type fakeSender struct {
	calls int
	err   error
}

type fakeNotificationEventStore struct {
	updates []store.UpdateNotificationEventStatusParams
}

func (f *fakeNotificationEventStore) UpdateNotificationEventStatus(ctx context.Context, arg store.UpdateNotificationEventStatusParams) error {
	f.updates = append(f.updates, arg)
	return nil
}

func (f *fakeSender) SendEmail(ctx context.Context, to, subject string, body mail.Body) error {
	f.calls++
	if f.err != nil {
		return f.err
	}
	if to == "" || subject == "" || body.Text == "" || body.HTML == "" {
		return errors.New("invalid mail")
	}
	return nil
}

func TestMailWorker_Run_Success(t *testing.T) {
	expires := time.Now().UTC().Add(24 * time.Hour)
	eventID := int64(10)
	payload, _ := json.Marshal(queue.MailNotification{ShipmentID: uuid.New(), RecipientID: uuid.New(), NotificationEvent: &eventID, Email: "a@example.com", Token: "token", Subject: "件名", ExpiresAt: &expires})
	consumer := &fakeConsumer{messages: []queue.ReceivedMessage{{MessageID: "m1", Body: string(payload), ReceiptHandle: "rh1"}}}
	sender := &fakeSender{}
	eventStore := &fakeNotificationEventStore{}

	w := &MailWorker{Queue: consumer, Mailer: sender, EventStore: eventStore, FrontendURL: "https://frontend.example.com", MaxMessages: 1, WaitSeconds: 1}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	if err := w.Run(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("expected send once got=%d", sender.calls)
	}
	if len(consumer.deleted) != 1 {
		t.Fatalf("expected delete once got=%d", len(consumer.deleted))
	}
	if len(eventStore.updates) != 1 || eventStore.updates[0].Status != "sent" {
		t.Fatalf("expected sent update once got=%+v", eventStore.updates)
	}
}

func TestMailWorker_handleMessage_DecodeErrorDeletes(t *testing.T) {
	consumer := &fakeConsumer{}
	w := &MailWorker{Queue: consumer, Mailer: &fakeSender{}, FrontendURL: "https://frontend.example.com"}
	err := w.handleMessage(context.Background(), queue.ReceivedMessage{MessageID: "m1", Body: "{", ReceiptHandle: "rh1"})
	if err == nil {
		t.Fatal("expected decode error")
	}
	if len(consumer.deleted) != 1 {
		t.Fatalf("expected poison delete once got=%d", len(consumer.deleted))
	}
}

func TestMailWorker_handleMessage_SendErrorKeepsMessage(t *testing.T) {
	eventID := int64(20)
	payload, _ := json.Marshal(queue.MailNotification{ShipmentID: uuid.New(), RecipientID: uuid.New(), NotificationEvent: &eventID, Email: "a@example.com", Token: "token", Subject: "件名"})
	consumer := &fakeConsumer{}
	eventStore := &fakeNotificationEventStore{}
	w := &MailWorker{Queue: consumer, Mailer: &fakeSender{err: errors.New("ses failed")}, EventStore: eventStore, FrontendURL: "https://frontend.example.com"}
	err := w.handleMessage(context.Background(), queue.ReceivedMessage{MessageID: "m1", Body: string(payload), ReceiptHandle: "rh1"})
	if err == nil {
		t.Fatal("expected send error")
	}
	if len(consumer.deleted) != 0 {
		t.Fatalf("expected not delete on send error got=%d", len(consumer.deleted))
	}
	if len(eventStore.updates) != 1 || eventStore.updates[0].Status != "failed" {
		t.Fatalf("expected failed update once got=%+v", eventStore.updates)
	}
}
