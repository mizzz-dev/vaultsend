package http

import (
	stdhttp "net/http"

	"github.com/example/vaultsend/internal/config"
	"github.com/example/vaultsend/internal/http/handler"
	appmw "github.com/example/vaultsend/internal/http/middleware"
	"github.com/example/vaultsend/internal/store"
	"github.com/go-chi/chi/v5"
)

func NewServer(cfg config.Config, queries *store.Queries) stdhttp.Handler {
	r := chi.NewRouter()

	// 共通ミドルウェア。
	r.Use(appmw.RequestID)
	r.Use(appmw.Recovery)
	r.Use(appmw.RequestLogger)
	r.Use(appmw.Timeout(cfg.HTTPRequestTimeout))

	uploadHandler := handler.UploadHandler{Queries: queries, S3Bucket: cfg.S3Bucket}
	shipmentHandler := handler.ShipmentHandler{Queries: queries}

	r.Get("/healthz", handler.Health)
	r.Route("/v1", func(r chi.Router) {
		r.Post("/uploads", uploadHandler.CreateUpload)
		r.Post("/uploads/{id}/complete", uploadHandler.CompleteUpload)
		r.Post("/shipments", shipmentHandler.CreateShipment)
		r.Get("/shipments/{id}", shipmentHandler.GetShipment)
	})

	return r
}
