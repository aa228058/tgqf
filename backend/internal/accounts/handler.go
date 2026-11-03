package accounts

import (
 "errors"
 "encoding/json"
 "encoding/csv"
 "fmt"
 "io"
 "net/http"
 "strconv"
 "strings"
 "time"
 "context"
 "bytes"
 "image"
 _ "image/jpeg"
 _ "image/png"
 "github.com/aa228058/tgqf/backend/internal/telegrammtgo"

 "github.com/aa228058/tgqf/backend/internal/auth"
)
type Handler struct{service *Service;auth *auth.Handler;telegram *telegrammtgo.Manager}
type createRequest struct{Phone string `json:"phone"`;DisplayName string `json:"display_name"`;GroupName string `json:"group_name"`;Session string `json:"session"`;Proxy json.RawMessage `json:"proxy"`}
type batchImportRequest struct{Accounts []createRequest `json:"accounts"`}
type groupRequest struct{GroupName string `json:"group_name"`}
type bulkGroupRequest struct{AccountIDs []string `json:"account_ids"`;GroupName string `json:"group_name"`}
type bulkRuntimeActionRequest struct{AccountIDs []string `json:"account_ids"`;Action string `json:"action"`}
type qrStartRequest struct{GroupName string `json:"group_name"`;Proxy json.RawMessage `json:"proxy"`}
type phoneStartRequest struct{Phone string `json:"phone"`;GroupName string `json:"group_name"`;Proxy json.RawMessage `json:"proxy"`}
type phoneCodeRequest struct{Code string `json:"code"`}
type phonePasswordRequest struct{Password string `json:"password"`}
type bulkPhoneStartItem struct { Phone string `json:"phone"`; GroupName string `json:"group_name"`; Proxy json.RawMessage `json:"proxy"` }
type bulkPhoneStartRequest struct { Accounts []bulkPhoneStartItem `json:"accounts"` }
type bulkPhoneCancelRequest struct { AccountIDs []string `json:"account_ids"` }
type bulkPhoneVerifyItem struct { AccountID string `json:"account_id"`; Code string `json:"code"`; Password string `json:"password"` }
type bulkPhoneVerifyRequest struct { Accounts []bulkPhoneVerifyItem `json:"accounts"` }
type bulkPhoneStartResult struct { Phone string `json:"phone"`; Account *Account `json:"account,omitempty"`; Status string `json:"status"`; DeliveryType string `json:"delivery_type,omitempty"`; ResendAfterSeconds int `json:"resend_after_seconds,omitempty"`; Error string `json:"error,omitempty"` }
func NewHandler(s *Service,a *auth.Handler,m *telegrammtgo.Manager)*Handler{return &Handler{service:s,auth:a,telegram:m}}
func(h *Handler)Routes(m *http.ServeMux){
 m.Handle("GET /api/v1/accounts",h.auth.Authenticated(http.HandlerFunc(h.list)))
 m.Handle("GET /api/v1/accounts/export.csv",h.auth.Authenticated(http.HandlerFunc(h.exportCSV)))
 m.Handle("POST /api/v1/accounts",h.auth.Authenticated(http.HandlerFunc(h.create)))
 m.Handle("POST /api/v1/accounts/import",h.auth.Authenticated(http.HandlerFunc(h.importBatch)))
 m.Handle("POST /api/v1/accounts/import-file",h.auth.Authenticated(http.HandlerFunc(h.importFile)))
 m.Handle("GET /api/v1/accounts/username-generation-settings",h.auth.Authenticated(http.HandlerFunc(h.getUsernameGenerationSettings)))
 m.Handle("PUT /api/v1/accounts/username-generation-settings",h.auth.Authenticated(http.HandlerFunc(h.saveUsernameGenerationSettings)))
 m.Handle("POST /api/v1/accounts/profile-updates/photo",h.auth.Authenticated(http.HandlerFunc(h.createProfilePhotoUpdates)))
 m.Handle("POST /api/v1/accounts/profile-updates",h.auth.Authenticated(http.HandlerFunc(h.createProfileUpdates)))
 m.Handle("GET /api/v1/accounts/profile-updates",h.auth.Authenticated(http.HandlerFunc(h.listProfileUpdates)))
 m.Handle("POST /api/v1/accounts/profile-updates/{id}/retry",h.auth.Authenticated(http.HandlerFunc(h.retryProfileUpdate)))
 m.Handle("POST /api/v1/accounts/profile-updates/{id}/resume",h.auth.Authenticated(http.HandlerFunc(h.resumeProfileUpdate)))
 m.Handle("POST /api/v1/accounts/profile-updates/{id}/cancel",h.auth.Authenticated(http.HandlerFunc(h.cancelProfileUpdate)))
 m.Handle("DELETE /api/v1/accounts/{id}",h.auth.Authenticated(http.HandlerFunc(h.delete)))
 m.Handle("PATCH /api/v1/accounts/{id}/group",h.auth.Authenticated(http.HandlerFunc(h.setGroup)))
 m.Handle("PATCH /api/v1/accounts/group",h.auth.Authenticated(http.HandlerFunc(h.setGroupBatch)))
 m.Handle("PATCH /api/v1/accounts/proxy",h.auth.Authenticated(http.HandlerFunc(h.bindProxyBatch)))
 m.Handle("POST /api/v1/accounts/bulk-action",h.auth.Authenticated(http.HandlerFunc(h.bulkRuntimeAction)))
 m.Handle("PATCH /api/v1/accounts/{id}/proxy",h.auth.Authenticated(http.HandlerFunc(h.bindProxy)))
 m.Handle("GET /api/v1/proxies",h.auth.Authenticated(http.HandlerFunc(h.listProxies)))
 m.Handle("POST /api/v1/proxies",h.auth.Authenticated(http.HandlerFunc(h.createProxy)))
 m.Handle("PUT /api/v1/proxies/default",h.auth.Authenticated(http.HandlerFunc(h.setDefaultProxy)))
 m.Handle("PUT /api/v1/proxies/{id}",h.auth.Authenticated(http.HandlerFunc(h.updateProxy)))
 m.Handle("DELETE /api/v1/proxies/{id}",h.auth.Authenticated(http.HandlerFunc(h.deleteProxy)))
 m.Handle("POST /api/v1/proxies/{id}/check",h.auth.Authenticated(http.HandlerFunc(h.checkProxy)))
 if h.telegram!=nil { m.Handle("POST /api/v1/accounts/phone/bulk/start",h.auth.Authenticated(http.HandlerFunc(h.startBulkPhoneLogin))); m.Handle("POST /api/v1/accounts/phone/bulk/verify",h.auth.Authenticated(http.HandlerFunc(h.verifyBulkPhoneLogin))); m.Handle("POST /api/v1/accounts/phone/bulk/cancel",h.auth.Authenticated(http.HandlerFunc(h.cancelBulkPhoneLogin))); m.Handle("POST /api/v1/accounts/phone/start",h.auth.Authenticated(http.HandlerFunc(h.startPhoneLogin))); m.Handle("POST /api/v1/accounts/{id}/phone/verify",h.auth.Authenticated(http.HandlerFunc(h.verifyPhoneCode))); m.Handle("POST /api/v1/accounts/{id}/phone/2fa",h.auth.Authenticated(http.HandlerFunc(h.verifyPhonePassword))); m.Handle("POST /api/v1/accounts/qr/start",h.auth.Authenticated(http.HandlerFunc(h.startQR))); m.Handle("GET /api/v1/accounts/{id}/qr/status",h.auth.Authenticated(http.HandlerFunc(h.pollQR))); m.Handle("DELETE /api/v1/accounts/{id}/qr",h.auth.Authenticated(http.HandlerFunc(h.cancelQR))) }
}

func safeCSVCell(value string) string {
	if len(value)==0 { return value }
	switch value[0] { case '=', '+', '-', '@', '\t', '\r': return "'"+value }
	return value
}

func(h *Handler)exportCSV(w http.ResponseWriter,r *http.Request){
	u:=auth.UserFromContext(r.Context())
	query:=AccountListOptions{Query:r.URL.Query().Get("q"),GroupName:r.URL.Query().Get("group"),Status:r.URL.Query().Get("status"),Limit:100}
	items,total,err:=h.service.ListPage(r.Context(),u.TenantID,query)
	if err!=nil{writeError(w,http.StatusBadRequest,err.Error());return}
	w.Header().Set("Content-Type","text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",fmt.Sprintf("attachment; filename=telegram-accounts-%s.csv",time.Now().UTC().Format("20060102-150405")))
	w.WriteHeader(http.StatusOK)
	writer:=csv.NewWriter(w)
	_ = writer.Write([]string{"id","phone","display_name","group_name","status","has_session","has_proxy","last_error","created_at","updated_at"})
	for offset:=0;offset<total;offset+=100{
		if offset>0{query.Offset=offset;items,_,err=h.service.ListPage(r.Context(),u.TenantID,query);if err!=nil{return}}
		for _,a:=range items{_ = writer.Write([]string{safeCSVCell(a.ID),safeCSVCell(a.Phone),safeCSVCell(a.DisplayName),safeCSVCell(a.GroupName),safeCSVCell(a.Status),fmt.Sprint(a.HasSession),fmt.Sprint(a.HasProxy),safeCSVCell(a.LastError),a.CreatedAt.UTC().Format(time.RFC3339),a.UpdatedAt.UTC().Format(time.RFC3339)})}
	}
	writer.Flush()
}

func(h *Handler)list(w http.ResponseWriter,r *http.Request){u:=auth.UserFromContext(r.Context());limit,_:=strconv.Atoi(r.URL.Query().Get("limit"));offset,_:=strconv.Atoi(r.URL.Query().Get("offset"));items,total,err:=h.service.ListPage(r.Context(),u.TenantID,AccountListOptions{Query:r.URL.Query().Get("q"),GroupName:r.URL.Query().Get("group"),Status:r.URL.Query().Get("status"),Limit:limit,Offset:offset});if err!=nil{writeError(w,400,err.Error());return};if limit<1||limit>100{limit=50};if offset<0{offset=0};writeJSON(w,200,map[string]any{"accounts":items,"total":total,"limit":limit,"offset":offset})}

func(h *Handler)importBatch(w http.ResponseWriter,r *http.Request){
 var input batchImportRequest
 if !decode(w,r,&input){writeError(w,http.StatusBadRequest,"invalid import payload");return}
 h.importAccounts(w,r,input.Accounts)
}

func(h *Handler)create(w http.ResponseWriter,r *http.Request){var q createRequest;if !decode(w,r,&q){writeError(w,400,"invalid request body");return};u:=auth.UserFromContext(r.Context());a,err:=h.service.Create(r.Context(),u.TenantID,q.Phone,q.DisplayName,q.GroupName,q.Session,q.Proxy);if err!=nil{if strings.Contains(err.Error(),"account limit reached"){writeError(w,409,err.Error());return};if strings.Contains(err.Error(),"SESSION_ENCRYPTION_KEY"){writeError(w,503,"session encryption is not configured");return};writeError(w,400,err.Error());return};writeJSON(w,201,map[string]any{"account":a})}
func(h *Handler)delete(w http.ResponseWriter,r *http.Request){u:=auth.UserFromContext(r.Context());id:=r.PathValue("id");items,err:=h.service.List(r.Context(),u.TenantID);if err!=nil{writeError(w,500,"failed to verify account");return};found:=false;for _,a:=range items{if a.ID==id{found=true;break}};if !found{writeError(w,404,"account not found");return};if h.telegram!=nil{if err:=h.telegram.Stop(id);err!=nil{writeError(w,502,"could not stop account Telegram client");return}};if err:=h.service.Delete(r.Context(),u.TenantID,id);err!=nil{writeError(w,404,"account not found");return};w.WriteHeader(204)}
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
 _=h.telegram.CancelPhoneLogin(id)
 if err:=h.service.Delete(r.Context(),u.TenantID,id);err!=nil{writeError(w,500,"failed to cancel QR login");return}
 w.WriteHeader(http.StatusNoContent)
}

type profileUpdateBatchRequest struct { Accounts []ProfileUpdateInput `json:"accounts"` }


func (h *Handler) createProfilePhotoUpdates(w http.ResponseWriter,r *http.Request) {
	const maxPhotoBytes=5<<20
	r.Body=http.MaxBytesReader(w,r.Body,maxPhotoBytes+(64<<10))
	if err:=r.ParseMultipartForm(1<<20);err!=nil { writeError(w,http.StatusBadRequest,"invalid multipart upload or request exceeds size limit");return }
	if r.MultipartForm!=nil { defer r.MultipartForm.RemoveAll() }
	accountIDsRaw:=strings.TrimSpace(r.FormValue("account_ids"))
	var accountIDs []string
	if accountIDsRaw=="" || json.Unmarshal([]byte(accountIDsRaw),&accountIDs)!=nil || len(accountIDs)<1 || len(accountIDs)>20 {
		writeError(w,http.StatusBadRequest,"account_ids must be a JSON array containing 1-20 account IDs");return
	}
	file,header,err:=r.FormFile("photo")
	if err!=nil { writeError(w,http.StatusBadRequest,"photo file is required");return }
	defer file.Close()
	if header.Size<=0 || header.Size>maxPhotoBytes { writeError(w,http.StatusBadRequest,"photo must be between 1 byte and 5 MiB");return }
	data,err:=io.ReadAll(io.LimitReader(file,maxPhotoBytes+1))
	if err!=nil || len(data)==0 || len(data)>maxPhotoBytes { writeError(w,http.StatusBadRequest,"invalid photo or photo exceeds 5 MiB");return }
	cfg,format,err:=image.DecodeConfig(bytes.NewReader(data))
	if err!=nil || (format!="jpeg" && format!="png") { writeError(w,http.StatusBadRequest,"photo must be a valid JPEG or PNG image");return }
	if cfg.Width<1 || cfg.Height<1 || cfg.Width>4096 || cfg.Height>4096 || int64(cfg.Width)*int64(cfg.Height)>16_000_000 {
		writeError(w,http.StatusBadRequest,"photo dimensions must not exceed 4096x4096 or 16 megapixels");return
	}
	if _,decodedFormat,decodeErr:=image.Decode(bytes.NewReader(data));decodeErr!=nil || decodedFormat!=format {
		writeError(w,http.StatusBadRequest,"photo image data is corrupted");return
	}
	inputs:=make([]ProfileUpdateInput,0,len(accountIDs))
	for _,id:=range accountIDs { inputs=append(inputs,ProfileUpdateInput{AccountID:id,PhotoData:data}) }
	user:=auth.UserFromContext(r.Context())
	jobs,err:=h.service.CreateProfileUpdateBatch(r.Context(),user.TenantID,user.ID,inputs)
	if err!=nil {
		status:=http.StatusBadRequest
		if strings.Contains(err.Error(),"already active") { status=http.StatusConflict }
		if strings.Contains(err.Error(),"inactive") { status=http.StatusForbidden }
		writeError(w,status,err.Error());return
	}
	batchID:=""
	if len(jobs)>0 { batchID=jobs[0].BatchID }
	writeJSON(w,http.StatusAccepted,map[string]any{"batch_id":batchID,"jobs":jobs,"photo_format":format})
}

func (h *Handler) createProfileUpdates(w http.ResponseWriter,r *http.Request) {
	var body profileUpdateBatchRequest
	if !decode(w,r,&body) { writeError(w,http.StatusBadRequest,"invalid profile update batch");return }
	user:=auth.UserFromContext(r.Context())
	jobs,err:=h.service.CreateProfileUpdateBatch(r.Context(),user.TenantID,user.ID,body.Accounts)
	if err!=nil {
		status:=http.StatusBadRequest
		if strings.Contains(err.Error(),"already active") { status=http.StatusConflict }
		if strings.Contains(err.Error(),"inactive") { status=http.StatusForbidden }
		writeError(w,status,err.Error());return
	}
	batchID:=""
	if len(jobs)>0 { batchID=jobs[0].BatchID }
	writeJSON(w,http.StatusAccepted,map[string]any{"batch_id":batchID,"jobs":jobs})
}

func (h *Handler) listProfileUpdates(w http.ResponseWriter,r *http.Request) {
	user:=auth.UserFromContext(r.Context())
	limit,_:=strconv.Atoi(r.URL.Query().Get("limit"));offset,_:=strconv.Atoi(r.URL.Query().Get("offset"))
	jobs,total,err:=h.service.ListProfileUpdateJobs(r.Context(),user.TenantID,r.URL.Query().Get("batch_id"),r.URL.Query().Get("status"),limit,offset)
	if err!=nil { writeError(w,http.StatusBadRequest,err.Error());return }
	if limit<1||limit>100 { limit=50 };if offset<0 { offset=0 }
	writeJSON(w,http.StatusOK,map[string]any{"jobs":jobs,"total":total,"limit":limit,"offset":offset})
}

func (h *Handler) retryProfileUpdate(w http.ResponseWriter,r *http.Request) {
	user:=auth.UserFromContext(r.Context())
	if err:=h.service.RetryProfileUpdate(r.Context(),user.TenantID,r.PathValue("id"));err!=nil {
		status:=http.StatusConflict
		if strings.Contains(err.Error(),"not found") { status=http.StatusNotFound }
		if strings.Contains(err.Error(),"inactive") { status=http.StatusForbidden }
		writeError(w,status,err.Error());return
	}
	writeJSON(w,http.StatusOK,map[string]string{"status":"pending"})
}

func (h *Handler) resumeProfileUpdate(w http.ResponseWriter,r *http.Request) {
	user:=auth.UserFromContext(r.Context())
	if err:=h.service.ResumeProfileUpdate(r.Context(),user.TenantID,r.PathValue("id"));err!=nil {
		status:=http.StatusConflict
		if strings.Contains(err.Error(),"not found") { status=http.StatusNotFound }
		if strings.Contains(err.Error(),"inactive") { status=http.StatusForbidden }
		writeError(w,status,err.Error());return
	}
	writeJSON(w,http.StatusOK,map[string]string{"status":"pending"})
}

func (h *Handler) cancelProfileUpdate(w http.ResponseWriter,r *http.Request) {
 user:=auth.UserFromContext(r.Context())
 if err:=h.service.CancelProfileUpdate(r.Context(),user.TenantID,r.PathValue("id"));err!=nil {
  status:=http.StatusConflict
  if strings.Contains(err.Error(),"not found") {status=http.StatusNotFound}
  writeError(w,status,err.Error());return
 }
 writeJSON(w,http.StatusOK,map[string]string{"status":"cancelled"})
}

func (h *Handler) getUsernameGenerationSettings(w http.ResponseWriter,r *http.Request) {
	user:=auth.UserFromContext(r.Context())
	settings,err:=h.service.GetUsernameGenerationSettings(r.Context(),user.TenantID)
	if err!=nil { writeError(w,http.StatusInternalServerError,"failed to load username generation settings");return }
	writeJSON(w,http.StatusOK,map[string]any{"settings":settings})
}

func (h *Handler) saveUsernameGenerationSettings(w http.ResponseWriter,r *http.Request) {
	var settings UsernameGenerationSettings
	if !decode(w,r,&settings) { writeError(w,http.StatusBadRequest,"invalid username generation settings");return }
	user:=auth.UserFromContext(r.Context())
	saved,err:=h.service.SaveUsernameGenerationSettings(r.Context(),user.TenantID,settings)
	if err!=nil { writeError(w,http.StatusBadRequest,err.Error());return }
	writeJSON(w,http.StatusOK,map[string]any{"settings":saved})
}

func (h *Handler) setGroupBatch(w http.ResponseWriter, r *http.Request) {
	var input bulkGroupRequest
	if !decode(w, r, &input) { writeError(w, http.StatusBadRequest, "invalid request body"); return }
	user := auth.UserFromContext(r.Context())
	count, err := h.service.SetGroupBatch(r.Context(), user.TenantID, input.AccountIDs, input.GroupName)
	if err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
	writeJSON(w, http.StatusOK, map[string]any{"updated": count, "group_name": strings.TrimSpace(input.GroupName)})
}


func (h *Handler) startPhoneLogin(w http.ResponseWriter,r *http.Request) {
	var q phoneStartRequest
	if !decode(w,r,&q) { writeError(w,400,"invalid request body");return }
	u:=auth.UserFromContext(r.Context())
	a,err:=h.service.CreatePending(r.Context(),u.TenantID,q.GroupName,q.Proxy)
	if err!=nil {if strings.Contains(err.Error(),"account limit reached"){writeError(w,409,err.Error())}else{writeError(w,400,err.Error())};return}
	profile,err:=h.service.LoadDeviceProfile(r.Context(),u.TenantID,a.ID)
	if err!=nil {_=h.service.Delete(context.Background(),u.TenantID,a.ID);writeError(w,500,"could not load persisted device profile");return}
	proxyJSON,err:=h.service.LoadPendingProxyJSON(r.Context(),u.TenantID,a.ID)
	if err!=nil {_=h.service.Delete(context.Background(),u.TenantID,a.ID);writeError(w,500,"could not load account proxy configuration");return}
	result,err:=h.telegram.StartPhoneLogin(r.Context(),a.ID,q.Phone,proxyJSON,profile)
	if err!=nil {_=h.service.Delete(context.Background(),u.TenantID,a.ID);writeError(w,502,"could not start Telegram phone login");return}
	writeJSON(w,201,map[string]any{"account":a,"status":"code_sent","delivery_type":result.DeliveryType,"resend_after_seconds":result.Timeout})
}

func normalizeBulkPhoneStart(input bulkPhoneStartRequest) (bulkPhoneStartRequest,error) {
	if len(input.Accounts)<1 || len(input.Accounts)>20 { return input,errors.New("phone login batch must contain 1-20 accounts") }
	seen:=map[string]bool{}
	for i:=range input.Accounts {
		item:=&input.Accounts[i]
		item.Phone=strings.TrimSpace(item.Phone)
		item.GroupName=strings.TrimSpace(item.GroupName)
		if len(item.Phone)<5 || len(item.Phone)>32 || len(item.GroupName)>80 { return input,fmt.Errorf("account %d: invalid phone or group name",i+1) }
		for j,r:=range item.Phone { if r=='+' && j==0 { continue }; if r<'0'||r>'9' { return input,fmt.Errorf("account %d: phone must contain only digits and an optional leading plus",i+1) } }
		canonical:=strings.TrimPrefix(item.Phone,"+")
		if seen[canonical] { return input,errors.New("duplicate phone number in batch") }
		seen[canonical]=true
		if len(item.Proxy)>16384 { return input,fmt.Errorf("account %d: proxy configuration is too large",i+1) }
		if len(item.Proxy)>0 && string(item.Proxy)!="null" && string(item.Proxy)!="{}" { var value map[string]any; if err:=json.Unmarshal(item.Proxy,&value);err!=nil || value==nil { return input,fmt.Errorf("account %d: proxy must be a JSON object",i+1) } }
	}
	return input,nil
}

func normalizeBulkPhoneAccountIDs(ids []string)([]string,error) {
	if len(ids)<1 || len(ids)>20 { return nil,errors.New("phone login cancellation requires 1-20 account IDs") }
	out:=make([]string,0,len(ids));seen:=map[string]bool{}
	for _,id:=range ids { id=strings.TrimSpace(id);if !validAccountUUID(id)||seen[id] { return nil,errors.New("invalid or duplicate account ID") };seen[id]=true;out=append(out,id) }
	return out,nil
}

// startBulkPhoneLogin starts independent MTGo OTP challenges for a bounded batch.
// Each account is isolated: one Telegram failure does not discard other challenges.
func (h *Handler) startBulkPhoneLogin(w http.ResponseWriter,r *http.Request) {
	var input bulkPhoneStartRequest
	if !decode(w,r,&input) { writeError(w,http.StatusBadRequest,"invalid phone login batch");return }
	input,err:=normalizeBulkPhoneStart(input);if err!=nil { writeError(w,http.StatusBadRequest,err.Error());return }
	u:=auth.UserFromContext(r.Context())
	results:=make([]bulkPhoneStartResult,0,len(input.Accounts));started:=0
	for _,item:=range input.Accounts {
		result:=bulkPhoneStartResult{Phone:item.Phone,Status:"failed"}
		account,createErr:=h.service.CreatePending(r.Context(),u.TenantID,item.GroupName,item.Proxy)
		if createErr!=nil {
			result.Error="could not create pending account"
			if strings.Contains(createErr.Error(),"account limit reached") { result.Error=createErr.Error() }
			results=append(results,result);continue
		}
		profile,profileErr:=h.service.LoadDeviceProfile(r.Context(),u.TenantID,account.ID)
		proxyJSON,proxyErr:=h.service.LoadPendingProxyJSON(r.Context(),u.TenantID,account.ID)
		if profileErr!=nil || proxyErr!=nil {
			_ = h.service.Delete(context.Background(),u.TenantID,account.ID)
			result.Error="could not load persisted account settings";results=append(results,result);continue
		}
		challenge,startErr:=h.telegram.StartPhoneLogin(r.Context(),account.ID,item.Phone,proxyJSON,profile)
		if startErr!=nil {
			_ = h.telegram.CancelPhoneLogin(account.ID)
			_ = h.service.Delete(context.Background(),u.TenantID,account.ID)
			result.Error="Telegram could not start phone login";results=append(results,result);continue
		}
		result.Account=&account;result.Status="code_sent";result.DeliveryType=challenge.DeliveryType;result.ResendAfterSeconds=challenge.Timeout
		started++;results=append(results,result)
	}
	status:=http.StatusAccepted
	if started==0 { status=http.StatusBadGateway }
	writeJSON(w,status,map[string]any{"requested":len(input.Accounts),"started":started,"failed":len(input.Accounts)-started,"results":results,"next_step":"verify each account with POST /api/v1/accounts/{id}/phone/verify; use /phone/2fa when Telegram requests a password"})
}

func normalizeBulkPhoneVerify(input bulkPhoneVerifyRequest)(bulkPhoneVerifyRequest,error) {
	if len(input.Accounts)<1 || len(input.Accounts)>20 { return input,errors.New("phone verification batch must contain 1-20 accounts") }
	seen:=map[string]bool{}
	for i:=range input.Accounts {
		item:=&input.Accounts[i];item.AccountID=strings.TrimSpace(item.AccountID);item.Code=strings.TrimSpace(item.Code)
		if !validAccountUUID(item.AccountID)||seen[item.AccountID] { return input,fmt.Errorf("account %d: invalid or duplicate account ID",i+1) }
		seen[item.AccountID]=true
		if (item.Code=="")== (strings.TrimSpace(item.Password)=="") { return input,fmt.Errorf("account %d: provide exactly one of code or password",i+1) }
		if len(item.Code)>32||len(item.Password)>1024 { return input,fmt.Errorf("account %d: verification credential is too long",i+1) }
	}
	return input,nil
}

// verifyBulkPhoneLogin verifies independent OTP/2FA challenges and persists each
// authorized MTGo session separately. Credentials are never echoed in responses.
func (h *Handler) verifyBulkPhoneLogin(w http.ResponseWriter,r *http.Request) {
	var input bulkPhoneVerifyRequest
	if !decode(w,r,&input) { writeError(w,http.StatusBadRequest,"invalid phone verification batch");return }
	input,err:=normalizeBulkPhoneVerify(input);if err!=nil { writeError(w,http.StatusBadRequest,err.Error());return }
	u:=auth.UserFromContext(r.Context())
	results:=make([]map[string]any,0,len(input.Accounts));authorized:=0
	for _,item:=range input.Accounts {
		entry:=map[string]any{"account_id":item.AccountID,"status":"failed"}
		pending,checkErr:=h.service.IsPending(r.Context(),u.TenantID,item.AccountID)
		if checkErr!=nil || !pending { entry["error"]="pending account not found";results=append(results,entry);continue }
		var authResult telegrammtgo.PhoneAuthResult
		var needsPassword bool
		if item.Code!="" {
			authResult,needsPassword,err=h.telegram.VerifyPhoneCode(r.Context(),item.AccountID,item.Code)
			if err==nil && needsPassword { entry["status"]="two_factor_required";results=append(results,entry);continue }
		} else {
			authResult,err=h.telegram.VerifyPhonePassword(r.Context(),item.AccountID,item.Password)
		}
		if err!=nil { entry["error"]="Telegram verification failed";results=append(results,entry);continue }
		account,saveErr:=h.service.CompleteQRAuthorization(r.Context(),u.TenantID,item.AccountID,authResult.Phone,authResult.FirstName,authResult.SessionString)
		_ = h.telegram.Stop(item.AccountID)
		if saveErr!=nil { entry["error"]="Telegram authorization succeeded but account details could not be saved";results=append(results,entry);continue }
		entry["status"]="authorized";entry["account"]=account;entry["telegram_user"]=map[string]any{"user_id":authResult.UserID,"username":authResult.Username,"first_name":authResult.FirstName}
		authorized++;results=append(results,entry)
	}
	status:=http.StatusOK
	if authorized==0 { status=http.StatusMultiStatus }
	writeJSON(w,status,map[string]any{"requested":len(input.Accounts),"authorized":authorized,"pending_or_failed":len(input.Accounts)-authorized,"results":results})
}

// cancelBulkPhoneLogin removes only pending phone-login accounts belonging to the caller's tenant.
func (h *Handler) cancelBulkPhoneLogin(w http.ResponseWriter,r *http.Request) {
	var input bulkPhoneCancelRequest
	if !decode(w,r,&input) { writeError(w,http.StatusBadRequest,"invalid cancellation request");return }
	ids,err:=normalizeBulkPhoneAccountIDs(input.AccountIDs);if err!=nil { writeError(w,http.StatusBadRequest,err.Error());return }
	u:=auth.UserFromContext(r.Context())
	results:=make([]map[string]string,0,len(ids));cancelled:=0
	for _,id:=range ids {
		pending,checkErr:=h.service.IsPending(r.Context(),u.TenantID,id)
		if checkErr!=nil || !pending { results=append(results,map[string]string{"account_id":id,"status":"not_pending"});continue }
		if stopErr:=h.telegram.CancelPhoneLogin(id);stopErr!=nil { results=append(results,map[string]string{"account_id":id,"status":"stop_failed"});continue }
		if deleteErr:=h.service.Delete(r.Context(),u.TenantID,id);deleteErr!=nil { results=append(results,map[string]string{"account_id":id,"status":"delete_failed"});continue }
		cancelled++;results=append(results,map[string]string{"account_id":id,"status":"cancelled"})
	}
	writeJSON(w,http.StatusOK,map[string]any{"requested":len(ids),"cancelled":cancelled,"results":results})
}

func (h *Handler) verifyPhoneCode(w http.ResponseWriter,r *http.Request) {
	var q phoneCodeRequest
	if !decode(w,r,&q){writeError(w,400,"invalid request body");return}
	u:=auth.UserFromContext(r.Context());id:=r.PathValue("id")
	pending,err:=h.service.IsPending(r.Context(),u.TenantID,id)
	if err!=nil{writeError(w,500,"failed to verify pending account");return};if !pending{writeError(w,404,"pending account not found");return}
	result,needsPassword,err:=h.telegram.VerifyPhoneCode(r.Context(),id,q.Code)
	if err!=nil{writeError(w,400,err.Error());return}
	if needsPassword{writeJSON(w,202,map[string]any{"status":"two_factor_required"});return}
	account,err:=h.service.CompleteQRAuthorization(r.Context(),u.TenantID,id,result.Phone,result.FirstName,result.SessionString)
	_=h.telegram.Stop(id)
	if err!=nil{writeError(w,409,"Telegram authorization succeeded but account details could not be saved");return}
	writeJSON(w,200,map[string]any{"status":"authorized","account":account,"telegram_user":map[string]any{"user_id":result.UserID,"username":result.Username,"first_name":result.FirstName}})
}

func (h *Handler) verifyPhonePassword(w http.ResponseWriter,r *http.Request) {
	var q phonePasswordRequest
	if !decode(w,r,&q){writeError(w,400,"invalid request body");return}
	u:=auth.UserFromContext(r.Context());id:=r.PathValue("id")
	pending,err:=h.service.IsPending(r.Context(),u.TenantID,id)
	if err!=nil{writeError(w,500,"failed to verify pending account");return};if !pending{writeError(w,404,"pending account not found");return}
	result,err:=h.telegram.VerifyPhonePassword(r.Context(),id,q.Password)
	if err!=nil{writeError(w,400,err.Error());return}
	account,err:=h.service.CompleteQRAuthorization(r.Context(),u.TenantID,id,result.Phone,result.FirstName,result.SessionString)
	_=h.telegram.Stop(id)
	if err!=nil{writeError(w,409,"Telegram authorization succeeded but account details could not be saved");return}
	writeJSON(w,200,map[string]any{"status":"authorized","account":account,"telegram_user":map[string]any{"user_id":result.UserID,"username":result.Username,"first_name":result.FirstName}})
}


func (h *Handler) bulkRuntimeAction(w http.ResponseWriter, r *http.Request) {
	var input bulkRuntimeActionRequest
	if !decode(w, r, &input) { writeError(w, http.StatusBadRequest, "invalid request body"); return }
	user := auth.UserFromContext(r.Context())
	changed, err := h.service.ApplyBulkRuntimeAction(r.Context(), user.TenantID, input.AccountIDs, input.Action)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") { status = http.StatusNotFound }
		if strings.Contains(err.Error(), "cannot be") || strings.Contains(err.Error(), "no saved") { status = http.StatusConflict }
		writeError(w, status, err.Error()); return
	}
	writeJSON(w, http.StatusOK, map[string]any{"action": strings.ToLower(strings.TrimSpace(input.Action)), "requested": len(input.AccountIDs), "changed": changed, "status": map[string]string{"disconnect":"disconnected", "reconnect":"authorized"}[strings.ToLower(strings.TrimSpace(input.Action))]})
}
