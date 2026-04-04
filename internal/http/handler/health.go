package handler

import (
	"net/http"

	"github.com/example/vaultsend/internal/http/render"
)

// Health は liveness/readiness の最小エンドポイント。
func Health(w http.ResponseWriter, r *http.Request) {
	render.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
