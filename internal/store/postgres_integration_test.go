//go:build integration

package store

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresStoreShipmentAndFileLifecycle(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL が未設定のためintegration testをスキップします")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("PostgreSQL poolの作成に失敗しました: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("PostgreSQLへの接続に失敗しました: %v", err)
	}

	queries := New(pool)
	expiresAt := time.Now().UTC().Add(24 * time.Hour)

	firstShipment, err := queries.CreateShipment(ctx, CreateShipmentParams{
		OwnerType:    "anonymous",
		Status:       "uploading",
		ShareMode:    "recipient_restricted",
		Title:        "integration test shipment",
		MaxDownloads: 10,
		ExpiresAt:    expiresAt,
	})
	if err != nil {
		t.Fatalf("shipmentの作成に失敗しました: %v", err)
	}

	secondShipment, err := queries.CreateShipment(ctx, CreateShipmentParams{
		OwnerType:    "anonymous",
		Status:       "uploading",
		ShareMode:    "recipient_restricted",
		Title:        "integration test destination",
		MaxDownloads: 10,
		ExpiresAt:    expiresAt,
	})
	if err != nil {
		t.Fatalf("2件目のshipment作成に失敗しました: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM shipments WHERE id = ANY($1::uuid[])`, []uuid.UUID{firstShipment.ID, secondShipment.ID})
	})

	storedShipment, err := queries.GetShipment(ctx, firstShipment.ID)
	if err != nil {
		t.Fatalf("shipmentの取得に失敗しました: %v", err)
	}
	if storedShipment.ID != firstShipment.ID {
		t.Fatalf("shipment IDが一致しません: got=%s want=%s", storedShipment.ID, firstShipment.ID)
	}
	if storedShipment.Title != "integration test shipment" {
		t.Fatalf("shipment titleが一致しません: %q", storedShipment.Title)
	}

	file, err := queries.CreateFile(ctx, CreateFileParams{
		ShipmentID:     firstShipment.ID,
		OriginalName:   "integration.txt",
		SizeBytes:      128,
		MimeType:       "text/plain",
		StorageBucket:  "vaultsend-integration",
		StorageKey:     "integration/" + uuid.NewString(),
		ChecksumSha256: strings.Repeat("a", 64),
		UploadStatus:   "completed",
	})
	if err != nil {
		t.Fatalf("fileの作成に失敗しました: %v", err)
	}

	_, err = pool.Exec(ctx, `UPDATE files SET shipment_id = $1 WHERE id = $2`, secondShipment.ID, file.ID)
	if err == nil {
		t.Fatal("files.shipment_idの付け替えが成功してしまいました")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("shipment付け替え時にcheck violationを期待しました: %v", err)
	}

	var actualShipmentID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT shipment_id FROM files WHERE id = $1`, file.ID).Scan(&actualShipmentID); err != nil {
		t.Fatalf("file所属shipmentの確認に失敗しました: %v", err)
	}
	if actualShipmentID != firstShipment.ID {
		t.Fatalf("fileのshipment_idが変更されています: got=%s want=%s", actualShipmentID, firstShipment.ID)
	}

	if err := queries.DeleteShipmentCascade(ctx, firstShipment.ID); err != nil {
		t.Fatalf("shipmentのcascade削除に失敗しました: %v", err)
	}

	_, err = queries.GetShipment(ctx, firstShipment.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("削除済みshipmentでErrNotFoundを期待しました: %v", err)
	}

	var remainingFiles int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM files WHERE id = $1`, file.ID).Scan(&remainingFiles); err != nil {
		t.Fatalf("file削除確認に失敗しました: %v", err)
	}
	if remainingFiles != 0 {
		t.Fatalf("shipment削除後もfileが残っています: %d", remainingFiles)
	}
}
