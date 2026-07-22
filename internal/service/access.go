package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/example/vaultsend/internal/storage"
	"github.com/example/vaultsend/internal/store"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	defaultDownloadURLTTL = 60 * time.Second
	defaultAccessGrantTTL = 10 * time.Minute
)

type AccessStore interface {
	GetAccessTokenByHash(ctx context.Context, tokenHash string) (store.AccessToken, error)
	GetShipmentByID(ctx context.Context, id uuid.UUID) (store.Shipment, error)
	GetFilesByShipmentID(ctx context.Context, shipmentID uuid.UUID) ([]store.File, error)
	GetFileByID(ctx context.Context, id uuid.UUID) (store.File, error)
	CountDownloadEventsByShipment(ctx context.Context, shipmentID uuid.UUID) (int32, error)
	CreateDownloadEvent(ctx context.Context, arg store.CreateDownloadEventParams) (store.DownloadEvent, error)
	UpdateAccessTokenUsage(ctx context.Context, tokenID uuid.UUID) error
	IncrementShipmentDownloadCount(ctx context.Context, shipmentID uuid.UUID) error
}

type AccessService struct {
	Store             AccessStore
	ObjectStore       storage.ObjectStore
	DownloadURLTTL    time.Duration
	AccessGrantTTL    time.Duration
	AccessGrantSecret string
	Guard             *AccessGuard
}

type AccessInspectOutput struct {
	RequiresPassword bool                     `json:"requires_password"`
	Shipment         AccessShipmentView       `json:"shipment"`
	Files            []CreateShipmentFileView `json:"files"`
}

type AccessShipmentView struct {
	ID               uuid.UUID `json:"id"`
	ShareMode        string    `json:"share_mode"`
	Subject          string    `json:"subject"`
	Message          *string   `json:"message,omitempty"`
	ExpiresAt        time.Time `json:"expires_at"`
	MaxDownloadCount int32     `json:"max_download_count"`
}

type VerifyAccessInput struct {
	Token    string
	Password *string
}

type VerifyAccessOutput struct {
	Granted   bool      `json:"granted"`
	Grant     string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

type DownloadURLInput struct {
	Token       string
	AccessGrant string
	FileID      uuid.UUID
	IPAddress   string
	UserAgent   string
}

type DownloadURLOutput struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *AccessService) InspectAccess(ctx context.Context, rawToken string) (AccessInspectOutput, error) {
	state, err := s.resolveAccessState(ctx, rawToken)
	if err != nil {
		return AccessInspectOutput{}, err
	}
	if state.downloadCount >= state.shipment.MaxDownloads {
		return AccessInspectOutput{}, &APIError{Status: 409, Code: "download_limit_exceeded", Message: "ダウンロード回数制限を超過しています"}
	}

	out := AccessInspectOutput{
		RequiresPassword: state.shipment.PasswordHash != nil,
		Shipment: AccessShipmentView{
			ID:               state.shipment.ID,
			ShareMode:        normalizeShareModeForResponse(state.shipment.ShareMode),
			Subject:          state.shipment.Title,
			Message:          state.shipment.Message,
			ExpiresAt:        state.shipment.ExpiresAt,
			MaxDownloadCount: state.shipment.MaxDownloads,
		},
		Files: make([]CreateShipmentFileView, 0, len(state.files)),
	}
	for _, f := range state.files {
		out.Files = append(out.Files, CreateShipmentFileView{ID: f.ID, OriginalName: f.OriginalName, SizeBytes: f.SizeBytes})
	}
	return out, nil
}

func (s *AccessService) VerifyAccess(ctx context.Context, in VerifyAccessInput) (VerifyAccessOutput, error) {
	if s.guard().VerifyAllowed(in.Token) == false {
		log.Printf("event=verify_locked token_hash=%s", hashToken(in.Token))
		return VerifyAccessOutput{}, &APIError{Status: 429, Code: "verify_locked", Message: "パスワード再試行が上限を超えました。時間をおいて再試行してください"}
	}

	state, err := s.resolveAccessState(ctx, in.Token)
	if err != nil {
		return VerifyAccessOutput{}, err
	}
	if state.shipment.PasswordHash == nil {
		return VerifyAccessOutput{Granted: true}, nil
	}
	if in.Password == nil || strings.TrimSpace(*in.Password) == "" {
		s.guard().RegisterVerifyFailure(in.Token)
		log.Printf("event=verify_failure token_hash=%s reason=password_required", hashToken(in.Token))
		return VerifyAccessOutput{}, &APIError{Status: 400, Code: "password_required", Message: "password が必要です"}
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*state.shipment.PasswordHash), []byte(*in.Password)); err != nil {
		s.guard().RegisterVerifyFailure(in.Token)
		log.Printf("event=verify_failure token_hash=%s reason=invalid_password", hashToken(in.Token))
		return VerifyAccessOutput{}, &APIError{Status: 401, Code: "invalid_password", Message: "password が一致しません"}
	}

	s.guard().ResetVerify(in.Token)
	expiresAt := s.accessGrantExpiresAt(state)
	grant, err := issueAccessGrant(s.AccessGrantSecret, in.Token, expiresAt)
	if err != nil {
		return VerifyAccessOutput{}, fmt.Errorf("issue access grant: %w", err)
	}
	return VerifyAccessOutput{Granted: true, Grant: grant, ExpiresAt: expiresAt}, nil
}

func (s *AccessService) GenerateDownloadURL(ctx context.Context, in DownloadURLInput) (DownloadURLOutput, error) {
	state, err := s.resolveAccessState(ctx, in.Token)
	if err != nil {
		return DownloadURLOutput{}, err
	}
	if state.shipment.PasswordHash != nil {
		if err := validateAccessGrant(s.AccessGrantSecret, in.Token, in.AccessGrant, time.Now().UTC()); err != nil {
			return DownloadURLOutput{}, &APIError{Status: 401, Code: "access_verification_required", Message: "パスワードの再確認が必要です"}
		}
	}

	downloadKey := hashToken(in.Token) + ":" + hashIP(in.IPAddress)
	if !s.guard().AllowDownload(downloadKey) {
		log.Printf("event=download_abuse_block token_hash=%s ip_hash=%s", hashToken(in.Token), hashIP(in.IPAddress))
		return DownloadURLOutput{}, &APIError{Status: 429, Code: "download_rate_limited", Message: "短時間でのダウンロードURL発行回数が多すぎます"}
	}

	file, err := s.Store.GetFileByID(ctx, in.FileID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return DownloadURLOutput{}, &APIError{Status: 404, Code: "file_not_found", Message: "file が見つかりません"}
		}
		return DownloadURLOutput{}, fmt.Errorf("get file by id: %w", err)
	}
	if file.ShipmentID != state.shipment.ID {
		return DownloadURLOutput{}, &APIError{Status: 404, Code: "file_not_found", Message: "token に紐づかない file です"}
	}
	if state.downloadCount >= state.shipment.MaxDownloads {
		s.logDownloadEvent(ctx, file.ID, state, "over_limit", in)
		return DownloadURLOutput{}, &APIError{Status: 409, Code: "download_limit_exceeded", Message: "ダウンロード回数制限を超過しています"}
	}

	ttl := s.downloadURLTTL()
	now := time.Now().UTC()
	url, err := s.ObjectStore.GenerateDownloadURL(ctx, file.StorageBucket, file.StorageKey, ttl)
	if err != nil {
		return DownloadURLOutput{}, fmt.Errorf("generate download url: %w", err)
	}
	if _, err := s.Store.CreateDownloadEvent(ctx, store.CreateDownloadEventParams{
		ShipmentID:  state.shipment.ID,
		FileID:      file.ID,
		RecipientID: state.token.RecipientID,
		Result:      "success",
		IPHash:      hashIP(in.IPAddress),
		UserAgent:   optionalString(in.UserAgent),
	}); err != nil {
		return DownloadURLOutput{}, fmt.Errorf("create download event: %w", err)
	}
	if err := s.Store.UpdateAccessTokenUsage(ctx, state.token.ID); err != nil {
		return DownloadURLOutput{}, fmt.Errorf("update access token usage: %w", err)
	}
	if err := s.Store.IncrementShipmentDownloadCount(ctx, state.shipment.ID); err != nil {
		return DownloadURLOutput{}, fmt.Errorf("increment shipment download count: %w", err)
	}
	return DownloadURLOutput{URL: url, ExpiresAt: now.Add(ttl)}, nil
}

type accessState struct {
	token         store.AccessToken
	shipment      store.Shipment
	files         []store.File
	downloadCount int32
}

func (s *AccessService) resolveAccessState(ctx context.Context, rawToken string) (accessState, error) {
	if strings.TrimSpace(rawToken) == "" {
		return accessState{}, &APIError{Status: 400, Code: "invalid_token", Message: "token は必須です"}
	}
	token, err := s.Store.GetAccessTokenByHash(ctx, hashToken(rawToken))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return accessState{}, &APIError{Status: 404, Code: "token_not_found", Message: "token が見つかりません"}
		}
		return accessState{}, fmt.Errorf("get access token by hash: %w", err)
	}
	if token.TokenType != "download_access" {
		return accessState{}, &APIError{Status: 401, Code: "invalid_token", Message: "token が不正です"}
	}
	if token.Status == "revoked" || token.RevokedAt != nil {
		return accessState{}, &APIError{Status: 403, Code: "access_forbidden", Message: "token は無効化されています"}
	}
	now := time.Now().UTC()
	if token.ExpiresAt.Before(now) {
		return accessState{}, &APIError{Status: 410, Code: "token_expired", Message: "token の有効期限が切れています"}
	}
	if token.MaxUses > 0 && token.UsedCount >= token.MaxUses {
		return accessState{}, &APIError{Status: 409, Code: "download_limit_exceeded", Message: "トークン利用上限を超過しています"}
	}

	shipment, err := s.Store.GetShipmentByID(ctx, token.ShipmentID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return accessState{}, &APIError{Status: 404, Code: "shipment_not_found", Message: "shipment が見つかりません"}
		}
		return accessState{}, fmt.Errorf("get shipment by id: %w", err)
	}
	if shipment.ExpiresAt.Before(now) {
		return accessState{}, &APIError{Status: 410, Code: "shipment_expired", Message: "shipment の有効期限が切れています"}
	}
	if shipment.Status == "revoked" || shipment.Status == "deleted" || shipment.Status == "expired" {
		return accessState{}, &APIError{Status: 403, Code: "access_forbidden", Message: "shipment へアクセスできません"}
	}
	if shipment.Status != "sent" && shipment.Status != "accessed" {
		return accessState{}, &APIError{Status: 403, Code: "access_forbidden", Message: "shipment の状態が不正です"}
	}
	if shipment.ShareMode == "recipient_restricted" && token.RecipientID == nil {
		return accessState{}, &APIError{Status: 403, Code: "access_forbidden", Message: "受信者限定shipmentへのアクセス権がありません"}
	}

	downloadCount, err := s.Store.CountDownloadEventsByShipment(ctx, shipment.ID)
	if err != nil {
		return accessState{}, fmt.Errorf("count download events by shipment: %w", err)
	}
	files, err := s.Store.GetFilesByShipmentID(ctx, shipment.ID)
	if err != nil {
		return accessState{}, fmt.Errorf("get files by shipment id: %w", err)
	}
	return accessState{token: token, shipment: shipment, files: files, downloadCount: downloadCount}, nil
}

func (s *AccessService) logDownloadEvent(ctx context.Context, fileID uuid.UUID, state accessState, result string, in DownloadURLInput) {
	_, _ = s.Store.CreateDownloadEvent(ctx, store.CreateDownloadEventParams{
		ShipmentID:  state.shipment.ID,
		FileID:      fileID,
		RecipientID: state.token.RecipientID,
		Result:      result,
		IPHash:      hashIP(in.IPAddress),
		UserAgent:   optionalString(in.UserAgent),
	})
}

func hashIP(ip string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(ip)))
	return hex.EncodeToString(sum[:])
}

func optionalString(v string) *string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func (s *AccessService) accessGrantExpiresAt(state accessState) time.Time {
	expiresAt := time.Now().UTC().Add(s.accessGrantTTL())
	if state.token.ExpiresAt.Before(expiresAt) {
		expiresAt = state.token.ExpiresAt
	}
	if state.shipment.ExpiresAt.Before(expiresAt) {
		expiresAt = state.shipment.ExpiresAt
	}
	return expiresAt
}

func (s *AccessService) downloadURLTTL() time.Duration {
	if s.DownloadURLTTL <= 0 {
		return defaultDownloadURLTTL
	}
	return s.DownloadURLTTL
}

func (s *AccessService) accessGrantTTL() time.Duration {
	if s.AccessGrantTTL <= 0 {
		return defaultAccessGrantTTL
	}
	return s.AccessGrantTTL
}

func (s *AccessService) guard() *AccessGuard {
	if s.Guard == nil {
		s.Guard = NewAccessGuard()
	}
	return s.Guard
}
