package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config はアプリケーション全体で利用する設定値を保持する。
// MVP時点では環境変数ベースで管理し、後続PRでSecret Manager連携を検討する。
type Config struct {
	AppEnv       string
	Port         string
	DatabaseURL  string
	AWSRegion    string
	S3Bucket     string
	SQSQueueURL  string
	SESFromEmail string

	// HTTPRequestTimeout はtimeout middlewareの既定値。
	HTTPRequestTimeout time.Duration
	UploadURLTTL       time.Duration
	UploadPartSize     int32
	UploadMaxFileSize  int64
	UploadMaxParts     int
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv:             getEnv("APP_ENV", "local"),
		Port:               getEnv("PORT", "8080"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		AWSRegion:          os.Getenv("AWS_REGION"),
		S3Bucket:           os.Getenv("S3_BUCKET"),
		SQSQueueURL:        os.Getenv("SQS_QUEUE_URL"),
		SESFromEmail:       os.Getenv("SES_FROM_EMAIL"),
		HTTPRequestTimeout: 30 * time.Second,
		UploadURLTTL:       15 * time.Minute,
		UploadPartSize:     8 * 1024 * 1024,
		UploadMaxFileSize:  10 * 1024 * 1024 * 1024,
		UploadMaxParts:     1000,
	}

	if v := os.Getenv("HTTP_REQUEST_TIMEOUT_SEC"); v != "" {
		sec, err := strconv.Atoi(v)
		if err != nil || sec <= 0 {
			return Config{}, fmt.Errorf("invalid HTTP_REQUEST_TIMEOUT_SEC: %q", v)
		}
		cfg.HTTPRequestTimeout = time.Duration(sec) * time.Second
	}
	if v := os.Getenv("UPLOAD_URL_TTL_SEC"); v != "" {
		sec, err := strconv.Atoi(v)
		if err != nil || sec <= 0 {
			return Config{}, fmt.Errorf("invalid UPLOAD_URL_TTL_SEC: %q", v)
		}
		cfg.UploadURLTTL = time.Duration(sec) * time.Second
	}

	missing := make([]string, 0)
	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if cfg.AWSRegion == "" {
		missing = append(missing, "AWS_REGION")
	}
	if cfg.S3Bucket == "" {
		missing = append(missing, "S3_BUCKET")
	}
	if cfg.SQSQueueURL == "" {
		missing = append(missing, "SQS_QUEUE_URL")
	}
	if cfg.SESFromEmail == "" {
		missing = append(missing, "SES_FROM_EMAIL")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required envs: %v", missing)
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
