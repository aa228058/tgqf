package auth

import (
    "net/http"
    "log/slog"
)

func (h *Handler) systemHealth(w http.ResponseWriter, r *http.Request) {
    snapshot, err := h.service.SystemHealth(r.Context())
    if err != nil {
        writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error":"system health collection incomplete","health":snapshot})
        return
    }
    if syncErr:=h.service.SyncSystemAlerts(r.Context(),snapshot);syncErr!=nil { slog.Error("sync system alerts","error",syncErr) }
    status := "healthy"
    if snapshot.HasWarnings() { status = "warning" }
    writeJSON(w, http.StatusOK, map[string]any{"status":status,"health":snapshot})
}

func (h *Handler) listSystemAlerts(w http.ResponseWriter, r *http.Request) {
    items,err:=h.service.ListSystemAlerts(r.Context(),100)
    if err!=nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"failed to load system alerts"});return }
    active:=0
    for _,item:=range items { if item.Status=="active" {active++} }
    writeJSON(w,http.StatusOK,map[string]any{"alerts":items,"active":active})
}
