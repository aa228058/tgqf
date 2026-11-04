package messages

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/aa228058/tgqf/backend/internal/auth"
)

const mediaAssetUploadLimit = MaxKeywordMediaAssetBytes + (1 << 20)

func (h *Handler) keywordMediaAssets(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	switch r.Method {
	case http.MethodGet:
		items, err := h.service.ListMediaAssets(r.Context(), user.TenantID, 100)
		if err != nil {
			messageJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list media assets"})
			return
		}
		messageJSON(w, http.StatusOK, map[string]any{"assets": items})
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, mediaAssetUploadLimit)
		if err := r.ParseMultipartForm(mediaAssetUploadLimit); err != nil {
			messageJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or oversized multipart upload"})
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			messageJSON(w, http.StatusBadRequest, map[string]string{"error": "multipart field 'file' is required"})
			return
		}
		defer file.Close()
		content, err := io.ReadAll(io.LimitReader(file, MaxKeywordMediaAssetBytes+1))
		if err != nil {
			messageJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read upload"})
			return
		}
		if len(content) == 0 || len(content) > MaxKeywordMediaAssetBytes {
			messageJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "media asset must be between 1 byte and 20 MiB"})
			return
		}
		name := filepath.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
		if name == "." || name == string(filepath.Separator) {
			messageJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid file name"})
			return
		}
		detected := http.DetectContentType(content)
		mediaType, _, _ := mime.ParseMediaType(detected)
		if !strings.HasPrefix(mediaType, "image/") && !strings.HasPrefix(mediaType, "video/") && !strings.HasPrefix(mediaType, "audio/") && mediaType != "application/pdf" && mediaType != "application/zip" && mediaType != "application/octet-stream" {
			messageJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "unsupported media content type"})
			return
		}
		declared, _, _ := mime.ParseMediaType(header.Header.Get("Content-Type"))
		if declared != "" && declared != "application/octet-stream" && declared != mediaType {
			messageJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "declared content type does not match uploaded content"})
			return
		}
		asset, err := h.service.CreateMediaAsset(r.Context(), user.TenantID, name, mediaType, content)
		if err != nil {
			messageJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("failed to store media asset: %v", err)})
			return
		}
		messageJSON(w, http.StatusCreated, map[string]any{"asset": asset})
	default:
		messageJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *Handler) keywordMediaAsset(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	id := r.PathValue("id")
	switch r.Method {
	case http.MethodGet:
		asset, err := h.service.GetMediaAsset(r.Context(), user.TenantID, id)
		if err == ErrMediaAssetNotFound {
			messageJSON(w, http.StatusNotFound, map[string]string{"error": "media asset not found"})
			return
		}
		if err != nil {
			messageJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load media asset"})
			return
		}
		w.Header().Set("Content-Type", asset.MIMEType)
		w.Header().Set("Content-Length", fmt.Sprint(len(asset.Content)))
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": asset.FileName}))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(asset.Content)
	case http.MethodDelete:
		if err := h.service.DeleteMediaAsset(r.Context(), user.TenantID, id); err != nil {
			if err == ErrMediaAssetNotFound {
				messageJSON(w, http.StatusNotFound, map[string]string{"error": "media asset not found"})
			} else {
				messageJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete media asset"})
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		messageJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}
