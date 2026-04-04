package mail

import "context"

// Sender は SES 送信基盤の差し替えを可能にするためのインターフェース。
// TODO: 次PRでテンプレートID・再送制御を含む詳細APIに拡張する。
type Sender interface {
	Send(ctx context.Context, to []string, subject, body string) error
}
