package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/example/vaultsend/internal/store"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	defaultAuthMinPasswordLength = 8
)

type AuthStore interface {
	CreateUser(ctx context.Context, arg store.CreateUserParams) (store.User, error)
	GetUserByEmail(ctx context.Context, emailNormalized string) (store.User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (store.User, error)
	CreateSession(ctx context.Context, arg store.CreateSessionParams) (store.Session, error)
	GetSessionByHash(ctx context.Context, tokenHash string) (store.Session, error)
	GetSessionByHashWithUser(ctx context.Context, tokenHash string) (store.SessionWithUser, error)
	RevokeSession(ctx context.Context, tokenHash string) error
	UpdateSessionLastUsed(ctx context.Context, tokenHash string, lastUsedAt time.Time) error
}

type AuthService struct {
	Store      AuthStore
	SessionTTL time.Duration
	Now        func() time.Time
}

type AuthUser struct {
	ID              uuid.UUID  `json:"id"`
	Email           string     `json:"email"`
	DisplayName     *string    `json:"display_name,omitempty"`
	Status          string     `json:"status"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type RegisterInput struct {
	Email       string
	Password    string
	DisplayName *string
}

type LoginInput struct {
	Email     string
	Password  string
	UserAgent *string
	IPHash    *string
}

type RegisterOutput struct {
	User         AuthUser
	SessionToken string
	ExpiresAt    time.Time
}

type LoginOutput struct {
	User         AuthUser
	SessionToken string
	ExpiresAt    time.Time
}

func (s *AuthService) Register(ctx context.Context, in RegisterInput) (RegisterOutput, error) {
	emailNormalized, err := normalizeAuthEmail(in.Email)
	if err != nil {
		return RegisterOutput{}, &APIError{Status: 400, Code: "invalid_email", Message: "email の形式が不正です"}
	}
	if len(in.Password) < defaultAuthMinPasswordLength {
		return RegisterOutput{}, &APIError{Status: 400, Code: "invalid_password", Message: "password は8文字以上必要です"}
	}
	if in.DisplayName != nil && len([]rune(strings.TrimSpace(*in.DisplayName))) > 80 {
		return RegisterOutput{}, &APIError{Status: 400, Code: "invalid_display_name", Message: "display_name が長すぎます"}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return RegisterOutput{}, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.Store.CreateUser(ctx, store.CreateUserParams{
		Email:           strings.TrimSpace(in.Email),
		EmailNormalized: emailNormalized,
		PasswordHash:    string(hash),
		DisplayName:     normalizeOptionalString(in.DisplayName),
		Status:          "active",
	})
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			return RegisterOutput{}, &APIError{Status: 409, Code: "email_already_exists", Message: "このemailは既に登録済みです"}
		}
		return RegisterOutput{}, fmt.Errorf("create user: %w", err)
	}

	sessionToken, sessionHash, err := generateSessionToken()
	if err != nil {
		return RegisterOutput{}, fmt.Errorf("generate session token: %w", err)
	}
	expiresAt := s.now().Add(s.sessionTTL())
	if _, err := s.Store.CreateSession(ctx, store.CreateSessionParams{UserID: user.ID, TokenHash: sessionHash, ExpiresAt: expiresAt}); err != nil {
		return RegisterOutput{}, fmt.Errorf("create session: %w", err)
	}

	return RegisterOutput{User: toAuthUser(user), SessionToken: sessionToken, ExpiresAt: expiresAt}, nil
}

func (s *AuthService) Login(ctx context.Context, in LoginInput) (LoginOutput, error) {
	emailNormalized, err := normalizeAuthEmail(in.Email)
	if err != nil {
		return LoginOutput{}, &APIError{Status: 401, Code: "invalid_credentials", Message: "email または password が正しくありません"}
	}
	user, err := s.Store.GetUserByEmail(ctx, emailNormalized)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return LoginOutput{}, &APIError{Status: 401, Code: "invalid_credentials", Message: "email または password が正しくありません"}
		}
		return LoginOutput{}, fmt.Errorf("get user by email: %w", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password)) != nil {
		return LoginOutput{}, &APIError{Status: 401, Code: "invalid_credentials", Message: "email または password が正しくありません"}
	}

	sessionToken, sessionHash, err := generateSessionToken()
	if err != nil {
		return LoginOutput{}, fmt.Errorf("generate session token: %w", err)
	}
	expiresAt := s.now().Add(s.sessionTTL())
	if _, err := s.Store.CreateSession(ctx, store.CreateSessionParams{
		UserID:    user.ID,
		TokenHash: sessionHash,
		ExpiresAt: expiresAt,
		UserAgent: normalizeOptionalString(in.UserAgent),
		IPHash:    normalizeOptionalString(in.IPHash),
	}); err != nil {
		return LoginOutput{}, fmt.Errorf("create session: %w", err)
	}

	return LoginOutput{User: toAuthUser(user), SessionToken: sessionToken, ExpiresAt: expiresAt}, nil
}

func (s *AuthService) Logout(ctx context.Context, sessionToken string) error {
	if strings.TrimSpace(sessionToken) == "" {
		return &APIError{Status: 401, Code: "unauthorized", Message: "ログインが必要です"}
	}
	if err := s.Store.RevokeSession(ctx, hashSHA256(sessionToken)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return &APIError{Status: 401, Code: "unauthorized", Message: "有効なセッションがありません"}
		}
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (s *AuthService) Me(ctx context.Context, sessionToken string) (AuthUser, error) {
	if strings.TrimSpace(sessionToken) == "" {
		return AuthUser{}, &APIError{Status: 401, Code: "unauthorized", Message: "ログインが必要です"}
	}
	row, err := s.Store.GetSessionByHashWithUser(ctx, hashSHA256(sessionToken))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return AuthUser{}, &APIError{Status: 401, Code: "unauthorized", Message: "セッションが無効です"}
		}
		return AuthUser{}, fmt.Errorf("get session with user: %w", err)
	}
	now := s.now()
	if row.Session.RevokedAt != nil || row.Session.ExpiresAt.Before(now) {
		return AuthUser{}, &APIError{Status: 401, Code: "unauthorized", Message: "セッションが期限切れです"}
	}
	_ = s.Store.UpdateSessionLastUsed(ctx, hashSHA256(sessionToken), now)
	return toAuthUser(row.User), nil
}

func (s *AuthService) SessionFromToken(ctx context.Context, sessionToken string) (AuthUser, error) {
	return s.Me(ctx, sessionToken)
}

func (s *AuthService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *AuthService) sessionTTL() time.Duration {
	if s.SessionTTL > 0 {
		return s.SessionTTL
	}
	return 7 * 24 * time.Hour
}

func toAuthUser(u store.User) AuthUser {
	return AuthUser{ID: u.ID, Email: u.Email, DisplayName: u.DisplayName, Status: u.Status, EmailVerifiedAt: u.EmailVerifiedAt, CreatedAt: u.CreatedAt}
}

func normalizeAuthEmail(v string) (string, error) {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" || len(v) > 320 {
		return "", errors.New("invalid email")
	}
	if _, err := mail.ParseAddress(v); err != nil {
		return "", err
	}
	return v, nil
}

func normalizeOptionalString(v *string) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return nil
	}
	return &s
}

func generateSessionToken() (raw string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(buf)
	return raw, hashSHA256(raw), nil
}

func hashSHA256(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
