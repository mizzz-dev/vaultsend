package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/example/vaultsend/internal/http/render"
	"github.com/example/vaultsend/internal/service"
	chimw "github.com/go-chi/chi/v5/middleware"
)

const SessionCookieName = "vs_session"

type ctxKey string

const authUserKey ctxKey = "auth_user"

type SessionAuthenticator interface {
	SessionFromToken(ctx context.Context, sessionToken string) (service.AuthUser, error)
}

func OptionalAuth(auth SessionAuthenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if auth == nil {
				next.ServeHTTP(w, r)
				return
			}
			cookie, err := r.Cookie(SessionCookieName)
			if err == nil && strings.TrimSpace(cookie.Value) != "" {
				if user, authErr := auth.SessionFromToken(ctx, cookie.Value); authErr == nil {
					ctx = WithAuthUser(ctx, user)
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireAuth(auth SessionAuthenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(SessionCookieName)
			if err != nil || strings.TrimSpace(cookie.Value) == "" {
				render.Error(w, http.StatusUnauthorized, "unauthorized", "ログインが必要です", chimw.GetReqID(r.Context()))
				return
			}
			user, authErr := auth.SessionFromToken(r.Context(), cookie.Value)
			if authErr != nil {
				render.Error(w, http.StatusUnauthorized, "unauthorized", "セッションが無効です", chimw.GetReqID(r.Context()))
				return
			}
			next.ServeHTTP(w, r.WithContext(WithAuthUser(r.Context(), user)))
		})
	}
}

func WithAuthUser(ctx context.Context, user service.AuthUser) context.Context {
	return context.WithValue(ctx, authUserKey, user)
}

func AuthUserFromContext(ctx context.Context) (*service.AuthUser, bool) {
	v := ctx.Value(authUserKey)
	if v == nil {
		return nil, false
	}
	user, ok := v.(service.AuthUser)
	if !ok {
		return nil, false
	}
	return &user, true
}
