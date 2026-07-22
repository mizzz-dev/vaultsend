package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/example/vaultsend/internal/http/render"
	"github.com/example/vaultsend/internal/service"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

const accessGrantCookiePrefix = "vaultsend_access_grant_"

type AccessHandler struct {
	Service        *service.AccessService
	CookieDomain   string
	CookieSecure   bool
	CookieSameSite http.SameSite
}

func (h AccessHandler) InspectAccess(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if !validAccessToken(token) {
		render.Error(w, http.StatusBadRequest, "invalid_token", "token が不正です", chimw.GetReqID(r.Context()))
		return
	}
	out, err := h.Service.InspectAccessWithGrant(r.Context(), token, readAccessGrantCookie(r, token))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusOK, out)
}

func (h AccessHandler) VerifyAccess(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if !validAccessToken(token) {
		render.Error(w, http.StatusBadRequest, "invalid_token", "token が不正です", chimw.GetReqID(r.Context()))
		return
	}
	var req struct {
		Password *string `json:"password"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		render.Error(w, http.StatusBadRequest, "invalid_request", "不正なJSONです", chimw.GetReqID(r.Context()))
		return
	}
	if req.Password != nil && len(*req.Password) > 256 {
		render.Error(w, http.StatusBadRequest, "invalid_password", "password が長すぎます", chimw.GetReqID(r.Context()))
		return
	}
	out, err := h.Service.VerifyAccess(r.Context(), service.VerifyAccessInput{Token: token, Password: req.Password})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if out.Grant != "" && out.ExpiresAt != nil {
		h.setAccessGrantCookie(w, token, out.Grant, *out.ExpiresAt)
	}
	render.JSON(w, http.StatusOK, out)
}

func (h AccessHandler) GenerateDownloadURL(w http.ResponseWriter, r *http.Request) {
	fileID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid_file_id", "file id が不正です", chimw.GetReqID(r.Context()))
		return
	}
	token := r.URL.Query().Get("access_token")
	if !validAccessToken(token) {
		render.Error(w, http.StatusBadRequest, "invalid_token", "access_token が不正です", chimw.GetReqID(r.Context()))
		return
	}
	out, err := h.Service.GenerateDownloadURL(r.Context(), service.DownloadURLInput{
		Token:       token,
		AccessGrant: readAccessGrantCookie(r, token),
		FileID:      fileID,
		IPAddress:   clientIP(r),
		UserAgent:   r.UserAgent(),
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusOK, out)
}

func (h AccessHandler) setAccessGrantCookie(w http.ResponseWriter, token, grant string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     accessGrantCookieName(token),
		Value:    grant,
		Path:     "/",
		Domain:   h.CookieDomain,
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   h.CookieSecure,
		SameSite: h.cookieSameSite(),
	})
}

func (h AccessHandler) cookieSameSite() http.SameSite {
	if h.CookieSameSite == 0 {
		return http.SameSiteLaxMode
	}
	return h.CookieSameSite
}

func readAccessGrantCookie(r *http.Request, token string) string {
	cookie, err := r.Cookie(accessGrantCookieName(token))
	if err != nil || cookie == nil {
		return ""
	}
	return cookie.Value
}

func accessGrantCookieName(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return accessGrantCookiePrefix + hex.EncodeToString(sum[:8])
}

func validAccessToken(token string) bool {
	trimmed := strings.TrimSpace(token)
	return trimmed != "" && len(trimmed) <= 256
}

func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		return strings.TrimSpace(strings.Split(v, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
