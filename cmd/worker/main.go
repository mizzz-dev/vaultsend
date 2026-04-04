package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/example/vaultsend/internal/config"
	"github.com/example/vaultsend/internal/mail"
	"github.com/example/vaultsend/internal/queue"
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
	queries := store.New(pool)

	mailQueue := queue.NewSQSQueue(sqs.NewFromConfig(awsCfg), cfg.SQSQueueURL)
	sesMailer := mail.NewSESMailer(sesv2.NewFromConfig(awsCfg), cfg.SESFromEmail)

	w := &worker.MailWorker{
		Queue:       mailQueue,
		Mailer:      sesMailer,
		EventStore:  queries,
		FrontendURL: cfg.FrontendURL,
		MaxMessages: 5,
		WaitSeconds: 20,
	}
	log.Printf("mail worker starting env=%s queue=%s", cfg.AppEnv, cfg.SQSQueueURL)
	if err := w.Run(ctx); err != nil {
		log.Fatalf("mail worker stopped with error: %v", err)
	}
	log.Printf("mail worker stopped")
}
