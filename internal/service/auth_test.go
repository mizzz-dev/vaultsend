package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/vaultsend/internal/store"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type fakeAuthStore struct {
	createUserErr      error
	getUserByEmailErr  error
	getSessionWithErr  error
	revokeErr          error
	createdUser        store.User
	userByEmail        store.User
	sessionWithUser    store.SessionWithUser
	createSessionCount int
	updatedLastUsed    bool
}

func (f *fakeAuthStore) CreateUser(ctx context.Context, arg store.CreateUserParams) (store.User, error) {
	if f.createUserErr != nil {
		return store.User{}, f.createUserErr
	}
	if f.createdUser.ID == uuid.Nil {
		f.createdUser = store.User{ID: uuid.New(), Email: arg.Email, EmailNormalized: arg.EmailNormalized, PasswordHash: arg.PasswordHash, DisplayName: arg.DisplayName, Status: arg.Status, CreatedAt: time.Now().UTC()}
	}
	return f.createdUser, nil
}
func (f *fakeAuthStore) GetUserByEmail(ctx context.Context, emailNormalized string) (store.User, error) {
	if f.getUserByEmailErr != nil {
		return store.User{}, f.getUserByEmailErr
	}
	return f.userByEmail, nil
}
func (f *fakeAuthStore) GetUserByID(ctx context.Context, id uuid.UUID) (store.User, error) {
	return store.User{}, nil
}
func (f *fakeAuthStore) CreateSession(ctx context.Context, arg store.CreateSessionParams) (store.Session, error) {
	f.createSessionCount++
	return store.Session{ID: uuid.New(), UserID: arg.UserID, TokenHash: arg.TokenHash, ExpiresAt: arg.ExpiresAt}, nil
}
func (f *fakeAuthStore) GetSessionByHash(ctx context.Context, tokenHash string) (store.Session, error) {
	return store.Session{}, nil
}
func (f *fakeAuthStore) GetSessionByHashWithUser(ctx context.Context, tokenHash string) (store.SessionWithUser, error) {
	if f.getSessionWithErr != nil {
		return store.SessionWithUser{}, f.getSessionWithErr
	}
	return f.sessionWithUser, nil
}
func (f *fakeAuthStore) RevokeSession(ctx context.Context, tokenHash string) error {
	return f.revokeErr
}
func (f *fakeAuthStore) UpdateSessionLastUsed(ctx context.Context, tokenHash string, lastUsedAt time.Time) error {
	f.updatedLastUsed = true
	return nil
}

func TestAuthRegisterSuccess(t *testing.T) {
	st := &fakeAuthStore{}
	svc := &AuthService{Store: st, SessionTTL: time.Hour}
	out, err := svc.Register(context.Background(), RegisterInput{Email: "A@Example.com", Password: "password123"})
	if err != nil || out.User.ID == uuid.Nil || out.SessionToken == "" {
		t.Fatalf("unexpected register result err=%v", err)
	}
	if st.createSessionCount != 1 {
		t.Fatal("session should be created")
	}
}

func TestAuthRegisterConflict(t *testing.T) {
	st := &fakeAuthStore{createUserErr: store.ErrConflict}
	svc := &AuthService{Store: st}
	_, err := svc.Register(context.Background(), RegisterInput{Email: "a@example.com", Password: "password123"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 409 {
		t.Fatalf("expected 409 got=%v", err)
	}
}

func TestAuthLoginPasswordMismatch(t *testing.T) {
	h, _ := bcrypt.GenerateFromPassword([]byte("correct-pass"), bcrypt.DefaultCost)
	st := &fakeAuthStore{userByEmail: store.User{ID: uuid.New(), PasswordHash: string(h), Email: "a@example.com", Status: "active"}}
	svc := &AuthService{Store: st}
	_, err := svc.Login(context.Background(), LoginInput{Email: "a@example.com", Password: "wrong"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 401 {
		t.Fatalf("expected 401 got=%v", err)
	}
}

func TestAuthMeInvalidSession(t *testing.T) {
	st := &fakeAuthStore{getSessionWithErr: store.ErrNotFound}
	svc := &AuthService{Store: st}
	_, err := svc.Me(context.Background(), "token")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 401 {
		t.Fatalf("expected 401 got=%v", err)
	}
}

func TestAuthLogout(t *testing.T) {
	st := &fakeAuthStore{}
	svc := &AuthService{Store: st}
	if err := svc.Logout(context.Background(), "token"); err != nil {
		t.Fatalf("unexpected err=%v", err)
	}
}
