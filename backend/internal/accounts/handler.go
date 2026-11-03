package accounts

import (
 "encoding/json"
 "net/http"
 "strings"

 "github.com/aa228058/tgqf/backend/internal/auth"
)
type Handler struct{service *Service;auth *auth.Handler}
type createRequest struct{Phone string `json:"phone"`;DisplayName string `json:"display_name"`;GroupName string `json:"group_name"`;Session string `json:"session"`;Proxy json.RawMessage `json:"proxy"`}
type groupRequest struct{GroupName string `json:"group_name"`}
func NewHandler(s *Service,a *auth.Handler)*Handler{return &Handler{service:s,auth:a}}
func(h *Handler)Routes(m *http.ServeMux){
 m.Handle("GET /api/v1/accounts",h.auth.Authenticated(http.HandlerFunc(h.list)))
 m.Handle("POST /api/v1/accounts",h.auth.Authenticated(http.HandlerFunc(h.create)))
 m.Handle("DELETE /api/v1/accounts/{id}",h.auth.Authenticated(http.HandlerFunc(h.delete)))
 m.Handle("PATCH /api/v1/accounts/{id}/group",h.auth.Authenticated(http.HandlerFunc(h.setGroup)))
}
func(h *Handler)list(w http.ResponseWriter,r *http.Request){u:=auth.UserFromContext(r.Context());items,err:=h.service.List(r.Context(),u.TenantID);if err!=nil{writeError(w,500,"failed to list accounts");return};writeJSON(w,200,map[string]any{"accounts":items})}
func(h *Handler)create(w http.ResponseWriter,r *http.Request){var q createRequest;if !decode(w,r,&q){writeError(w,400,"invalid request body");return};u:=auth.UserFromContext(r.Context());a,err:=h.service.Create(r.Context(),u.TenantID,q.Phone,q.DisplayName,q.GroupName,q.Session,q.Proxy);if err!=nil{if strings.Contains(err.Error(),"account limit reached"){writeError(w,409,err.Error());return};if strings.Contains(err.Error(),"SESSION_ENCRYPTION_KEY"){writeError(w,503,"session encryption is not configured");return};writeError(w,400,err.Error());return};writeJSON(w,201,map[string]any{"account":a})}
func(h *Handler)delete(w http.ResponseWriter,r *http.Request){u:=auth.UserFromContext(r.Context());if err:=h.service.Delete(r.Context(),u.TenantID,r.PathValue("id"));err!=nil{writeError(w,404,"account not found");return};w.WriteHeader(204)}
func(h *Handler)setGroup(w http.ResponseWriter,r *http.Request){var q groupRequest;if !decode(w,r,&q){writeError(w,400,"invalid request body");return};u:=auth.UserFromContext(r.Context());if err:=h.service.SetGroup(r.Context(),u.TenantID,r.PathValue("id"),q.GroupName);err!=nil{writeError(w,400,err.Error());return};writeJSON(w,200,map[string]string{"group_name":strings.TrimSpace(q.GroupName)})}
func decode(w http.ResponseWriter,r *http.Request,v any)bool{r.Body=http.MaxBytesReader(w,r.Body,1<<20);d:=json.NewDecoder(r.Body);d.DisallowUnknownFields();return d.Decode(v)==nil}
func writeJSON(w http.ResponseWriter,status int,v any){w.Header().Set("Content-Type","application/json; charset=utf-8");w.WriteHeader(status);_=json.NewEncoder(w).Encode(v)}
func writeError(w http.ResponseWriter,status int,message string){writeJSON(w,status,map[string]string{"error":message})}
