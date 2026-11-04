package auth

import (
    "net/http"
)

func (h *Handler) systemHealth(w http.ResponseWriter, r *http.Request) {
    snapshot, err := h.service.SystemHealth(r.Context())
    if err != nil {
        writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"system health collection incomplete","health":snapshot})
        return
    }
    status := "healthy"
    if snapshot.HasWarnings() { status = "warning" }
    writeJSON(w, http.StatusOK, map[string]any{"status":status,"health":snapshot})
}
