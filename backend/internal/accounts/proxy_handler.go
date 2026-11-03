package accounts

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/aa228058/tgqf/backend/internal/auth"
)

type bindProxyRequest struct {
	ProxyID string `json:"proxy_id"`
}
type bindProxyBatchRequest struct { AccountIDs []string `json:"account_ids"`; ProxyID string `json:"proxy_id"` }

func (h *Handler) listProxies(w http.ResponseWriter,r *http.Request) {
	u:=auth.UserFromContext(r.Context())
	items,err:=h.service.ListProxies(r.Context(),u.TenantID)
	if err!=nil{writeError(w,http.StatusInternalServerError,"failed to list proxies");return}
	writeJSON(w,http.StatusOK,map[string]any{"proxies":items})
}

func (h *Handler) listProxyChecks(w http.ResponseWriter,r *http.Request) {
	u:=auth.UserFromContext(r.Context())
	limit,_:=strconv.Atoi(r.URL.Query().Get("limit"))
	items,err:=h.service.ListProxyChecks(r.Context(),u.TenantID,r.PathValue("id"),limit)
	if err!=nil{if strings.Contains(err.Error(),"proxy not found"){writeError(w,http.StatusNotFound,err.Error());return};writeError(w,http.StatusInternalServerError,"failed to list proxy check history");return}
	writeJSON(w,http.StatusOK,map[string]any{"checks":items})
}
func (h *Handler) createProxy(w http.ResponseWriter,r *http.Request) {
	var input ProxyInput
	if !decode(w,r,&input){writeError(w,http.StatusBadRequest,"invalid request body");return}
	u:=auth.UserFromContext(r.Context())
	item,err:=h.service.CreateProxy(r.Context(),u.TenantID,input)
	if err!=nil{
		if strings.Contains(err.Error(),"SESSION_ENCRYPTION_KEY"){writeError(w,http.StatusServiceUnavailable,"proxy encryption is not configured");return}
		writeError(w,http.StatusBadRequest,err.Error());return
	}
	writeJSON(w,http.StatusCreated,map[string]any{"proxy":item})
}

func (h *Handler) deleteProxy(w http.ResponseWriter,r *http.Request) {
	u:=auth.UserFromContext(r.Context())
	err:=h.service.DeleteProxy(r.Context(),u.TenantID,r.PathValue("id"))
	if err!=nil{
		if strings.Contains(err.Error(),"assigned"){writeError(w,http.StatusConflict,err.Error());return}
		if errors.Is(err,context.Canceled){writeError(w,http.StatusRequestTimeout,"request cancelled");return}
		writeError(w,http.StatusNotFound,"proxy not found or could not be deleted");return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) bindProxy(w http.ResponseWriter,r *http.Request) {
	var input bindProxyRequest
	if !decode(w,r,&input){writeError(w,http.StatusBadRequest,"invalid request body");return}
	u:=auth.UserFromContext(r.Context())
	if err:=h.service.BindProxy(r.Context(),u.TenantID,r.PathValue("id"),strings.TrimSpace(input.ProxyID));err!=nil{
		if strings.Contains(err.Error(),"proxy not found") || strings.Contains(err.Error(),"account not found"){writeError(w,http.StatusNotFound,err.Error());return}
		writeError(w,http.StatusBadRequest,err.Error());return
	}
	writeJSON(w,http.StatusOK,map[string]string{"proxy_id":strings.TrimSpace(input.ProxyID)})
}

func (h *Handler) checkProxy(w http.ResponseWriter,r *http.Request) {
	u:=auth.UserFromContext(r.Context())
	ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second);defer cancel()
	if err:=h.service.CheckProxy(ctx,u.TenantID,r.PathValue("id"));err!=nil{
		if strings.Contains(err.Error(),"proxy not found"){writeError(w,http.StatusNotFound,err.Error());return}
		writeJSON(w,http.StatusBadGateway,map[string]any{"reachable":false,"message":"proxy protocol authentication or Telegram data-center tunnel failed"})
		return
	}
	writeJSON(w,http.StatusOK,map[string]any{"reachable":true,"check":"proxy_auth_and_telegram_dc_tcp"})
}

func (h *Handler) updateProxy(w http.ResponseWriter,r *http.Request) {
	var input ProxyInput
	if !decode(w,r,&input){writeError(w,http.StatusBadRequest,"invalid request body");return}
	u:=auth.UserFromContext(r.Context())
	item,err:=h.service.UpdateProxy(r.Context(),u.TenantID,r.PathValue("id"),input)
	if err!=nil{
		if strings.Contains(err.Error(),"SESSION_ENCRYPTION_KEY"){writeError(w,http.StatusServiceUnavailable,"proxy encryption is not configured");return}
		if errors.Is(err,sql.ErrNoRows){writeError(w,http.StatusNotFound,"proxy not found");return}
		writeError(w,http.StatusBadRequest,err.Error());return
	}
	writeJSON(w,http.StatusOK,map[string]any{"proxy":item})
}


func (h *Handler) setDefaultProxy(w http.ResponseWriter,r *http.Request) {
	var input bindProxyRequest
	if !decode(w,r,&input){writeError(w,http.StatusBadRequest,"invalid request body");return}
	u:=auth.UserFromContext(r.Context())
	id:=strings.TrimSpace(input.ProxyID)
	if err:=h.service.SetDefaultProxy(r.Context(),u.TenantID,id);err!=nil{
		if strings.Contains(err.Error(),"proxy not found"){writeError(w,http.StatusNotFound,err.Error());return}
		writeError(w,http.StatusBadRequest,"could not set default proxy");return
	}
	writeJSON(w,http.StatusOK,map[string]string{"proxy_id":id})
}


func (h *Handler) bindProxyBatch(w http.ResponseWriter,r *http.Request) {
	var input bindProxyBatchRequest
	if !decode(w,r,&input){writeError(w,http.StatusBadRequest,"invalid request body");return}
	u:=auth.UserFromContext(r.Context())
	count,err:=h.service.BindProxyBatch(r.Context(),u.TenantID,input.AccountIDs,input.ProxyID)
	if err!=nil{
		if strings.Contains(err.Error(),"proxy not found"){writeError(w,http.StatusNotFound,err.Error());return}
		writeError(w,http.StatusBadRequest,err.Error());return
	}
	writeJSON(w,http.StatusOK,map[string]any{"updated":count,"proxy_id":strings.TrimSpace(input.ProxyID)})
}
