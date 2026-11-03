package messages

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/aa228058/tgqf/backend/internal/auth"
)

type Handler struct { service *Service; auth *auth.Handler }
func NewHandler(service *Service,authHandler *auth.Handler)*Handler{return &Handler{service:service,auth:authHandler}}
func(h *Handler)Routes(mux *http.ServeMux){
	mux.Handle("GET /api/v1/messages",h.auth.Authenticated(http.HandlerFunc(h.list)))
	mux.Handle("GET /api/v1/messages/settings",h.auth.Authenticated(http.HandlerFunc(h.getSettings)))
	mux.Handle("PUT /api/v1/messages/settings",h.auth.Authenticated(http.HandlerFunc(h.updateSettings)))
	mux.Handle("PATCH /api/v1/messages/{id}/processed",h.auth.Authenticated(http.HandlerFunc(h.markProcessed)))
}
func(h *Handler)list(w http.ResponseWriter,r *http.Request){
	user:=auth.UserFromContext(r.Context())
	limit,_:=strconv.Atoi(r.URL.Query().Get("limit"));offset,_:=strconv.Atoi(r.URL.Query().Get("offset"))
	include:=r.URL.Query().Get("include_processed")=="true"
	items,total,err:=h.service.List(r.Context(),user.TenantID,ListOptions{Limit:limit,Offset:offset,IncludeProcessed:include})
	if err!=nil{messageJSON(w,http.StatusInternalServerError,map[string]string{"error":"failed to load messages"});return}
	messageJSON(w,http.StatusOK,map[string]any{"messages":items,"total":total,"limit":normalizeLimit(limit),"offset":max(0,offset)})
}
func(h *Handler)markProcessed(w http.ResponseWriter,r *http.Request){
	user:=auth.UserFromContext(r.Context())
	if err:=h.service.MarkProcessed(r.Context(),user.TenantID,r.PathValue("id"));err!=nil{messageJSON(w,http.StatusNotFound,map[string]string{"error":"message not found"});return}
	w.WriteHeader(http.StatusNoContent)
}
func normalizeLimit(v int)int{if v<1||v>100{return 50};return v}
func messageJSON(w http.ResponseWriter,status int,value any){w.Header().Set("Content-Type","application/json; charset=utf-8");w.WriteHeader(status);_=json.NewEncoder(w).Encode(value)}

func(h *Handler)getSettings(w http.ResponseWriter,r *http.Request){
	user:=auth.UserFromContext(r.Context())
	settings,err:=h.service.GetSettings(r.Context(),user.TenantID)
	if err!=nil{messageJSON(w,http.StatusInternalServerError,map[string]string{"error":"failed to load message settings"});return}
	messageJSON(w,http.StatusOK,map[string]any{"settings":settings})
}

func(h *Handler)updateSettings(w http.ResponseWriter,r *http.Request){
	var settings CollectionSettings
	decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20));decoder.DisallowUnknownFields()
	if err:=decoder.Decode(&settings);err!=nil{messageJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid settings body"});return}
	user:=auth.UserFromContext(r.Context())
	updated,err:=h.service.UpdateSettings(r.Context(),user.TenantID,settings)
	if err!=nil{messageJSON(w,http.StatusBadRequest,map[string]string{"error":err.Error()});return}
	messageJSON(w,http.StatusOK,map[string]any{"settings":updated})
}
