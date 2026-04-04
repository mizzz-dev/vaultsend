package http

import (
	stdhttp "net/http"

	"github.com/example/vaultsend/internal/config"
	"github.com/example/vaultsend/internal/http/handler"
	appmw "github.com/example/vaultsend/internal/http/middleware"
	"github.com/example/vaultsend/internal/service"
	"github.com/example/vaultsend/internal/store"
	"github.com/go-chi/chi/v5"
)

func NewServer(cfg config.Config, queries *store.Queries, uploadSvc *service.UploadService, shipmentSvc *service.ShipmentService, accessSvc *service.AccessService, authSvc *service.AuthService, billingSvc *service.BillingService) stdhttp.Handler {
	r := chi.NewRouter()
	rateLimiter := appmw.NewInMemoryRateLimiter()

	r.Use(appmw.RequestID)
	r.Use(appmw.Recovery)
	r.Use(appmw.RequestLogger)
	r.Use(appmw.SecurityHeaders)
	r.Use(appmw.Timeout(cfg.HTTPRequestTimeout))
	r.Use(appmw.RateLimit(rateLimiter, appmw.RateLimitConfig{
		PerMinuteLimit: cfg.RateLimitRPS,
		VerifyLimit:    max(10, cfg.VerifyMaxAttempts*2),
	}))
	r.Use(appmw.OptionalAuth(authSvc))
	r.Use(appmw.OptionalPlan(billingSvc))

	uploadHandler := handler.UploadHandler{Service: uploadSvc}
	shipmentHandler := handler.ShipmentHandler{Service: shipmentSvc}
	accessHandler := handler.AccessHandler{Service: accessSvc}
	authHandler := handler.AuthHandler{Service: authSvc, CookieDomain: cfg.CookieDomain, CookieSecure: cfg.CookieSecure, CookieSameSite: cfg.CookieSameSite}
	billingHandler := handler.BillingHandler{Service: billingSvc}

	r.Get("/healthz", handler.Health)
	r.Route("/v1", func(r chi.Router) {
		r.Post("/uploads", uploadHandler.CreateUpload)
		r.Post("/uploads/{id}/complete", uploadHandler.CompleteUpload)
		r.Post("/shipments", shipmentHandler.CreateShipment)
		r.Group(func(r chi.Router) {
			r.Use(appmw.RequireAuth(authSvc))
			r.Get("/shipments", shipmentHandler.ListShipments)
			r.Get("/shipments/{id}", shipmentHandler.GetShipment)
			r.Get("/shipments/{id}/notifications", shipmentHandler.ListShipmentNotifications)
			r.Get("/shipments/{id}/recipients", shipmentHandler.ListShipmentRecipients)
			r.Post("/shipments/{id}/resend", shipmentHandler.ResendShipment)
			r.Delete("/shipments/{id}", shipmentHandler.DeleteShipment)
		})
		r.Get("/access/{token}", accessHandler.InspectAccess)
		r.Post("/access/{token}/verify", accessHandler.VerifyAccess)
		r.Get("/files/{id}/download-url", accessHandler.GenerateDownloadURL)

		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)
			r.Group(func(r chi.Router) {
				r.Use(appmw.RequireAuth(authSvc))
				r.Post("/logout", authHandler.Logout)
				r.Get("/me", authHandler.Me)
			})
		})
		r.Post("/billing/checkout", billingHandler.CreateCheckout)
		r.Post("/billing/webhook", billingHandler.Webhook)
	})

	_ = queries
	return r
}
