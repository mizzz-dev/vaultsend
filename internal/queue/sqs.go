package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type sqsAPI interface {
	SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
	ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
}

// SQSQueue は SQS ベースのキュー実装。
type SQSQueue struct {
	client   sqsAPI
	queueURL string
}

func NewSQSQueue(client sqsAPI, queueURL string) *SQSQueue {
	return &SQSQueue{client: client, queueURL: queueURL}
}

func (q *SQSQueue) EnqueueMail(ctx context.Context, msg MailNotification) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal mail notification: %w", err)
	}
	_, err = q.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(q.queueURL),
		MessageBody: aws.String(string(body)),
	})
	if err != nil {
		return fmt.Errorf("sqs send message: %w", err)
	}
	return nil
}

func (q *SQSQueue) Receive(ctx context.Context, maxMessages int32, waitSeconds int32) ([]ReceivedMessage, error) {
	out, err := q.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(q.queueURL),
		MaxNumberOfMessages: maxMessages,
		WaitTimeSeconds:     waitSeconds,
	})
	if err != nil {
		return nil, fmt.Errorf("sqs receive message: %w", err)
	}
	messages := make([]ReceivedMessage, 0, len(out.Messages))
	for _, m := range out.Messages {
		messages = append(messages, ReceivedMessage{
			MessageID:     aws.ToString(m.MessageId),
			Body:          aws.ToString(m.Body),
			ReceiptHandle: aws.ToString(m.ReceiptHandle),
		})
	}
	return messages, nil
}

func (q *SQSQueue) Delete(ctx context.Context, receiptHandle string) error {
	_, err := q.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(q.queueURL),
		ReceiptHandle: aws.String(receiptHandle),
	})
	if err != nil {
		return fmt.Errorf("sqs delete message: %w", err)
	}
	return nil
}
