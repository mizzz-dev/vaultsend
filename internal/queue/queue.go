package queue

import "context"

// Publisher は SQS 等にイベント投入するための最小インターフェース。
// TODO: 次PRで ShipmentSent などのイベント型を追加する。
type Publisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}
