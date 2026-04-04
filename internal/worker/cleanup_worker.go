package worker

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/example/vaultsend/internal/store"
	"github.com/google/uuid"
)

const defaultS3DeleteRetries = 3

type CleanupStore interface {
	ListExpiredShipments(ctx context.Context, now time.Time, limit int32) ([]store.Shipment, error)
	MarkShipmentExpired(ctx context.Context, shipmentID uuid.UUID, now time.Time) error
	ListDeletedShipmentsForCleanup(ctx context.Context, deletedBefore time.Time, limit int32) ([]store.Shipment, error)
	GetFilesByShipmentID(ctx context.Context, shipmentID uuid.UUID) ([]store.File, error)
	DeleteShipmentCascade(ctx context.Context, shipmentID uuid.UUID) error
}

type CleanupObjectStore interface {
	DeleteObject(ctx context.Context, bucket, key string) error
}

type CleanupWorker struct {
	Store              CleanupStore
	ObjectStore        CleanupObjectStore
	Interval           time.Duration
	BatchSize          int32
	DeletionGrace      time.Duration
	S3DeleteMaxRetries int
}

type CleanupRunResult struct {
	ExpiredMarked int
	Deleted       int
}

func (w *CleanupWorker) Run(ctx context.Context) error {
	if w.Interval <= 0 {
		w.Interval = 3 * time.Minute
	}

	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()

	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		if _, err := w.RunOnce(ctx); err != nil {
			log.Printf("cleanup worker: run failed err=%v", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *CleanupWorker) RunOnce(ctx context.Context) (CleanupRunResult, error) {
	if w.Store == nil {
		return CleanupRunResult{}, errors.New("cleanup worker: store is required")
	}
	if w.ObjectStore == nil {
		return CleanupRunResult{}, errors.New("cleanup worker: object store is required")
	}
	if w.BatchSize <= 0 {
		w.BatchSize = 100
	}
	if w.DeletionGrace <= 0 {
		w.DeletionGrace = 24 * time.Hour
	}
	if w.S3DeleteMaxRetries <= 0 {
		w.S3DeleteMaxRetries = defaultS3DeleteRetries
	}

	now := time.Now().UTC()
	log.Printf("cleanup worker: start now=%s batch_size=%d grace=%s", now.Format(time.RFC3339), w.BatchSize, w.DeletionGrace)

	result := CleanupRunResult{}

	expired, err := w.Store.ListExpiredShipments(ctx, now, w.BatchSize)
	if err != nil {
		return result, err
	}
	for _, shipment := range expired {
		if err := w.Store.MarkShipmentExpired(ctx, shipment.ID, now); err != nil {
			log.Printf("cleanup worker: mark expired failed shipment_id=%s err=%v", shipment.ID, err)
			continue
		}
		result.ExpiredMarked++
	}

	threshold := now.Add(-w.DeletionGrace)
	deletedTargets, err := w.Store.ListDeletedShipmentsForCleanup(ctx, threshold, w.BatchSize)
	if err != nil {
		return result, err
	}
	for _, shipment := range deletedTargets {
		if err := w.cleanupShipment(ctx, shipment.ID); err != nil {
			log.Printf("cleanup worker: cleanup failed shipment_id=%s err=%v", shipment.ID, err)
			continue
		}
		result.Deleted++
		log.Printf("cleanup worker: deleted shipment_id=%s", shipment.ID)
	}

	log.Printf("cleanup worker: end expired_marked=%d deleted=%d", result.ExpiredMarked, result.Deleted)
	return result, nil
}

func (w *CleanupWorker) cleanupShipment(ctx context.Context, shipmentID uuid.UUID) error {
	files, err := w.Store.GetFilesByShipmentID(ctx, shipmentID)
	if err != nil {
		return err
	}

	for _, f := range files {
		if err := w.deleteObjectWithRetry(ctx, f.StorageBucket, f.StorageKey); err != nil {
			return err
		}
	}
	if err := w.Store.DeleteShipmentCascade(ctx, shipmentID); err != nil {
		return err
	}
	return nil
}

func (w *CleanupWorker) deleteObjectWithRetry(ctx context.Context, bucket, key string) error {
	var lastErr error
	for i := 0; i < w.S3DeleteMaxRetries; i++ {
		if err := w.ObjectStore.DeleteObject(ctx, bucket, key); err != nil {
			lastErr = err
			time.Sleep(time.Duration(i+1) * 150 * time.Millisecond)
			continue
		}
		return nil
	}
	return lastErr
}
