package service

import (
	"context"
	"testing"
	"time"

	"github.com/example/vaultsend/internal/store"
	"github.com/google/uuid"
)

func TestInspectAccessWithGrant_ProtectedShipmentRestoresVerifiedState(t *testing.T) {
	fs := protectedAccessStore(t)
	svc := &AccessService{Store: fs, AccessGrantSecret: testAccessGrantSecret}
	grant, err := issueAccessGrant(testAccessGrantSecret, "raw-token", time.Now().UTC().Add(5*time.Minute))
	if err != nil {
		t.Fatalf("issue grant: %v", err)
	}

	out, err := svc.InspectAccessWithGrant(context.Background(), "raw-token", grant)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !out.RequiresPassword || !out.Verified {
		t.Fatalf("expected protected and verified: %+v", out)
	}
}

func TestInspectAccessWithGrant_ProtectedShipmentWithoutGrantIsUnverified(t *testing.T) {
	fs := protectedAccessStore(t)
	svc := &AccessService{Store: fs, AccessGrantSecret: testAccessGrantSecret}

	out, err := svc.InspectAccessWithGrant(context.Background(), "raw-token", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !out.RequiresPassword || out.Verified {
		t.Fatalf("expected protected and unverified: %+v", out)
	}
}

func TestInspectAccessWithGrant_PasswordlessShipmentIsVerified(t *testing.T) {
	shipmentID := uuid.New()
	fs := &fakeAccessStore{
		token: store.AccessToken{
			ID:        uuid.New(),
			ShipmentID: shipmentID,
			TokenType: "download_access",
			ExpiresAt: time.Now().UTC().Add(time.Hour),
			MaxUses:   10,
			Status:    "active",
		},
		shipment: store.Shipment{
			ID:           shipmentID,
			Status:       "sent",
			ShareMode:    "url_shared",
			Title:        "件名",
			ExpiresAt:    time.Now().UTC().Add(time.Hour),
			MaxDownloads: 10,
		},
	}
	svc := &AccessService{Store: fs}

	out, err := svc.InspectAccessWithGrant(context.Background(), "raw-token", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.RequiresPassword || !out.Verified {
		t.Fatalf("expected passwordless and verified: %+v", out)
	}
}
