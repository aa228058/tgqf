package messages

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/aa228058/tgqf/backend/internal/auth"
)

func (h *Handler) keywordRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		user := auth.UserFromContext(r.Context())
		rules, err := h.service.ListKeywordRules(r.Context(), user.TenantID)
		if err != nil {
			messageJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load keyword rules"})
			return
		}
		messageJSON(w, http.StatusOK, map[string]any{"rules": rules})
	case http.MethodPost:
		var rule KeywordRule
		if err := decodeMessageJSON(w, r, 32<<10, &rule); err != nil {
			messageJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid keyword rule body"})
			return
		}
		user := auth.UserFromContext(r.Context())
		created, err := h.service.CreateKeywordRule(r.Context(), user.TenantID, rule)
		if err != nil {
			status := http.StatusBadRequest
			if err.Error() == "active keyword rule limit reached (100)" {
				status = http.StatusConflict
			}
			messageJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		messageJSON(w, http.StatusCreated, map[string]any{"rule": created})
	default:
		messageJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *Handler) keywordRule(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	id := r.PathValue("id")
	switch r.Method {
	case http.MethodPut:
		var rule KeywordRule
		if err := decodeMessageJSON(w, r, 32<<10, &rule); err != nil {
			messageJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid keyword rule body"})
			return
		}
		updated, err := h.service.UpdateKeywordRule(r.Context(), user.TenantID, id, rule)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, ErrInvalidKeywordRule) {
				status = http.StatusBadRequest
			} else if err.Error() == "keyword rule not found" {
				status = http.StatusNotFound
			} else if err.Error() == "active keyword rule limit reached (100)" {
				status = http.StatusConflict
			}
			messageJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		messageJSON(w, http.StatusOK, map[string]any{"rule": updated})
	case http.MethodDelete:
		if err := h.service.DeleteKeywordRule(r.Context(), user.TenantID, id); err != nil {
			messageJSON(w, http.StatusNotFound, map[string]string{"error": "keyword rule not found"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		messageJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *Handler) keywordMatches(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.service.ListKeywordMatches(r.Context(), user.TenantID, r.URL.Query().Get("rule_id"), limit, offset)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrInvalidKeywordRule) {
			status = http.StatusBadRequest
		}
		messageJSON(w, status, map[string]string{"error": "failed to load keyword matches"})
		return
	}
	messageJSON(w, http.StatusOK, map[string]any{"matches": items, "total": total, "limit": normalizeLimit(limit), "offset": max(0, offset)})
}

func (h *Handler) keywordActions(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.service.ListKeywordActions(r.Context(), user.TenantID, r.URL.Query().Get("status"), limit, offset)
	if err != nil {
		messageJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	messageJSON(w, http.StatusOK, map[string]any{"actions": items, "total": total, "limit": normalizeLimit(limit), "offset": max(0, offset)})
}

func (h *Handler) keywordActionControl(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	action := r.PathValue("action")
	if action != "pause" && action != "resume" && action != "retry" {
		messageJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported keyword action"})
		return
	}
	if err := h.service.ChangeKeywordActionStatus(r.Context(), user.TenantID, r.PathValue("id"), action); err != nil {
		status := http.StatusConflict
		if err.Error() == "keyword action not found" {
			status = http.StatusNotFound
		}
		messageJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	messageJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
