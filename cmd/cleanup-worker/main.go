package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/example/vaultsend/internal/config"
	"github.com/example/vaultsend/internal/storage"
	"github.com/example/vaultsend/internal/store"
	"github.com/example/vaultsend/internal/worker"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	awsCfg, err := awscfg.LoadDefaultConfig(ctx, awscfg.WithRegion(cfg.AWSRegion))
	if err != nil {
		log.Fatalf("failed to load aws config: %v", err)
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}
	defer pool.Close()

	w := &worker.CleanupWorker{
		Store:         store.New(pool),
		ObjectStore:   storage.NewS3ObjectStore(s3.NewFromConfig(awsCfg)),
		Interval:      cfg.CleanupInterval,
		BatchSize:     cfg.CleanupBatchSize,
		DeletionGrace: cfg.DeletionGracePeriod,
	}

	log.Printf("cleanup worker starting env=%s interval=%s batch_size=%d grace_period=%s", cfg.AppEnv, cfg.CleanupInterval, cfg.CleanupBatchSize, cfg.DeletionGracePeriod)
	if err := w.Run(ctx); err != nil {
		log.Fatalf("cleanup worker stopped with error: %v", err)
	}
	log.Printf("cleanup worker stopped")
}
