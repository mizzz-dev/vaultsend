package service

import (
	"testing"
	"time"
)

func TestAccessGrant_RejectsTamperedSignature(t *testing.T) {
	grant, err := issueAccessGrant(testAccessGrantSecret, "raw-token", time.Now().UTC().Add(5*time.Minute))
	if err != nil {
		t.Fatalf("issue grant: %v", err)
	}
	replacement := "A"
	if grant[len(grant)-1:] == replacement {
		replacement = "B"
	}
	tampered := grant[:len(grant)-1] + replacement
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
