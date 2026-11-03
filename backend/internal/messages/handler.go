package messages

import (
	"context"
	"errors"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
	"strconv"

	"github.com/aa228058/tgqf/backend/internal/auth"
)

func decodeMessageJSON(w http.ResponseWriter, r *http.Request, limit int64, dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("request body must contain exactly one JSON value")
		}
		return err
	}
	return nil
}

type Handler struct { service *Service; auth *auth.Handler }
func NewHandler(service *Service,authHandler *auth.Handler)*Handler{return &Handler{service:service,auth:authHandler}}
func(h *Handler)Routes(mux *http.ServeMux){
	mux.Handle("GET /api/v1/messages",h.auth.Authenticated(http.HandlerFunc(h.list)))
	mux.Handle("GET /api/v1/messages/events",h.auth.Authenticated(http.HandlerFunc(h.events)))
	mux.Handle("GET /api/v1/messages/chats",h.auth.Authenticated(http.HandlerFunc(h.listChats)))
	mux.Handle("GET /api/v1/groups",h.auth.Authenticated(http.HandlerFunc(h.listGroupLibrary)))
	mux.Handle("GET /api/v1/groups/{id}/sources",h.auth.Authenticated(http.HandlerFunc(h.listGroupSources)))
	mux.Handle("GET /api/v1/messages/settings",h.auth.Authenticated(http.HandlerFunc(h.getSettings)))
	mux.Handle("PUT /api/v1/messages/settings",h.auth.Authenticated(http.HandlerFunc(h.updateSettings)))
	mux.Handle("PATCH /api/v1/messages/{id}/processed",h.auth.Authenticated(http.HandlerFunc(h.markProcessed)))
	mux.Handle("POST /api/v1/messages/{id}/reply",h.auth.Authenticated(http.HandlerFunc(h.createReply)))
	mux.Handle("GET /api/v1/message-replies/{id}",h.auth.Authenticated(http.HandlerFunc(h.getReply)))
	mux.Handle("POST /api/v1/message-replies/{id}/resume",h.auth.Authenticated(http.HandlerFunc(h.resumeReply)))
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
	if err:=decodeMessageJSON(w,r,1<<20,&settings);err!=nil{messageJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid settings body"});return}
	user:=auth.UserFromContext(r.Context())
	updated,err:=h.service.UpdateSettings(r.Context(),user.TenantID,settings)
	if err!=nil{messageJSON(w,http.StatusBadRequest,map[string]string{"error":err.Error()});return}
	messageJSON(w,http.StatusOK,map[string]any{"settings":updated})
}

func(h *Handler)listChats(w http.ResponseWriter,r *http.Request){
	user:=auth.UserFromContext(r.Context())
	limit,_:=strconv.Atoi(r.URL.Query().Get("limit"));offset,_:=strconv.Atoi(r.URL.Query().Get("offset"))
	items,total,err:=h.service.ListChats(r.Context(),user.TenantID,limit,offset)
	if err!=nil{messageJSON(w,http.StatusInternalServerError,map[string]string{"error":"failed to load message chats"});return}
	messageJSON(w,http.StatusOK,map[string]any{"chats":items,"total":total,"limit":normalizeLimit(limit),"offset":max(0,offset)})
}

func(h *Handler)createReply(w http.ResponseWriter,r *http.Request){
	var body struct{Text string `json:"text"`}
	if err:=decodeMessageJSON(w,r,16<<10,&body);err!=nil{messageJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid reply body"});return}
	user:=auth.UserFromContext(r.Context())
	job,err:=h.service.CreateReply(r.Context(),user.TenantID,r.PathValue("id"),body.Text)
	if err!=nil{
		status:=http.StatusBadRequest
		if err.Error()=="message not found"{status=http.StatusNotFound}
		messageJSON(w,status,map[string]string{"error":err.Error()});return
	}
	messageJSON(w,http.StatusAccepted,map[string]any{"reply":job})
}


func(h *Handler)resumeReply(w http.ResponseWriter,r *http.Request){
	user:=auth.UserFromContext(r.Context())
	if err:=h.service.ResumeReply(r.Context(),user.TenantID,r.PathValue("id"));err!=nil{
		status:=http.StatusConflict
		if err.Error()=="reply job not found"{status=http.StatusNotFound}
		messageJSON(w,status,map[string]string{"error":err.Error()});return
	}
	messageJSON(w,http.StatusOK,map[string]string{"status":"resumed"})
}

func(h *Handler)getReply(w http.ResponseWriter,r *http.Request){
	user:=auth.UserFromContext(r.Context())
	job,err:=h.service.GetReply(r.Context(),user.TenantID,r.PathValue("id"))
	if err!=nil{messageJSON(w,http.StatusNotFound,map[string]string{"error":"reply job not found"});return}
	messageJSON(w,http.StatusOK,map[string]any{"reply":job})
}

func (h *Handler) events(w http.ResponseWriter,r *http.Request) {
 user:=auth.UserFromContext(r.Context())
 sub:=h.service.SubscribeMessages(r.Context(),user.TenantID)
 if sub==nil {
  messageJSON(w,http.StatusServiceUnavailable,map[string]string{"error":"realtime message stream is unavailable"})
  return
 }
 defer sub.Close()
 // Wait for Redis to confirm the tenant subscription before sending headers;
 // otherwise a message arriving during the handshake could be missed.
 subscribeCtx,cancelSubscribe:=context.WithTimeout(r.Context(),3*time.Second)
 _,subscribeErr:=sub.Receive(subscribeCtx)
 cancelSubscribe()
 if subscribeErr!=nil {
  messageJSON(w,http.StatusServiceUnavailable,map[string]string{"error":"realtime message stream could not subscribe"})
  return
 }
 flusher,ok:=w.(http.Flusher)
 if !ok { http.Error(w,"streaming is not supported",http.StatusInternalServerError);return }
 w.Header().Set("Content-Type","text/event-stream")
 w.Header().Set("Cache-Control","no-cache, no-transform")
 w.Header().Set("Connection","keep-alive")
 w.Header().Set("X-Accel-Buffering","no")
 w.WriteHeader(http.StatusOK)
 _,_=fmt.Fprint(w,": connected\n\n")
 flusher.Flush()
 keepalive:=time.NewTicker(25*time.Second)
 defer keepalive.Stop()
 events:=sub.Channel()
 for {
  select {
  case <-r.Context().Done(): return
  case event,open:=<-events:
   if !open { return }
   _,_=fmt.Fprintf(w,"event: message\ndata: %s\n\n",event.Payload)
   flusher.Flush()
  case <-keepalive.C:
   _,_=fmt.Fprint(w,": keep-alive\n\n")
   flusher.Flush()
  }
 }
}


func (h *Handler) listGroupSources(w http.ResponseWriter,r *http.Request) {
	user:=auth.UserFromContext(r.Context())
	id,err:=strconv.ParseInt(r.PathValue("id"),10,64)
	if err!=nil||id<=0 { messageJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid group id"});return }
	items,err:=h.service.ListGroupSources(r.Context(),user.TenantID,id)
	if err!=nil { messageJSON(w,http.StatusInternalServerError,map[string]string{"error":"failed to load group sources"});return }
	messageJSON(w,http.StatusOK,map[string]any{"sources":items})
}

func (h *Handler) listGroupLibrary(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.service.ListGroupLibrary(r.Context(), user.TenantID, r.URL.Query().Get("chat_type"), r.URL.Query().Get("q"), limit, offset)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrInvalidGroupLibraryQuery) { status = http.StatusBadRequest }
		messageJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	messageJSON(w, http.StatusOK, map[string]any{"groups": items, "total": total, "limit": normalizeLimit(limit), "offset": max(0, offset)})
}
