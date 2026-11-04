package auth

import (
 "context"
 "encoding/json"
 "errors"
 "net/http"
 "net"
 "os"
 "log/slog"
 "strconv"
 "strings"
 "time"
)
type contextKey string
const userKey contextKey="auth_user";const sessionKey contextKey="auth_session_id"
type Handler struct{service *Service};func NewHandler(s *Service)*Handler{return &Handler{service:s}}
type registerRequest struct{Username string `json:"username"`;Password string `json:"password"`;TenantName string `json:"tenant_name"`};type loginRequest struct{Username string `json:"username"`;Password string `json:"password"`;DeviceLabel string `json:"device_label"`};type activateRequest struct{Code string `json:"code"`};type passwordRequest struct{CurrentPassword string `json:"current_password"`;NewPassword string `json:"new_password"`}
type issueCardsRequest struct{Count int `json:"count"`;DurationDays int `json:"duration_days"`};type tenantLimitRequest struct{AccountLimit int `json:"account_limit"`};type tenantExpiryRequest struct{ExpiresAt time.Time `json:"expires_at"`};type tenantStatusRequest struct{Status string `json:"status"`}
func(h *Handler)Routes(m *http.ServeMux){m.HandleFunc("POST /api/v1/auth/register",h.register);m.HandleFunc("POST /api/v1/auth/login",h.login);m.Handle("POST /api/v1/auth/activate",h.requireAuth(http.HandlerFunc(h.activate)));m.Handle("POST /api/v1/auth/logout",h.requireAuth(http.HandlerFunc(h.logout)));m.Handle("POST /api/v1/auth/password",h.requireAuth(http.HandlerFunc(h.changePassword)));m.Handle("GET /api/v1/auth/me",h.requireAuth(http.HandlerFunc(h.me)));h.adminRoutes(m)}
func(h *Handler)register(w http.ResponseWriter,r *http.Request){if !h.rateLimit(w,r,"register",envInt("AUTH_REGISTER_LIMIT",5),envDuration("AUTH_REGISTER_WINDOW","10m")){return};var q registerRequest;if !decode(w,r,&q){return};u,e:=h.service.Register(r.Context(),q.Username,q.Password,q.TenantName);if e!=nil{writeError(w,e);return};if err:=h.service.Audit(r.Context(),u.TenantID,u.ID,"tenant.register","tenant",u.TenantID,map[string]any{"username":u.Username});err!=nil{slog.Error("audit log write failed","error",err)};writeJSON(w,201,map[string]any{"user":u,"activation_required":true})}
func(h *Handler)login(w http.ResponseWriter,r *http.Request){if !h.rateLimit(w,r,"login",envInt("AUTH_LOGIN_LIMIT",10),envDuration("AUTH_LOGIN_WINDOW","5m")){return};var q loginRequest;if !decode(w,r,&q){return};s,e:=h.service.Login(r.Context(),q.Username,q.Password,q.DeviceLabel);if e!=nil{writeError(w,e);return};if err:=h.service.Audit(r.Context(),s.User.TenantID,s.User.ID,"auth.login","session","",map[string]any{"device_label":safeLabel(q.DeviceLabel)});err!=nil{slog.Error("audit log write failed","error",err)};writeJSON(w,200,map[string]any{"access_token":s.Token,"token_type":"Bearer","expires_at":s.ExpiresAt,"user":s.User})}
func(h *Handler)activate(w http.ResponseWriter,r *http.Request){var q activateRequest;if !decode(w,r,&q){return};u:=r.Context().Value(userKey).(User);expiry,e:=h.service.Activate(r.Context(),u.TenantID,q.Code);if e!=nil{writeError(w,e);return};writeJSON(w,200,map[string]any{"status":"activated","expires_at":expiry})}
func(h *Handler)logout(w http.ResponseWriter,r *http.Request){if e:=h.service.Logout(r.Context(),r.Context().Value(sessionKey).(string));e!=nil{writeError(w,e);return};w.WriteHeader(204)}
func(h *Handler)changePassword(w http.ResponseWriter,r *http.Request){var q passwordRequest;if !decode(w,r,&q){return};u:=r.Context().Value(userKey).(User);sid:=r.Context().Value(sessionKey).(string);if e:=h.service.ChangePassword(r.Context(),u.ID,q.CurrentPassword,q.NewPassword,sid);e!=nil{writeError(w,e);return};writeJSON(w,200,map[string]string{"status":"password_changed","other_sessions":"revoked"})}
func(h *Handler)me(w http.ResponseWriter,r *http.Request){writeJSON(w,200,r.Context().Value(userKey))}
func(h *Handler)requireAuth(next http.Handler)http.Handler{return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){head:=strings.TrimSpace(r.Header.Get("Authorization"));token:="";if strings.HasPrefix(head,"Bearer "){token=strings.TrimSpace(strings.TrimPrefix(head,"Bearer "))};u,sid,e:=h.service.Authenticate(r.Context(),token);if e!=nil{writeJSON(w,401,map[string]string{"error":"unauthorized"});return};allowInactive:=r.URL.Path=="/api/v1/auth/activate"||r.URL.Path=="/api/v1/auth/logout"||r.URL.Path=="/api/v1/auth/password"||r.URL.Path=="/api/v1/auth/me";if !allowInactive&&!h.service.CanOperate(r.Context(),u.TenantID){writeJSON(w,403,map[string]string{"error":"subscription inactive; activate or renew your card"});return};ctx:=context.WithValue(r.Context(),userKey,u);ctx=context.WithValue(ctx,sessionKey,sid);next.ServeHTTP(w,r.WithContext(ctx))})}
func(h *Handler)requireSuperAdmin(next http.Handler)http.Handler{return h.requireAuth(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){u:=r.Context().Value(userKey).(User);if u.Role!="super_admin"{writeJSON(w,403,map[string]string{"error":"super_admin_required"});return};next.ServeHTTP(w,r)}))}
func(h *Handler)adminRoutes(m *http.ServeMux){m.Handle("POST /api/v1/admin/cards",h.requireSuperAdmin(http.HandlerFunc(h.issueCards)));m.Handle("GET /api/v1/admin/cards",h.requireSuperAdmin(http.HandlerFunc(h.listCards)));m.Handle("POST /api/v1/admin/cards/{id}/disable",h.requireSuperAdmin(http.HandlerFunc(h.disableCard)));m.Handle("GET /api/v1/admin/tenants",h.requireSuperAdmin(http.HandlerFunc(h.listTenants)));m.Handle("GET /api/v1/admin/workers",h.requireSuperAdmin(http.HandlerFunc(h.listWorkers)));m.Handle("GET /api/v1/admin/system-health",h.requireSuperAdmin(http.HandlerFunc(h.systemHealth)));m.Handle("GET /api/v1/admin/system-alerts",h.requireSuperAdmin(http.HandlerFunc(h.listSystemAlerts)));m.Handle("GET /api/v1/admin/audit-logs",h.requireSuperAdmin(http.HandlerFunc(h.listAuditLogs)));m.Handle("GET /api/v1/admin/system-config",h.requireSuperAdmin(http.HandlerFunc(h.listSystemConfigs)));m.Handle("PUT /api/v1/admin/system-config/{key}",h.requireSuperAdmin(http.HandlerFunc(h.updateSystemConfig)));m.Handle("PATCH /api/v1/admin/tenants/{id}/status",h.requireSuperAdmin(http.HandlerFunc(h.setTenantStatus)));m.Handle("PATCH /api/v1/admin/tenants/{id}/account-limit",h.requireSuperAdmin(http.HandlerFunc(h.setTenantLimit)));m.Handle("PATCH /api/v1/admin/tenants/{id}/expiry",h.requireSuperAdmin(http.HandlerFunc(h.setTenantExpiry)))}
func(h *Handler)issueCards(w http.ResponseWriter,r *http.Request){var q issueCardsRequest;if !decode(w,r,&q){return};u:=r.Context().Value(userKey).(User);v,e:=h.service.IssueCards(r.Context(),u.ID,q.Count,q.DurationDays);if e!=nil{writeError(w,e);return};if err:=h.service.Audit(r.Context(),"",u.ID,"admin.cards.issue","activation_cards","",map[string]any{"count":q.Count,"duration_days":q.DurationDays});err!=nil{slog.Error("audit log write failed","error",err)};writeJSON(w,201,map[string]any{"cards":v,"note":"codes are shown only once; save them securely"})}
func(h *Handler)listCards(w http.ResponseWriter,r *http.Request){n,_:=strconv.Atoi(r.URL.Query().Get("limit"));v,e:=h.service.ListCards(r.Context(),n);if e!=nil{writeError(w,e);return};writeJSON(w,200,map[string]any{"cards":v})}
func(h *Handler)disableCard(w http.ResponseWriter,r *http.Request){if e:=h.service.DisableCard(r.Context(),r.PathValue("id"));e!=nil{writeError(w,e);return};u:=r.Context().Value(userKey).(User);if err:=h.service.Audit(r.Context(),"",u.ID,"admin.card.disable","activation_card",r.PathValue("id"),map[string]any{});err!=nil{slog.Error("audit log write failed","error",err)};writeJSON(w,200,map[string]string{"status":"disabled"})}
func(h *Handler)listTenants(w http.ResponseWriter,r *http.Request){n,_:=strconv.Atoi(r.URL.Query().Get("limit"));v,e:=h.service.ListTenants(r.Context(),n);if e!=nil{writeError(w,e);return};writeJSON(w,200,map[string]any{"tenants":v})}
func(h *Handler)setTenantStatus(w http.ResponseWriter,r *http.Request){var q tenantStatusRequest;if !decode(w,r,&q){return};if e:=h.service.SetTenantStatus(r.Context(),r.PathValue("id"),strings.TrimSpace(q.Status));e!=nil{writeError(w,e);return};u:=r.Context().Value(userKey).(User);if err:=h.service.Audit(r.Context(),"",u.ID,"admin.tenant.status","tenant",r.PathValue("id"),map[string]any{"status":q.Status});err!=nil{slog.Error("audit log write failed","error",err)};writeJSON(w,200,map[string]string{"status":q.Status})}
func(h *Handler)setTenantLimit(w http.ResponseWriter,r *http.Request){var q tenantLimitRequest;if !decode(w,r,&q){return};if e:=h.service.SetTenantAccountLimit(r.Context(),r.PathValue("id"),q.AccountLimit);e!=nil{writeError(w,e);return};u:=r.Context().Value(userKey).(User);if err:=h.service.Audit(r.Context(),"",u.ID,"admin.tenant.account_limit","tenant",r.PathValue("id"),map[string]any{"account_limit":q.AccountLimit});err!=nil{slog.Error("audit log write failed","error",err)};writeJSON(w,200,map[string]int{"account_limit":q.AccountLimit})}
func(h *Handler)setTenantExpiry(w http.ResponseWriter,r *http.Request){var q tenantExpiryRequest;if !decode(w,r,&q){return};if q.ExpiresAt.IsZero(){writeJSON(w,400,map[string]string{"error":"expires_at is required"});return};if e:=h.service.SetTenantExpiry(r.Context(),r.PathValue("id"),q.ExpiresAt);e!=nil{writeError(w,e);return};u:=r.Context().Value(userKey).(User);if err:=h.service.Audit(r.Context(),"",u.ID,"admin.tenant.expiry","tenant",r.PathValue("id"),map[string]any{"expires_at":q.ExpiresAt});err!=nil{slog.Error("audit log write failed","error",err)};writeJSON(w,200,map[string]time.Time{"expires_at":q.ExpiresAt})}
func decode(w http.ResponseWriter,r *http.Request,v any)bool{r.Body=http.MaxBytesReader(w,r.Body,1<<20);d:=json.NewDecoder(r.Body);d.DisallowUnknownFields();if e:=d.Decode(v);e!=nil{writeJSON(w,400,map[string]string{"error":"invalid request body"});return false};return true}
func writeJSON(w http.ResponseWriter,status int,v any){w.Header().Set("Content-Type","application/json; charset=utf-8");w.WriteHeader(status);_=json.NewEncoder(w).Encode(v)}
func writeError(w http.ResponseWriter,e error){switch{case errors.Is(e,ErrInvalidCredentials):writeJSON(w,401,map[string]string{"error":e.Error()});case errors.Is(e,ErrAccountInactive):writeJSON(w,403,map[string]string{"error":e.Error()});case errors.Is(e,ErrCardInvalid):writeJSON(w,422,map[string]string{"error":e.Error()});case errors.Is(e,ErrUsernameTaken):writeJSON(w,409,map[string]string{"error":e.Error()});default:writeJSON(w,400,map[string]string{"error":e.Error()})}}

func(h *Handler)rateLimit(w http.ResponseWriter,r *http.Request,scope string,limit int,window time.Duration)bool{
 ip,_,err:=net.SplitHostPort(r.RemoteAddr);if err!=nil{ip=r.RemoteAddr}
 ctx,cancel:=context.WithTimeout(r.Context(),2*time.Second);defer cancel()
 allowed,err:=h.service.AllowRequest(ctx,scope,ip,limit,window)
 if err!=nil{writeJSON(w,http.StatusServiceUnavailable,map[string]string{"error":"rate limiter unavailable"});return false}
 if !allowed{w.Header().Set("Retry-After",strconv.Itoa(int(window.Seconds())));writeJSON(w,http.StatusTooManyRequests,map[string]string{"error":"too many requests; try again later"});return false}
 return true
}

func(h *Handler)Authenticated(next http.Handler)http.Handler{return h.requireAuth(next)}
func UserFromContext(ctx context.Context)User{u,_:=ctx.Value(userKey).(User);return u}

func envInt(key string,fallback int)int{v:=strings.TrimSpace(os.Getenv(key));if v==""{return fallback};n,e:=strconv.Atoi(v);if e!=nil||n<1{return fallback};return n}
func envDuration(key,fallback string)time.Duration{v:=strings.TrimSpace(os.Getenv(key));if v==""{v=fallback};d,e:=time.ParseDuration(v);if e!=nil||d<=0{d,_=time.ParseDuration(fallback)};return d}


func (h *Handler) listWorkers(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.service.ListWorkerInstances(r.Context(), limit)
	if err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error":"failed to load worker status"}); return }
	stale := 0
	for _, item := range items { if item.Stale { stale++ } }
	writeJSON(w, http.StatusOK, map[string]any{"workers":items,"total":len(items),"stale":stale,"stale_after_seconds":45})
}


func (h *Handler) listAuditLogs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.service.ListAuditLogs(r.Context(), r.URL.Query().Get("tenant_id"), r.URL.Query().Get("action"), limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load audit logs"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": items, "total": total, "limit": limit, "offset": offset})
}

type systemConfigUpdateRequest struct { Value json.RawMessage `json:"value"` }

func (h *Handler) listSystemConfigs(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListSystemConfigs(r.Context())
	if err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error":"failed to load system configuration"}); return }
	writeJSON(w, http.StatusOK, map[string]any{"configs":items})
}

func (h *Handler) updateSystemConfig(w http.ResponseWriter, r *http.Request) {
	var q systemConfigUpdateRequest
	if !decode(w, r, &q) { return }
	if len(q.Value) == 0 { writeJSON(w, http.StatusBadRequest, map[string]string{"error":"value is required"}); return }
	actor := UserFromContext(r.Context())
	item, err := h.service.UpdateSystemConfig(r.Context(), actor.ID, r.PathValue("key"), q.Value)
	if err != nil { writeError(w, err); return }
	writeJSON(w, http.StatusOK, map[string]any{"config":item, "effective":"immediate"})
}
