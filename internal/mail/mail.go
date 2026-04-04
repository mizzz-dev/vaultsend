package mail

import "context"

// Body はメール本文（テキスト/HTML）を表現する。
type Body struct {
	Text string
	HTML string
}

// Sender は SES 等の実装差し替えを可能にする。
type Sender interface {
	SendEmail(ctx context.Context, to, subject string, body Body) error
}
