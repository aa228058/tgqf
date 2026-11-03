package httpapi

import (
 "context"
 "encoding/json"
 "net/http"
 "time"

 "github.com/aa228058/tgqf/backend/internal/auth"
 "github.com/aa228058/tgqf/backend/internal/accounts"
 "github.com/aa228058/tgqf/backend/internal/telegrammtgo"
 "github.com/aa228058/tgqf/backend/internal/messages"
)
type healthResponse struct { Status string `json:"status"`; Service string `json:"service"`; Timestamp time.Time `json:"timestamp"` }
type dependencyChecker interface { Ping(context.Context) error }
func NewRouter(deps dependencyChecker, authService *auth.Service, accountService *accounts.Service, messageService *messages.Service, telegramManager *telegrammtgo.Manager) http.Handler {
 mux:=http.NewServeMux();mux.HandleFunc("GET /health/live",live);mux.HandleFunc("GET /health",live)
 mux.HandleFunc("GET /health/ready",func(w http.ResponseWriter,r *http.Request){if deps==nil{http.Error(w,"dependencies are not configured",http.StatusServiceUnavailable);return};ctx,cancel:=context.WithTimeout(r.Context(),2*time.Second);defer cancel();if err:=deps.Ping(ctx);err!=nil{http.Error(w,"dependency check failed",http.StatusServiceUnavailable);return};w.Header().Set("Content-Type","application/json; charset=utf-8");_=json.NewEncoder(w).Encode(map[string]string{"status":"ready"})})
 if authService!=nil { ah:=auth.NewHandler(authService);ah.Routes(mux);if accountService!=nil{accounts.NewHandler(accountService,ah,telegramManager).Routes(mux)};if messageService!=nil{messages.NewHandler(messageService,ah).Routes(mux)} }
 return securityHeaders(mux)
}
func live(w http.ResponseWriter,_ *http.Request){w.Header().Set("Content-Type","application/json; charset=utf-8");w.WriteHeader(http.StatusOK);_=json.NewEncoder(w).Encode(healthResponse{Status:"ok",Service:"tg-account-manager-api",Timestamp:time.Now().UTC()})}
func securityHeaders(next http.Handler)http.Handler{return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.Header().Set("X-Content-Type-Options","nosniff");w.Header().Set("X-Frame-Options","DENY");w.Header().Set("Referrer-Policy","strict-origin-when-cross-origin");next.ServeHTTP(w,r)})}
