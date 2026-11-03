package httpapi

import (
	"encoding/json"
	"net/http"
	"time"
)

type healthResponse struct {
	Status    string    `json:"status"`
	Service   string    `json:"service"`
	Timestamp time.Time `json:"timestamp"`
}

func NewRouter() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", live)
	mux.HandleFunc("GET /health", live)
	return securityHeaders(mux)
}

func live(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(healthResponse{
		Status:    "ok",
		Service:   "tg-account-manager-api",
		Timestamp: time.Now().UTC(),
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}
