package mail

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

type sesAPI interface {
	SendEmail(ctx context.Context, params *sesv2.SendEmailInput, optFns ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error)
}

// SESMailer は Amazon SES(v2) でメール送信を行う。
type SESMailer struct {
	client    sesAPI
	fromEmail string
}

func NewSESMailer(client sesAPI, fromEmail string) *SESMailer {
	return &SESMailer{client: client, fromEmail: fromEmail}
}

func (m *SESMailer) SendEmail(ctx context.Context, to, subject string, body Body) error {
	_, err := m.client.SendEmail(ctx, &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(m.fromEmail),
		Destination: &types.Destination{
			ToAddresses: []string{to},
		},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{Data: aws.String(subject), Charset: aws.String("UTF-8")},
				Body: &types.Body{
					Text: &types.Content{Data: aws.String(body.Text), Charset: aws.String("UTF-8")},
					Html: &types.Content{Data: aws.String(body.HTML), Charset: aws.String("UTF-8")},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("ses send email: %w", err)
	}
	return nil
}
