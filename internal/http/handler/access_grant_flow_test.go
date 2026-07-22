package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/vaultsend/internal/service"
	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
)

func TestAccessGrantFlow_VerifyCookieRestoresInspectState(t *testing.T) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	hash := string(passwordHash)
	svc := &service.AccessService{
		Store:             &fakeAccessHandlerStore{passwordHash: &hash},
		AccessGrantSecret: "handler-test-access-grant-secret-32-bytes",
		AccessGrantTTL:    5 * time.Minute,
	}
	h := AccessHandler{Service: svc, CookieSameSite: http.SameSiteLaxMode}
	r := chi.NewRouter()
	r.Post("/v1/access/{token}/verify", h.VerifyAccess)
	r.Get("/v1/access/{token}", h.InspectAccess)

	body, _ := json.Marshal(map[string]string{"password": "correct-password"})
	verifyReq := httptest.NewRequest(http.MethodPost, "/v1/access/raw-token/verify", bytes.NewReader(body))
	verifyRes := httptest.NewRecorder()
	r.ServeHTTP(verifyRes, verifyReq)
	if verifyRes.Code != http.StatusOK {
		t.Fatalf("verify status=%d body=%s", verifyRes.Code, verifyRes.Body.String())
	}
	cookies := verifyRes.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected grant cookie got=%d", len(cookies))
	}

	inspectReq := httptest.NewRequest(http.MethodGet, "/v1/access/raw-token", nil)
	inspectReq.AddCookie(cookies[0])
	inspectRes := httptest.NewRecorder()
	r.ServeHTTP(inspectRes, inspectReq)
	if inspectRes.Code != http.StatusOK {
		t.Fatalf("inspect status=%d body=%s", inspectRes.Code, inspectRes.Body.String())
	}
	var payload struct {
		RequiresPassword bool `json:"requires_password"`
		Verified         bool `json:"verified"`
	}
	if err := json.Unmarshal(inspectRes.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.RequiresPassword || !payload.Verified {
		t.Fatalf("expected protected and verified: %+v", payload)
	}
}
