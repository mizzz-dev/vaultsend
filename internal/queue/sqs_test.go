package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"
)

type fakeSQSClient struct {
	sendInput   *sqs.SendMessageInput
	receiveOut  *sqs.ReceiveMessageOutput
	receiveErr  error
	deleteInput *sqs.DeleteMessageInput
	sendErr     error
	deleteErr   error
}

func (f *fakeSQSClient) SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	f.sendInput = params
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	return &sqs.SendMessageOutput{}, nil
}

func (f *fakeSQSClient) ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	if f.receiveErr != nil {
		return nil, f.receiveErr
	}
	if f.receiveOut != nil {
		return f.receiveOut, nil
	}
	return &sqs.ReceiveMessageOutput{}, nil
}

func (f *fakeSQSClient) DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
	f.deleteInput = params
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	return &sqs.DeleteMessageOutput{}, nil
}

func TestSQSQueue_EnqueueMail(t *testing.T) {
	client := &fakeSQSClient{}
	q := NewSQSQueue(client, "https://sqs.local/queue")
	expires := time.Now().UTC().Add(time.Hour)
	err := q.EnqueueMail(context.Background(), MailNotification{
		ShipmentID:  uuid.New(),
		RecipientID: uuid.New(),
		Email:       "a@example.com",
		Token:       "token",
		Subject:     "subject",
		ExpiresAt:   &expires,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := aws.ToString(client.sendInput.QueueUrl); got != "https://sqs.local/queue" {
		t.Fatalf("unexpected queue url: %s", got)
	}
	if aws.ToString(client.sendInput.MessageBody) == "" {
		t.Fatal("message body should not be empty")
	}
}

func TestSQSQueue_ReceiveAndDelete(t *testing.T) {
	client := &fakeSQSClient{receiveOut: &sqs.ReceiveMessageOutput{Messages: []types.Message{{MessageId: aws.String("m1"), Body: aws.String("{}"), ReceiptHandle: aws.String("rh1")}}}}
	q := NewSQSQueue(client, "https://sqs.local/queue")
	msgs, err := q.Receive(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(msgs) != 1 || msgs[0].MessageID != "m1" {
		t.Fatalf("unexpected messages: %+v", msgs)
	}
	if err := q.Delete(context.Background(), "rh1"); err != nil {
		t.Fatalf("unexpected delete err: %v", err)
	}
}

func TestSQSQueue_EnqueueMail_Error(t *testing.T) {
	q := NewSQSQueue(&fakeSQSClient{sendErr: errors.New("down")}, "https://sqs.local/queue")
	if err := q.EnqueueMail(context.Background(), MailNotification{}); err == nil {
		t.Fatal("expected send error")
	}
}
