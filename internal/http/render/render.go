package render

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse は設計書の共通エラー形式に合わせたレスポンス。
type ErrorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

func JSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func Error(w http.ResponseWriter, status int, code, message, requestID string) {
	JSON(w, status, ErrorResponse{
		Code:      code,
		Message:   message,
		RequestID: requestID,
	})
}
