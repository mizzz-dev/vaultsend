package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/example/vaultsend/internal/http/render"
	"github.com/example/vaultsend/internal/store"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

const defaultPartSizeBytes int32 = 8 * 1024 * 1024

type UploadHandler struct {
	Queries  *store.Queries
	S3Bucket string
}

type CreateUploadRequest struct {
	ShipmentID     *uuid.UUID `json:"shipment_id"`
	FileName       string     `json:"file_name"`
	SizeBytes      int64      `json:"size_bytes"`
	MimeType       string     `json:"mime_type"`
	ChecksumSHA256 string     `json:"checksum_sha256"`
	PartCount      int        `json:"part_count"`
}

type CompleteUploadRequest struct {
	Parts []struct {
		PartNumber int    `json:"part_number"`
		ETag       string `json:"etag"`
	} `json:"parts"`
}

func (h UploadHandler) CreateUpload(w http.ResponseWriter, r *http.Request) {
	var req CreateUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		render.Error(w, http.StatusBadRequest, "invalid_request", "不正なJSONです", chimw.GetReqID(r.Context()))
		return
	}
	if req.SizeBytes <= 0 {
		render.Error(w, http.StatusBadRequest, "invalid_file_size", "size_bytes は正の値が必要です", chimw.GetReqID(r.Context()))
		return
	}

	// TODO: shipment_id未指定時の匿名送信フローでは、owner_type=anonymous の draft shipment を作成する。
	// 仮置きとして shipment は常に作成し、status=uploading で返却する。
	expiresAt := time.Now().UTC().Add(15 * time.Minute)
	shipment, err := h.Queries.CreateShipment(r.Context(), store.CreateShipmentParams{
		OwnerType:    "anonymous",
		OwnerUserID:  nil,
		Status:       "uploading",
		ShareMode:    "recipient_restricted",
		Title:        "(仮置き) untitled",
		Message:      nil,
		MaxDownloads: 10,
		ExpiresAt:    time.Now().UTC().Add(7 * 24 * time.Hour),
	})
	if err != nil {
		render.Error(w, http.StatusInternalServerError, "create_shipment_failed", "shipment作成に失敗しました", chimw.GetReqID(r.Context()))
		return
	}

	storageKey := "uploads/" + shipment.ID.String() + "/" + uuid.NewString() + "/" + req.FileName
	file, err := h.Queries.CreateFile(r.Context(), store.CreateFileParams{
		ShipmentID:     shipment.ID,
		OriginalName:   req.FileName,
		SizeBytes:      req.SizeBytes,
		MimeType:       req.MimeType,
		StorageBucket:  h.S3Bucket,
		StorageKey:     storageKey,
		ChecksumSha256: req.ChecksumSHA256,
		UploadStatus:   "initiated",
	})
	if err != nil {
		render.Error(w, http.StatusInternalServerError, "create_file_failed", "file作成に失敗しました", chimw.GetReqID(r.Context()))
		return
	}

	upload, err := h.Queries.CreateUploadSession(r.Context(), store.CreateUploadSessionParams{
		ShipmentID:        &shipment.ID,
		FileID:            &file.ID,
		StorageBucket:     h.S3Bucket,
		StorageKey:        storageKey,
		MultipartUploadID: "TODO-multipart-upload-id",
		PartSizeBytes:     defaultPartSizeBytes,
		Status:            "initiated",
		ExpiresAt:         expiresAt,
	})
	if err != nil {
		render.Error(w, http.StatusInternalServerError, "create_upload_session_failed", "upload_session作成に失敗しました", chimw.GetReqID(r.Context()))
		return
	}

	// TODO: 次PRで ObjectStore インターフェースを使い、実際の presigned URL を生成する。
	render.JSON(w, http.StatusCreated, map[string]any{
		"upload_session_id": upload.ID,
		"shipment_id":       shipment.ID,
		"file_id":           file.ID,
		"part_size_bytes":   upload.PartSizeBytes,
		"presigned_parts": []map[string]any{
			{"part_number": 1, "url": "https://example.invalid/presigned-part-1"},
		},
		"expires_at": upload.ExpiresAt,
	})
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

	upload, err := h.Queries.GetUploadSession(r.Context(), uploadID)
	if err != nil {
		render.Error(w, http.StatusNotFound, "upload_session_not_found", "upload session が見つかりません", chimw.GetReqID(r.Context()))
		return
	}

	// TODO: 次PRで parts整合性検証 + S3 CompleteMultipartUpload 実行 + files.upload_status 更新を実装する。
	render.JSON(w, http.StatusOK, map[string]any{
		"file_id":         upload.FileID,
		"upload_status":   "completed",
		"shipment_status": "ready",
	})
}
