package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/example/vaultsend/internal/http/render"
	"github.com/example/vaultsend/internal/service"
	"github.com/example/vaultsend/internal/storage"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type UploadHandler struct {
	Service *service.UploadService
}

type CreateUploadRequest struct {
	ShipmentID     *uuid.UUID `json:"shipment_id"`
	FileName       string     `json:"file_name"`
	FileSize       int64      `json:"file_size"`
	ContentType    string     `json:"content_type"`
	ChecksumSHA256 string     `json:"checksum_sha256"`
}

type CompleteUploadRequest struct {
	Parts []struct {
		PartNumber int32  `json:"part_number"`
		ETag       string `json:"etag"`
	} `json:"parts"`
}

func (h UploadHandler) CreateUpload(w http.ResponseWriter, r *http.Request) {
	var req CreateUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Error(w, http.StatusBadRequest, "invalid_request", "不正なJSONです", chimw.GetReqID(r.Context()))
		return
	}

	out, err := h.Service.CreateUploadSession(r.Context(), service.CreateUploadInput{
		ShipmentID:     req.ShipmentID,
		FileName:       req.FileName,
		ContentType:    req.ContentType,
		FileSize:       req.FileSize,
		ChecksumSHA256: req.ChecksumSHA256,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	render.JSON(w, http.StatusCreated, out)
}

func (h UploadHandler) CompleteUpload(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	uploadID, err := uuid.Parse(idStr)
	if err != nil {
		render.Error(w, http.StatusBadRequest, "invalid_upload_session_id", "upload session id が不正です", chimw.GetReqID(r.Context()))
		return
	}
	var req CompleteUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Error(w, http.StatusBadRequest, "invalid_request", "不正なJSONです", chimw.GetReqID(r.Context()))
		return
	}

	parts := make([]storage.CompletedPart, 0, len(req.Parts))
	for _, p := range req.Parts {
		parts = append(parts, storage.CompletedPart{PartNumber: p.PartNumber, ETag: p.ETag})
	}

	out, err := h.Service.CompleteUploadSession(r.Context(), service.CompleteUploadInput{UploadSessionID: uploadID, Parts: parts})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	render.JSON(w, http.StatusOK, out)
}

func (h UploadHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *service.APIError
	if errors.As(err, &apiErr) {
		render.Error(w, apiErr.Status, apiErr.Code, apiErr.Message, chimw.GetReqID(r.Context()))
		return
	}
	render.Error(w, http.StatusInternalServerError, "internal_error", "内部エラーが発生しました", chimw.GetReqID(r.Context()))
}
