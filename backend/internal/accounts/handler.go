package accounts

import (
 "encoding/json"
 "io"
 "net/http"
 "strconv"
 "strings"
 "context"
 "github.com/aa228058/tgqf/backend/internal/telegrammtgo"

 "github.com/aa228058/tgqf/backend/internal/auth"
)
type Handler struct{service *Service;auth *auth.Handler;telegram *telegrammtgo.Manager}
type createRequest struct{Phone string `json:"phone"`;DisplayName string `json:"display_name"`;GroupName string `json:"group_name"`;Session string `json:"session"`;Proxy json.RawMessage `json:"proxy"`}
type groupRequest struct{GroupName string `json:"group_name"`}
type qrStartRequest struct{GroupName string `json:"group_name"`;Proxy json.RawMessage `json:"proxy"`}
func NewHandler(s *Service,a *auth.Handler,m *telegrammtgo.Manager)*Handler{return &Handler{service:s,auth:a,telegram:m}}
func(h *Handler)Routes(m *http.ServeMux){
 m.Handle("GET /api/v1/accounts",h.auth.Authenticated(http.HandlerFunc(h.list)))
 m.Handle("POST /api/v1/accounts",h.auth.Authenticated(http.HandlerFunc(h.create)))
 m.Handle("DELETE /api/v1/accounts/{id}",h.auth.Authenticated(http.HandlerFunc(h.delete)))
 m.Handle("PATCH /api/v1/accounts/{id}/group",h.auth.Authenticated(http.HandlerFunc(h.setGroup)))
 m.Handle("PATCH /api/v1/accounts/{id}/proxy",h.auth.Authenticated(http.HandlerFunc(h.bindProxy)))
 m.Handle("GET /api/v1/proxies",h.auth.Authenticated(http.HandlerFunc(h.listProxies)))
 m.Handle("POST /api/v1/proxies",h.auth.Authenticated(http.HandlerFunc(h.createProxy)))
 m.Handle("PUT /api/v1/proxies/default",h.auth.Authenticated(http.HandlerFunc(h.setDefaultProxy)))
 m.Handle("PUT /api/v1/proxies/{id}",h.auth.Authenticated(http.HandlerFunc(h.updateProxy)))
 m.Handle("DELETE /api/v1/proxies/{id}",h.auth.Authenticated(http.HandlerFunc(h.deleteProxy)))
 m.Handle("POST /api/v1/proxies/{id}/check",h.auth.Authenticated(http.HandlerFunc(h.checkProxy)))
 if h.telegram!=nil { m.Handle("POST /api/v1/accounts/qr/start",h.auth.Authenticated(http.HandlerFunc(h.startQR))); m.Handle("GET /api/v1/accounts/{id}/qr/status",h.auth.Authenticated(http.HandlerFunc(h.pollQR))); m.Handle("DELETE /api/v1/accounts/{id}/qr",h.auth.Authenticated(http.HandlerFunc(h.cancelQR))) }
}
func(h *Handler)list(w http.ResponseWriter,r *http.Request){u:=auth.UserFromContext(r.Context());limit,_:=strconv.Atoi(r.URL.Query().Get("limit"));offset,_:=strconv.Atoi(r.URL.Query().Get("offset"));items,total,err:=h.service.ListPage(r.Context(),u.TenantID,AccountListOptions{Query:r.URL.Query().Get("q"),GroupName:r.URL.Query().Get("group"),Status:r.URL.Query().Get("status"),Limit:limit,Offset:offset});if err!=nil{writeError(w,400,err.Error());return};if limit<1||limit>100{limit=50};if offset<0{offset=0};writeJSON(w,200,map[string]any{"accounts":items,"total":total,"limit":limit,"offset":offset})}
func(h *Handler)create(w http.ResponseWriter,r *http.Request){var q createRequest;if !decode(w,r,&q){writeError(w,400,"invalid request body");return};u:=auth.UserFromContext(r.Context());a,err:=h.service.Create(r.Context(),u.TenantID,q.Phone,q.DisplayName,q.GroupName,q.Session,q.Proxy);if err!=nil{if strings.Contains(err.Error(),"account limit reached"){writeError(w,409,err.Error());return};if strings.Contains(err.Error(),"SESSION_ENCRYPTION_KEY"){writeError(w,503,"session encryption is not configured");return};writeError(w,400,err.Error());return};writeJSON(w,201,map[string]any{"account":a})}
func(h *Handler)delete(w http.ResponseWriter,r *http.Request){u:=auth.UserFromContext(r.Context());if err:=h.service.Delete(r.Context(),u.TenantID,r.PathValue("id"));err!=nil{writeError(w,404,"account not found");return};w.WriteHeader(204)}
func(h *Handler)setGroup(w http.ResponseWriter,r *http.Request){var q groupRequest;if !decode(w,r,&q){writeError(w,400,"invalid request body");return};u:=auth.UserFromContext(r.Context());if err:=h.service.SetGroup(r.Context(),u.TenantID,r.PathValue("id"),q.GroupName);err!=nil{if strings.Contains(err.Error(),"account not found"){writeError(w,404,"account not found");return};writeError(w,400,err.Error());return};writeJSON(w,200,map[string]string{"group_name":strings.TrimSpace(q.GroupName)})}
func decode(w http.ResponseWriter,r *http.Request,v any)bool{
 r.Body=http.MaxBytesReader(w,r.Body,1<<20)
 d:=json.NewDecoder(r.Body)
 d.DisallowUnknownFields()
 if err:=d.Decode(v);err!=nil{return false}
 var extra any
 return d.Decode(&extra)==io.EOF
}
func writeJSON(w http.ResponseWriter,status int,v any){w.Header().Set("Content-Type","application/json; charset=utf-8");w.WriteHeader(status);_=json.NewEncoder(w).Encode(v)}
func writeError(w http.ResponseWriter,status int,message string){writeJSON(w,status,map[string]string{"error":message})}

func (h *Handler) startQR(w http.ResponseWriter,r *http.Request) {
 var q qrStartRequest;if !decode(w,r,&q){writeError(w,400,"invalid request body");return}
 u:=auth.UserFromContext(r.Context())
 a,err:=h.service.CreatePending(r.Context(),u.TenantID,q.GroupName,q.Proxy)
 if err!=nil {if strings.Contains(err.Error(),"account limit reached"){writeError(w,409,err.Error())}else{writeError(w,400,err.Error())};return}
 profile,profileErr:=h.service.LoadDeviceProfile(r.Context(),u.TenantID,a.ID)
  if profileErr!=nil {_=h.service.Delete(context.Background(),u.TenantID,a.ID);writeError(w,500,"could not load persisted device profile");return}
  proxyJSON,proxyErr:=h.service.LoadPendingProxyJSON(r.Context(),u.TenantID,a.ID)
  if proxyErr!=nil {_=h.service.Delete(context.Background(),u.TenantID,a.ID);writeError(w,500,"could not load account proxy configuration");return}
  result,err:=h.telegram.StartQR(r.Context(),a.ID,proxyJSON,profile)
 if err!=nil {_=h.service.Delete(context.Background(),u.TenantID,a.ID);writeError(w,502,"could not start Telegram QR login");return}
 if !result.Pending { authorizedAccount,saveErr:=h.service.CompleteQRAuthorization(r.Context(),u.TenantID,a.ID,result.Phone,strings.TrimSpace(result.FirstName),result.SessionString); _=h.telegram.Stop(a.ID); if saveErr!=nil { writeError(w,409,"Telegram authorization completed but account details could not be saved");return }; writeJSON(w,201,map[string]any{"account":authorizedAccount,"status":"authorized"});return }
 writeJSON(w,201,map[string]any{"account":a,"qr":map[string]any{"url":result.Challenge.URL,"expires_at":result.Challenge.Expires}})
}
func (h *Handler) pollQR(w http.ResponseWriter,r *http.Request) {
 u:=auth.UserFromContext(r.Context());id:=r.PathValue("id")
 pending,err:=h.service.IsPending(r.Context(),u.TenantID,id);if err!=nil{writeError(w,500,"failed to verify account");return}
 if !pending{writeError(w,404,"pending account not found");return}
 result,err:=h.telegram.PollQR(r.Context(),id);if err!=nil{writeError(w,502,"Telegram QR status check failed");return}
 if result.Pending{writeJSON(w,200,map[string]any{"status":"pending","qr":map[string]any{"url":result.Challenge.URL,"expires_at":result.Challenge.Expires}});return}
 _,saveErr:=h.service.CompleteQRAuthorization(r.Context(),u.TenantID,id,result.Phone,strings.TrimSpace(result.FirstName),result.SessionString)
 _=h.telegram.Stop(id)
 if saveErr!=nil { writeError(w,409,"Telegram authorization succeeded but account details could not be saved");return }
 writeJSON(w,200,map[string]any{"status":"authorized","telegram_user":map[string]any{"user_id":result.UserID,"username":result.Username,"first_name":result.FirstName}})
}
func (h *Handler) cancelQR(w http.ResponseWriter,r *http.Request) {
 u:=auth.UserFromContext(r.Context());id:=r.PathValue("id")
 items,err:=h.service.List(r.Context(),u.TenantID);if err!=nil{writeError(w,500,"failed to verify account");return}
 found:=false;for _,a:=range items{if a.ID==id&&a.Status=="pending"{found=true;break}}
 if !found{writeError(w,404,"pending account not found");return}
 _=h.telegram.CancelQR(id)
 if err:=h.service.Delete(r.Context(),u.TenantID,id);err!=nil{writeError(w,500,"failed to cancel QR login");return}
 w.WriteHeader(http.StatusNoContent)
}
