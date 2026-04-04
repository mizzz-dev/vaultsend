package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/example/vaultsend/internal/queue"
	"github.com/example/vaultsend/internal/store"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	defaultShipmentMaxFiles       = 20
	defaultShipmentMaxRecipients  = 20
	defaultShipmentMaxDownloads   = 10
	defaultShipmentMinPasswordLen = 8
	defaultShipmentMinExpiryDays  = 1
	defaultShipmentMaxExpiryDays  = 14
)

var subjectUnsafeChars = regexp.MustCompile(`\p{C}`)

type ShipmentStore interface {
	GetShipment(ctx context.Context, id uuid.UUID) (store.Shipment, error)
	GetFilesByIDs(ctx context.Context, ids []uuid.UUID) ([]store.FileWithShipment, error)
	FinalizeShipment(ctx context.Context, arg store.FinalizeShipmentParams) (store.ShipmentFinalizeResult, error)
	GetFilesByShipmentID(ctx context.Context, shipmentID uuid.UUID) ([]store.File, error)
	GetRecipientsByShipmentID(ctx context.Context, shipmentID uuid.UUID) ([]store.Recipient, error)
}

type ShipmentService struct {
	Store       ShipmentStore
	Queue       queue.Enqueuer
	FrontendURL string
}

type ShipmentRecipientInput struct {
	Email string `json:"email"`
}

type CreateShipmentInput struct {
	ShipmentID       *uuid.UUID
	FileIDs          []uuid.UUID
	OwnerUserID      *uuid.UUID
	Subject          string
	Message          *string
	ShareMode        string
	Recipients       []ShipmentRecipientInput
	ExpiresAt        *time.Time
	MaxDownloadCount *int32
	Password         *string
}

type CreateShipmentOutput struct {
	ID               uuid.UUID                     `json:"id"`
	Status           string                        `json:"status"`
	ShareMode        string                        `json:"share_mode"`
	ExpiresAt        time.Time                     `json:"expires_at"`
	MaxDownloadCount int32                         `json:"max_download_count"`
	AccessURL        *string                       `json:"access_url,omitempty"`
	Recipients       []CreateShipmentRecipientView `json:"recipients"`
	Files            []CreateShipmentFileView      `json:"files"`
}

type CreateShipmentRecipientView struct {
	ID     uuid.UUID `json:"id"`
	Email  string    `json:"email"`
	Status string    `json:"status"`
}

type CreateShipmentFileView struct {
	ID           uuid.UUID `json:"id"`
	OriginalName string    `json:"original_name"`
	SizeBytes    int64     `json:"size_bytes"`
}

type ShipmentDetailOutput struct {
	ID               uuid.UUID                     `json:"id"`
	Status           string                        `json:"status"`
	ShareMode        string                        `json:"share_mode"`
	Subject          string                        `json:"subject"`
	Message          *string                       `json:"message,omitempty"`
	ExpiresAt        time.Time                     `json:"expires_at"`
	MaxDownloadCount int32                         `json:"max_download_count"`
	Files            []CreateShipmentFileView      `json:"files"`
	Recipients       []CreateShipmentRecipientView `json:"recipients"`
}

func (s *ShipmentService) CreateShipment(ctx context.Context, in CreateShipmentInput) (CreateShipmentOutput, error) {
	normalized, err := s.validateAndNormalize(ctx, in)
	if err != nil {
		return CreateShipmentOutput{}, err
	}

	result, err := s.Store.FinalizeShipment(ctx, normalized.finalizeParams)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return CreateShipmentOutput{}, &APIError{Status: 404, Code: "shipment_or_file_not_found", Message: "shipment または file が見つかりません"}
		}
		if errors.Is(err, store.ErrConflict) {
			return CreateShipmentOutput{}, &APIError{Status: 409, Code: "shipment_conflict", Message: "shipment の状態が競合しています"}
		}
		return CreateShipmentOutput{}, fmt.Errorf("finalize shipment: %w", err)
	}

	out := CreateShipmentOutput{
		ID:               result.Shipment.ID,
		Status:           result.Shipment.Status,
		ShareMode:        normalized.responseShareMode,
		ExpiresAt:        result.Shipment.ExpiresAt,
		MaxDownloadCount: result.Shipment.MaxDownloads,
		Recipients:       make([]CreateShipmentRecipientView, 0, len(result.Recipients)),
		Files:            make([]CreateShipmentFileView, 0, len(result.Files)),
	}
	for _, f := range result.Files {
		out.Files = append(out.Files, CreateShipmentFileView{ID: f.ID, OriginalName: f.OriginalName, SizeBytes: f.SizeBytes})
	}
	for _, r := range result.Recipients {
		out.Recipients = append(out.Recipients, CreateShipmentRecipientView{ID: r.ID, Email: r.Email, Status: r.Status})
	}
	if normalized.rawURLSharedToken != "" {
		base := strings.TrimRight(s.FrontendURL, "/")
		if base == "" {
			base = "https://app.example.com" // TODO: FRONTEND_URL 必須化済みだが後方互換としてfallbackを残す。
		}
		accessURL := base + "/r/" + normalized.rawURLSharedToken
		out.AccessURL = &accessURL
	}

	if normalized.responseShareMode == "recipient_restricted" && s.Queue != nil {
		for _, recipient := range result.Recipients {
			rawToken, ok := normalized.recipientRawTokenBy[recipient.EmailNormalized]
			if !ok || rawToken == "" {
				continue
			}
			event := queue.MailNotification{
				ShipmentID:  result.Shipment.ID,
				RecipientID: recipient.ID,
				Email:       recipient.Email,
				Token:       rawToken,
				Subject:     result.Shipment.Title,
				Message:     result.Shipment.Message,
				ExpiresAt:   &result.Shipment.ExpiresAt,
			}
			if err := s.Queue.EnqueueMail(ctx, event); err != nil {
				// TODO: outboxテーブル導入後は非同期再送で補償する。
				return CreateShipmentOutput{}, fmt.Errorf("enqueue mail notification: %w", err)
			}
		}
	}
	return out, nil
}

func (s *ShipmentService) GetShipmentDetail(ctx context.Context, shipmentID uuid.UUID) (ShipmentDetailOutput, error) {
	shipment, err := s.Store.GetShipment(ctx, shipmentID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ShipmentDetailOutput{}, &APIError{Status: 404, Code: "shipment_not_found", Message: "shipment が見つかりません"}
		}
		return ShipmentDetailOutput{}, fmt.Errorf("get shipment: %w", err)
	}
	files, err := s.Store.GetFilesByShipmentID(ctx, shipmentID)
	if err != nil {
		return ShipmentDetailOutput{}, fmt.Errorf("get files by shipment: %w", err)
	}
	recipients, err := s.Store.GetRecipientsByShipmentID(ctx, shipmentID)
	if err != nil {
		return ShipmentDetailOutput{}, fmt.Errorf("get recipients by shipment: %w", err)
	}

	out := ShipmentDetailOutput{
		ID:               shipment.ID,
		Status:           shipment.Status,
		ShareMode:        normalizeShareModeForResponse(shipment.ShareMode),
		Subject:          shipment.Title,
		Message:          shipment.Message,
		ExpiresAt:        shipment.ExpiresAt,
		MaxDownloadCount: shipment.MaxDownloads,
		Files:            make([]CreateShipmentFileView, 0, len(files)),
		Recipients:       make([]CreateShipmentRecipientView, 0, len(recipients)),
	}
	for _, f := range files {
		out.Files = append(out.Files, CreateShipmentFileView{ID: f.ID, OriginalName: f.OriginalName, SizeBytes: f.SizeBytes})
	}
	for _, r := range recipients {
		out.Recipients = append(out.Recipients, CreateShipmentRecipientView{ID: r.ID, Email: r.Email, Status: r.Status})
	}
	return out, nil
}

type normalizedCreateShipment struct {
	finalizeParams      store.FinalizeShipmentParams
	responseShareMode   string
	rawURLSharedToken   string
	recipientRawTokenBy map[string]string
}

func (s *ShipmentService) validateAndNormalize(ctx context.Context, in CreateShipmentInput) (normalizedCreateShipment, error) {
	_ = ctx
	if len(in.FileIDs) == 0 {
		return normalizedCreateShipment{}, &APIError{Status: 400, Code: "invalid_file_ids", Message: "file_ids は1件以上必要です"}
	}
	if len(in.FileIDs) > defaultShipmentMaxFiles {
		return normalizedCreateShipment{}, &APIError{Status: 400, Code: "file_ids_limit_exceeded", Message: "file_ids が上限を超えています"}
	}

	dedupFileIDs := dedupeUUIDs(in.FileIDs)
	files, err := s.Store.GetFilesByIDs(ctx, dedupFileIDs)
	if err != nil {
		return normalizedCreateShipment{}, fmt.Errorf("get files by ids: %w", err)
	}
	if len(files) != len(dedupFileIDs) {
		return normalizedCreateShipment{}, &APIError{Status: 404, Code: "file_not_found", Message: "対象 file が見つかりません"}
	}

	if strings.TrimSpace(in.Subject) == "" {
		return normalizedCreateShipment{}, &APIError{Status: 400, Code: "invalid_subject", Message: "subject は必須です"}
	}
	if len(in.Subject) > 200 || subjectUnsafeChars.MatchString(in.Subject) {
		return normalizedCreateShipment{}, &APIError{Status: 400, Code: "invalid_subject", Message: "subject が不正です"}
	}
	if in.Message != nil && len(*in.Message) > 5000 {
		return normalizedCreateShipment{}, &APIError{Status: 400, Code: "invalid_message", Message: "message が長すぎます"}
	}

	shareModeDB, shareModeResponse, err := normalizeShareMode(in.ShareMode)
	if err != nil {
		return normalizedCreateShipment{}, err
	}

	now := time.Now().UTC()
	expiresAt := now.AddDate(0, 0, 7)
	if in.ExpiresAt != nil {
		expiresAt = in.ExpiresAt.UTC()
	}
	min := now.AddDate(0, 0, defaultShipmentMinExpiryDays)
	max := now.AddDate(0, 0, defaultShipmentMaxExpiryDays)
	if expiresAt.Before(min) || expiresAt.After(max) {
		return normalizedCreateShipment{}, &APIError{Status: 400, Code: "invalid_expires_at", Message: "expires_at は1日以上14日以内で指定してください"}
	}

	maxDownload := int32(defaultShipmentMaxDownloads)
	if in.MaxDownloadCount != nil {
		maxDownload = *in.MaxDownloadCount
	}
	if maxDownload < 1 || maxDownload > 100 {
		return normalizedCreateShipment{}, &APIError{Status: 400, Code: "invalid_max_download_count", Message: "max_download_count が範囲外です"}
	}

	var passwordHash *string
	if in.Password != nil && strings.TrimSpace(*in.Password) != "" {
		if len(*in.Password) < defaultShipmentMinPasswordLen {
			return normalizedCreateShipment{}, &APIError{Status: 400, Code: "invalid_password", Message: "password は8文字以上必要です"}
		}
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(*in.Password), bcrypt.DefaultCost)
		if hashErr != nil {
			return normalizedCreateShipment{}, fmt.Errorf("hash password: %w", hashErr)
		}
		h := string(hash)
		passwordHash = &h
	}

	for _, f := range files {
		if f.UploadStatus != "completed" {
			return normalizedCreateShipment{}, &APIError{Status: 409, Code: "file_not_completed", Message: "upload 未完了の file は送信確定できません"}
		}
		if f.ShipmentStatus != "draft" && f.ShipmentStatus != "uploading" && f.ShipmentStatus != "ready" {
			return normalizedCreateShipment{}, &APIError{Status: 409, Code: "file_shipment_status_conflict", Message: "既に確定済みの shipment に紐づく file は利用できません"}
		}
		if in.OwnerUserID != nil && f.OwnerUserID != nil && *in.OwnerUserID != *f.OwnerUserID {
			return normalizedCreateShipment{}, &APIError{Status: 409, Code: "file_owner_conflict", Message: "owner_user_id が一致しない file が含まれています"}
		}
	}

	recipients := make([]store.CreateRecipientParams, 0)
	tokens := make([]store.CreateAccessTokenParams, 0)
	plainRecipientTokens := map[string]string{}
	var rawURLSharedToken string
	if shareModeResponse == "recipient_restricted" {
		if len(in.Recipients) == 0 {
			return normalizedCreateShipment{}, &APIError{Status: 400, Code: "invalid_recipients", Message: "recipient_restricted では recipients が必須です"}
		}
		if len(in.Recipients) > defaultShipmentMaxRecipients {
			return normalizedCreateShipment{}, &APIError{Status: 400, Code: "recipients_limit_exceeded", Message: "recipients が上限を超えています"}
		}
		normalizedMap := map[string]ShipmentRecipientInput{}
		for _, rc := range in.Recipients {
			normalizedEmail, normErr := normalizeEmail(rc.Email)
			if normErr != nil {
				return normalizedCreateShipment{}, &APIError{Status: 400, Code: "invalid_recipients", Message: "email 形式が不正です"}
			}
			if _, exists := normalizedMap[normalizedEmail]; exists {
				continue
			}
			normalizedMap[normalizedEmail] = ShipmentRecipientInput{Email: strings.TrimSpace(rc.Email)}
		}
		if len(normalizedMap) == 0 {
			return normalizedCreateShipment{}, &APIError{Status: 400, Code: "invalid_recipients", Message: "有効な recipient が存在しません"}
		}
		keys := make([]string, 0, len(normalizedMap))
		for k := range normalizedMap {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, normalizedEmail := range keys {
			rc := normalizedMap[normalizedEmail]
			recipients = append(recipients, store.CreateRecipientParams{Email: rc.Email, EmailNormalized: normalizedEmail, Status: "pending"})
			raw := generateRawToken()
			plainRecipientTokens[normalizedEmail] = raw
			tokens = append(tokens, store.CreateAccessTokenParams{RecipientEmailNormalized: normalizedEmail, TokenType: "download_access", TokenHash: hashToken(raw), ExpiresAt: expiresAt, MaxUses: maxDownload, Status: "active"})
		}
	} else {
		if len(in.Recipients) > 0 {
			return normalizedCreateShipment{}, &APIError{Status: 400, Code: "recipients_not_allowed", Message: "url_shared では recipients を指定できません"}
		}
		rawURLSharedToken = generateRawToken()
		tokens = append(tokens, store.CreateAccessTokenParams{TokenType: "download_access", TokenHash: hashToken(rawURLSharedToken), ExpiresAt: expiresAt, MaxUses: maxDownload, Status: "active"})
	}

	shipmentID := in.ShipmentID
	if shipmentID == nil {
		first := files[0].ShipmentID
		for _, f := range files[1:] {
			if f.ShipmentID != first {
				return normalizedCreateShipment{}, &APIError{Status: 409, Code: "mixed_shipment_files", Message: "異なる draft shipment の file を同時に確定できません"}
			}
		}
		shipmentID = &first
	}

	finalize := store.FinalizeShipmentParams{
		ShipmentID:               *shipmentID,
		ExpectedStatuses:         []string{"draft", "uploading", "ready"},
		Title:                    in.Subject,
		Message:                  in.Message,
		ShareMode:                shareModeDB,
		Status:                   "sent",
		ExpiresAt:                expiresAt,
		MaxDownloads:             maxDownload,
		PasswordHash:             passwordHash,
		OwnerUserID:              in.OwnerUserID,
		FileIDs:                  dedupFileIDs,
		Recipients:               recipients,
		AccessTokens:             tokens,
		PlainRecipientTokenByKey: plainRecipientTokens,
	}

	return normalizedCreateShipment{
		finalizeParams:      finalize,
		responseShareMode:   shareModeResponse,
		rawURLSharedToken:   rawURLSharedToken,
		recipientRawTokenBy: plainRecipientTokens,
	}, nil
}

func normalizeShareMode(v string) (string, string, error) {
	switch strings.TrimSpace(v) {
	case "", "recipient_restricted":
		return "recipient_restricted", "recipient_restricted", nil
	case "url_shared", "public_link":
		return "url_shared", "url_shared", nil
	default:
		return "", "", &APIError{Status: 400, Code: "invalid_share_mode", Message: "share_mode が不正です"}
	}
}

func normalizeShareModeForResponse(v string) string {
	if v == "public_link" {
		return "url_shared"
	}
	return v
}

func normalizeEmail(v string) (string, error) {
	trimmed := strings.TrimSpace(v)
	addr, err := mail.ParseAddress(trimmed)
	if err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(addr.Address)), nil
}

func dedupeUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := map[uuid.UUID]struct{}{}
	result := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func generateRawToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return uuid.NewString()
	}
	return hex.EncodeToString(buf)
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
