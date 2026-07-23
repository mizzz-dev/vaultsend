package service

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestAccessGrant_RejectsTamperedSignature(t *testing.T) {
	grant, err := issueAccessGrant(testAccessGrantSecret, "raw-token", time.Now().UTC().Add(5*time.Minute))
	if err != nil {
		t.Fatalf("issue grant: %v", err)
	}

	parts := strings.Split(grant, ".")
	if len(parts) != 2 {
		t.Fatalf("unexpected grant format: %q", grant)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	if len(signature) == 0 {
		t.Fatal("signature is empty")
	}
	signature[0] ^= 0xff
	tampered := parts[0] + "." + base64.RawURLEncoding.EncodeToString(signature)

	if err := validateAccessGrant(testAccessGrantSecret, "raw-token", tampered, time.Now().UTC()); err == nil {
		t.Fatal("tampered grant must be rejected")
	}
}

func TestAccessGrant_RejectsExpiredGrant(t *testing.T) {
	grant, err := issueAccessGrant(testAccessGrantSecret, "raw-token", time.Now().UTC().Add(-1*time.Minute))
	if err != nil {
		t.Fatalf("issue grant: %v", err)
	}
	if err := validateAccessGrant(testAccessGrantSecret, "raw-token", grant, time.Now().UTC()); err == nil {
		t.Fatal("expired grant must be rejected")
	}
}

func TestAccessGrant_RejectsDifferentSecret(t *testing.T) {
	grant, err := issueAccessGrant(testAccessGrantSecret, "raw-token", time.Now().UTC().Add(5*time.Minute))
	if err != nil {
		t.Fatalf("issue grant: %v", err)
	}
	if err := validateAccessGrant("different-access-grant-secret-at-least-32-bytes", "raw-token", grant, time.Now().UTC()); err == nil {
		t.Fatal("grant signed with another secret must be rejected")
	}
}
