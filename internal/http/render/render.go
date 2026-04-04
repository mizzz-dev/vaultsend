package render

import (
	"encoding/json"
	"net/http"

	"github.com/example/vaultsend/internal/service"
)

// ErrorResponse は設計書の共通エラー形式に合わせたレスポンス。
type ErrorResponse struct {
	Error           string `json:"error,omitempty"`
	Code            string `json:"code"`
	Message         string `json:"message"`
	RequestID       string `json:"request_id,omitempty"`
	UpgradeRequired bool   `json:"upgrade_required,omitempty"`
	UpgradeURL      string `json:"upgrade_url,omitempty"`
	RecommendedPlan string `json:"recommended_plan,omitempty"`
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

func ServiceError(w http.ResponseWriter, err *service.APIError, requestID string) {
	JSON(w, err.Status, ErrorResponse{
		Error:           err.Error,
		Code:            err.Code,
		Message:         err.Message,
		RequestID:       requestID,
		UpgradeRequired: err.UpgradeRequired,
		UpgradeURL:      err.UpgradeURL,
		RecommendedPlan: err.RecommendedPlan,
	})
}
