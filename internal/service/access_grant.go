package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var errInvalidAccessGrant = errors.New("invalid access grant")

// accessGrantPayload はパスワード検証済み状態を表す短命の署名対象。
// 生のaccess tokenは含めず、hashと期限だけを保持する。
type accessGrantPayload struct {
	TokenHash string
	ExpiresAt time.Time
}

func issueAccessGrant(secret, rawToken string, expiresAt time.Time) (string, error) {
	if len(secret) < 32 {
		return "", errors.New("access grant secret must be at least 32 bytes")
	}
	payload := hashToken(rawToken) + ":" + strconv.FormatInt(expiresAt.Unix(), 10)
	encodedPayload := base64.RawURLEncoding.EncodeToString([]byte(payload))
	signature := signAccessGrant(secret, encodedPayload)
	return encodedPayload + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func validateAccessGrant(secret, rawToken, grant string, now time.Time) error {
	if len(secret) < 32 || strings.TrimSpace(grant) == "" {
		return errInvalidAccessGrant
	}
	parts := strings.Split(grant, ".")
	if len(parts) != 2 {
		return errInvalidAccessGrant
	}
	providedSignature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return errInvalidAccessGrant
	}
	expectedSignature := signAccessGrant(secret, parts[0])
	if !hmac.Equal(providedSignature, expectedSignature) {
		return errInvalidAccessGrant
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return errInvalidAccessGrant
	}
	payload, err := parseAccessGrantPayload(string(payloadBytes))
	if err != nil {
		return errInvalidAccessGrant
	}
	if !hmac.Equal([]byte(payload.TokenHash), []byte(hashToken(rawToken))) {
		return errInvalidAccessGrant
	}
	if !payload.ExpiresAt.After(now) {
		return errInvalidAccessGrant
	}
	return nil
}

func parseAccessGrantPayload(value string) (accessGrantPayload, error) {
	separator := strings.LastIndex(value, ":")
	if separator <= 0 || separator == len(value)-1 {
		return accessGrantPayload{}, errInvalidAccessGrant
	}
	tokenHash := value[:separator]
	if len(tokenHash) != sha256.Size*2 {
		return accessGrantPayload{}, errInvalidAccessGrant
	}
	expiresUnix, err := strconv.ParseInt(value[separator+1:], 10, 64)
	if err != nil {
		return accessGrantPayload{}, fmt.Errorf("parse access grant expiry: %w", err)
	}
	return accessGrantPayload{TokenHash: tokenHash, ExpiresAt: time.Unix(expiresUnix, 0).UTC()}, nil
}

func signAccessGrant(secret, encodedPayload string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(encodedPayload))
	return mac.Sum(nil)
}
