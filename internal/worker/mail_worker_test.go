package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/example/vaultsend/internal/mail"
	"github.com/example/vaultsend/internal/queue"
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
	payload, _ := json.Marshal(queue.MailNotification{ShipmentID: uuid.New(), RecipientID: uuid.New(), Email: "a@example.com", Token: "token", Subject: "件名", ExpiresAt: &expires})
	consumer := &fakeConsumer{messages: []queue.ReceivedMessage{{MessageID: "m1", Body: string(payload), ReceiptHandle: "rh1"}}}
	sender := &fakeSender{}

	w := &MailWorker{Queue: consumer, Mailer: sender, FrontendURL: "https://frontend.example.com", MaxMessages: 1, WaitSeconds: 1}
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
	payload, _ := json.Marshal(queue.MailNotification{ShipmentID: uuid.New(), RecipientID: uuid.New(), Email: "a@example.com", Token: "token", Subject: "件名"})
	consumer := &fakeConsumer{}
	w := &MailWorker{Queue: consumer, Mailer: &fakeSender{err: errors.New("ses failed")}, FrontendURL: "https://frontend.example.com"}
	err := w.handleMessage(context.Background(), queue.ReceivedMessage{MessageID: "m1", Body: string(payload), ReceiptHandle: "rh1"})
	if err == nil {
		t.Fatal("expected send error")
	}
	if len(consumer.deleted) != 0 {
		t.Fatalf("expected not delete on send error got=%d", len(consumer.deleted))
	}
}
