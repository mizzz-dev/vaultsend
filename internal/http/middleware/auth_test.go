package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/example/vaultsend/internal/service"
	"github.com/google/uuid"
)

type fakeSessionAuth struct {
	user service.AuthUser
	err  error
}

func (f *fakeSessionAuth) SessionFromToken(ctx context.Context, sessionToken string) (service.AuthUser, error) {
	if f.err != nil {
		return service.AuthUser{}, f.err
	}
	return f.user, nil
}

func TestOptionalAuthSetsUser(t *testing.T) {
	mw := OptionalAuth(&fakeSessionAuth{user: service.AuthUser{ID: uuid.New(), Email: "a@example.com"}})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := AuthUserFromContext(r.Context()); !ok {
			t.Fatal("user should exist in context")
		}
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "token"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestRequireAuthRejectsInvalid(t *testing.T) {
	mw := RequireAuth(&fakeSessionAuth{err: context.Canceled})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "bad"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
}
