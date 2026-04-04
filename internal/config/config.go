package config

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config はアプリケーション全体で利用する設定値を保持する。
// MVP時点では環境変数ベースで管理し、後続PRでSecret Manager連携を検討する。
type Config struct {
	AppEnv              string
	Port                string
	DatabaseURL         string
	AWSRegion           string
	S3Bucket            string
	SQSQueueURL         string
	SESFromEmail        string
	FrontendURL         string
	StripeSecretKey     string
	StripeWebhookSecret string
	StripePriceIDPro    string

	// HTTPRequestTimeout はtimeout middlewareの既定値。
	HTTPRequestTimeout time.Duration
	UploadURLTTL       time.Duration
	PresignedURLTTL    time.Duration
	UploadPartSize     int32
	UploadMaxFileSize  int64
	UploadMaxParts     int

	RateLimitRPS        int
	VerifyMaxAttempts   int
	DownloadRateLimit   int
	CleanupInterval     time.Duration
	CleanupBatchSize    int32
	DeletionGracePeriod time.Duration

	// 認証セッション関連。
	SessionTTLHours int
	CookieDomain    string
	CookieSecure    bool
	CookieSameSite  http.SameSite
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv:              getEnv("APP_ENV", "local"),
		Port:                getEnv("PORT", "8080"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		AWSRegion:           os.Getenv("AWS_REGION"),
		S3Bucket:            os.Getenv("S3_BUCKET"),
		SQSQueueURL:         os.Getenv("SQS_QUEUE_URL"),
		SESFromEmail:        os.Getenv("SES_FROM_EMAIL"),
		FrontendURL:         os.Getenv("FRONTEND_URL"),
		StripeSecretKey:     os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		StripePriceIDPro:    os.Getenv("STRIPE_PRICE_ID_PRO"),
		HTTPRequestTimeout:  30 * time.Second,
		UploadURLTTL:        15 * time.Minute,
		PresignedURLTTL:     60 * time.Second,
		UploadPartSize:      8 * 1024 * 1024,
		UploadMaxFileSize:   10 * 1024 * 1024 * 1024,
		UploadMaxParts:      1000,
		RateLimitRPS:        100,
		VerifyMaxAttempts:   5,
		DownloadRateLimit:   10,
		CleanupInterval:     3 * time.Minute,
		CleanupBatchSize:    100,
		DeletionGracePeriod: 24 * time.Hour,
		SessionTTLHours:     24 * 7,
		CookieDomain:        strings.TrimSpace(os.Getenv("COOKIE_DOMAIN")),
		CookieSecure:        true,
		CookieSameSite:      http.SameSiteLaxMode,
	}

	if cfg.AppEnv == "local" || cfg.AppEnv == "test" {
		cfg.CookieSecure = false
	}
	if v := os.Getenv("COOKIE_SECURE"); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid COOKIE_SECURE: %q", v)
		}
		cfg.CookieSecure = parsed
	}
	if v := os.Getenv("COOKIE_SAMESITE"); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "lax":
			cfg.CookieSameSite = http.SameSiteLaxMode
		case "strict":
			cfg.CookieSameSite = http.SameSiteStrictMode
		case "none":
			cfg.CookieSameSite = http.SameSiteNoneMode
		default:
			return Config{}, fmt.Errorf("invalid COOKIE_SAMESITE: %q", v)
		}
	}

	if v := os.Getenv("SESSION_TTL_HOURS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("invalid SESSION_TTL_HOURS: %q", v)
		}
		cfg.SessionTTLHours = n
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
	if v := os.Getenv("PRESIGNED_URL_TTL"); v != "" {
		sec, err := strconv.Atoi(v)
		if err != nil || sec <= 0 {
			return Config{}, fmt.Errorf("invalid PRESIGNED_URL_TTL: %q", v)
		}
		cfg.PresignedURLTTL = time.Duration(sec) * time.Second
	}
	if v := os.Getenv("RATE_LIMIT_RPS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("invalid RATE_LIMIT_RPS: %q", v)
		}
		cfg.RateLimitRPS = n
	}
	if v := os.Getenv("VERIFY_MAX_ATTEMPTS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("invalid VERIFY_MAX_ATTEMPTS: %q", v)
		}
		cfg.VerifyMaxAttempts = n
	}
	if v := os.Getenv("DOWNLOAD_RATE_LIMIT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("invalid DOWNLOAD_RATE_LIMIT: %q", v)
		}
		cfg.DownloadRateLimit = n
	}
	if v := os.Getenv("CLEANUP_INTERVAL_SEC"); v != "" {
		sec, err := strconv.Atoi(v)
		if err != nil || sec <= 0 {
			return Config{}, fmt.Errorf("invalid CLEANUP_INTERVAL_SEC: %q", v)
		}
		cfg.CleanupInterval = time.Duration(sec) * time.Second
	}
	if v := os.Getenv("CLEANUP_BATCH_SIZE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("invalid CLEANUP_BATCH_SIZE: %q", v)
		}
		cfg.CleanupBatchSize = int32(n)
	}
	if v := os.Getenv("DELETION_GRACE_PERIOD_HOURS"); v != "" {
		h, err := strconv.Atoi(v)
		if err != nil || h <= 0 {
			return Config{}, fmt.Errorf("invalid DELETION_GRACE_PERIOD_HOURS: %q", v)
		}
		cfg.DeletionGracePeriod = time.Duration(h) * time.Hour
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
	if cfg.FrontendURL == "" {
		missing = append(missing, "FRONTEND_URL")
	}
	if cfg.StripeSecretKey == "" {
		missing = append(missing, "STRIPE_SECRET_KEY")
	}
	if cfg.StripeWebhookSecret == "" {
		missing = append(missing, "STRIPE_WEBHOOK_SECRET")
	}
	if cfg.StripePriceIDPro == "" {
		missing = append(missing, "STRIPE_PRICE_ID_PRO")
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
