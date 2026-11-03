package auth

import (
 "context"
 "encoding/json"
 "errors"
 "net/http"
 "strings"
)
type contextKey string
const userKey contextKey="auth_user";const sessionKey contextKey="auth_session_id"
type Handler struct{service *Service};func NewHandler(s *Service)*Handler{return &Handler{service:s}}
type registerRequest struct{Username string `json:"username"`;Password string `json:"password"`;TenantName string `json:"tenant_name"`};type loginRequest struct{Username string `json:"username"`;Password string `json:"password"`;DeviceLabel string `json:"device_label"`};type activateRequest struct{Code string `json:"code"`};type passwordRequest struct{CurrentPassword string `json:"current_password"`;NewPassword string `json:"new_password"`}
func(h *Handler)Routes(m *http.ServeMux){m.HandleFunc("POST /api/v1/auth/register",h.register);m.HandleFunc("POST /api/v1/auth/login",h.login);m.Handle("POST /api/v1/auth/activate",h.requireAuth(http.HandlerFunc(h.activate)));m.Handle("POST /api/v1/auth/logout",h.requireAuth(http.HandlerFunc(h.logout)));m.Handle("POST /api/v1/auth/password",h.requireAuth(http.HandlerFunc(h.changePassword)));m.Handle("GET /api/v1/auth/me",h.requireAuth(http.HandlerFunc(h.me)))}
func(h *Handler)register(w http.ResponseWriter,r *http.Request){var q registerRequest;if !decode(w,r,&q){return};u,e:=h.service.Register(r.Context(),q.Username,q.Password,q.TenantName);if e!=nil{writeError(w,e);return};writeJSON(w,201,map[string]any{"user":u,"activation_required":true})}
func(h *Handler)login(w http.ResponseWriter,r *http.Request){var q loginRequest;if !decode(w,r,&q){return};s,e:=h.service.Login(r.Context(),q.Username,q.Password,q.DeviceLabel);if e!=nil{writeError(w,e);return};writeJSON(w,200,map[string]any{"access_token":s.Token,"token_type":"Bearer","expires_at":s.ExpiresAt,"user":s.User})}
func(h *Handler)activate(w http.ResponseWriter,r *http.Request){var q activateRequest;if !decode(w,r,&q){return};u:=r.Context().Value(userKey).(User);expiry,e:=h.service.Activate(r.Context(),u.TenantID,q.Code);if e!=nil{writeError(w,e);return};writeJSON(w,200,map[string]any{"status":"activated","expires_at":expiry})}
func(h *Handler)logout(w http.ResponseWriter,r *http.Request){if e:=h.service.Logout(r.Context(),r.Context().Value(sessionKey).(string));e!=nil{writeError(w,e);return};w.WriteHeader(204)}
func(h *Handler)changePassword(w http.ResponseWriter,r *http.Request){var q passwordRequest;if !decode(w,r,&q){return};u:=r.Context().Value(userKey).(User);sid:=r.Context().Value(sessionKey).(string);if e:=h.service.ChangePassword(r.Context(),u.ID,q.CurrentPassword,q.NewPassword,sid);e!=nil{writeError(w,e);return};writeJSON(w,200,map[string]string{"status":"password_changed","other_sessions":"revoked"})}
func(h *Handler)me(w http.ResponseWriter,r *http.Request){writeJSON(w,200,r.Context().Value(userKey))}
func(h *Handler)requireAuth(next http.Handler)http.Handler{return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){head:=strings.TrimSpace(r.Header.Get("Authorization"));token:="";if strings.HasPrefix(head,"Bearer "){token=strings.TrimSpace(strings.TrimPrefix(head,"Bearer "))};u,sid,e:=h.service.Authenticate(r.Context(),token);if e!=nil{writeJSON(w,401,map[string]string{"error":"unauthorized"});return};if r.URL.Path!="/api/v1/auth/activate"&&!h.service.CanOperate(r.Context(),u.TenantID){writeJSON(w,403,map[string]string{"error":"subscription inactive; activate or renew your card"});return};ctx:=context.WithValue(r.Context(),userKey,u);ctx=context.WithValue(ctx,sessionKey,sid);next.ServeHTTP(w,r.WithContext(ctx))})}
func decode(w http.ResponseWriter,r *http.Request,v any)bool{r.Body=http.MaxBytesReader(w,r.Body,1<<20);d:=json.NewDecoder(r.Body);d.DisallowUnknownFields();if e:=d.Decode(v);e!=nil{writeJSON(w,400,map[string]string{"error":"invalid request body"});return false};return true}
func writeJSON(w http.ResponseWriter,status int,v any){w.Header().Set("Content-Type","application/json; charset=utf-8");w.WriteHeader(status);_=json.NewEncoder(w).Encode(v)}
func writeError(w http.ResponseWriter,e error){switch{case errors.Is(e,ErrInvalidCredentials):writeJSON(w,401,map[string]string{"error":e.Error()});case errors.Is(e,ErrAccountInactive):writeJSON(w,403,map[string]string{"error":e.Error()});case errors.Is(e,ErrCardInvalid):writeJSON(w,422,map[string]string{"error":e.Error()});case errors.Is(e,ErrUsernameTaken):writeJSON(w,409,map[string]string{"error":e.Error()});default:writeJSON(w,400,map[string]string{"error":e.Error()})}}
