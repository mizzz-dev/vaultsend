package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/vaultsend/internal/store"
	"github.com/google/uuid"
)

type fakeCleanupStore struct {
	expired               []store.Shipment
	deleted               []store.Shipment
	filesByShipment       map[uuid.UUID][]store.File
	markedExpired         []uuid.UUID
	cascadedDeleted       []uuid.UUID
	listExpiredErr        error
	listDeletedErr        error
	getFilesErrShipmentID uuid.UUID
	getFilesErr           error
	cascadeErrShipmentID  uuid.UUID
	cascadeErr            error
}

func (f *fakeCleanupStore) ListExpiredShipments(ctx context.Context, now time.Time, limit int32) ([]store.Shipment, error) {
	if f.listExpiredErr != nil {
		return nil, f.listExpiredErr
	}
	return f.expired, nil
}

func (f *fakeCleanupStore) MarkShipmentExpired(ctx context.Context, shipmentID uuid.UUID, now time.Time) error {
	f.markedExpired = append(f.markedExpired, shipmentID)
	return nil
}

func (f *fakeCleanupStore) ListDeletedShipmentsForCleanup(ctx context.Context, deletedBefore time.Time, limit int32) ([]store.Shipment, error) {
	if f.listDeletedErr != nil {
		return nil, f.listDeletedErr
	}
	return f.deleted, nil
}

func (f *fakeCleanupStore) GetFilesByShipmentID(ctx context.Context, shipmentID uuid.UUID) ([]store.File, error) {
	if f.getFilesErr != nil && f.getFilesErrShipmentID == shipmentID {
		return nil, f.getFilesErr
	}
	return f.filesByShipment[shipmentID], nil
}

func (f *fakeCleanupStore) DeleteShipmentCascade(ctx context.Context, shipmentID uuid.UUID) error {
	if f.cascadeErr != nil && f.cascadeErrShipmentID == shipmentID {
		return f.cascadeErr
	}
	f.cascadedDeleted = append(f.cascadedDeleted, shipmentID)
	return nil
}

type fakeCleanupObjectStore struct {
	calls        []string
	failCountKey map[string]int
}

func (f *fakeCleanupObjectStore) DeleteObject(ctx context.Context, bucket, key string) error {
	f.calls = append(f.calls, bucket+"/"+key)
	if remain, ok := f.failCountKey[key]; ok && remain > 0 {
		f.failCountKey[key] = remain - 1
		return errors.New("delete failed")
	}
	return nil
}

func TestCleanupWorker_RunOnce_MarksExpiredAndDeletesShipments(t *testing.T) {
	expiredID := uuid.New()
	deletedID := uuid.New()
	st := &fakeCleanupStore{
		expired: []store.Shipment{{ID: expiredID, Status: "sent", ExpiresAt: time.Now().UTC().Add(-time.Hour)}},
		deleted: []store.Shipment{{ID: deletedID, Status: "deleted", DeletedAt: ptrCleanupTime(time.Now().UTC().Add(-48 * time.Hour))}},
		filesByShipment: map[uuid.UUID][]store.File{
			deletedID: {
				{ShipmentID: deletedID, StorageBucket: "b", StorageKey: "k1"},
			},
		},
	}
	os := &fakeCleanupObjectStore{}
	w := &CleanupWorker{Store: st, ObjectStore: os, BatchSize: 50, DeletionGrace: 24 * time.Hour}

	result, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if result.ExpiredMarked != 1 || result.Deleted != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(st.markedExpired) != 1 || st.markedExpired[0] != expiredID {
		t.Fatalf("expired mark mismatch: %+v", st.markedExpired)
	}
	if len(st.cascadedDeleted) != 1 || st.cascadedDeleted[0] != deletedID {
		t.Fatalf("cascade delete mismatch: %+v", st.cascadedDeleted)
	}
	if len(os.calls) != 1 || os.calls[0] != "b/k1" {
		t.Fatalf("s3 delete mismatch: %+v", os.calls)
	}
}

func TestCleanupWorker_RunOnce_S3DeleteRetry(t *testing.T) {
	deletedID := uuid.New()
	st := &fakeCleanupStore{
		deleted: []store.Shipment{{ID: deletedID, Status: "deleted", DeletedAt: ptrCleanupTime(time.Now().UTC().Add(-48 * time.Hour))}},
		filesByShipment: map[uuid.UUID][]store.File{
			deletedID: {
				{ShipmentID: deletedID, StorageBucket: "b", StorageKey: "retry-key"},
			},
		},
	}
	os := &fakeCleanupObjectStore{failCountKey: map[string]int{"retry-key": 2}}
	w := &CleanupWorker{Store: st, ObjectStore: os, BatchSize: 50, DeletionGrace: 24 * time.Hour, S3DeleteMaxRetries: 3}

	result, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if result.Deleted != 1 {
		t.Fatalf("expected 1 deleted, got=%d", result.Deleted)
	}
	if len(os.calls) != 3 {
		t.Fatalf("expected 3 delete attempts, got=%d", len(os.calls))
	}
}

func TestCleanupWorker_RunOnce_SkipsOnDeleteFailure(t *testing.T) {
	deletedID := uuid.New()
	st := &fakeCleanupStore{
		deleted: []store.Shipment{{ID: deletedID, Status: "deleted", DeletedAt: ptrCleanupTime(time.Now().UTC().Add(-48 * time.Hour))}},
		filesByShipment: map[uuid.UUID][]store.File{
			deletedID: {
				{ShipmentID: deletedID, StorageBucket: "b", StorageKey: "never"},
			},
		},
	}
	os := &fakeCleanupObjectStore{failCountKey: map[string]int{"never": 10}}
	w := &CleanupWorker{Store: st, ObjectStore: os, BatchSize: 50, DeletionGrace: 24 * time.Hour, S3DeleteMaxRetries: 2}

	result, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if result.Deleted != 0 {
		t.Fatalf("expected 0 deleted, got=%d", result.Deleted)
	}
	if len(st.cascadedDeleted) != 0 {
		t.Fatalf("cascade delete should not run on s3 failure: %+v", st.cascadedDeleted)
	}
}

func ptrCleanupTime(v time.Time) *time.Time {
	return &v
}
