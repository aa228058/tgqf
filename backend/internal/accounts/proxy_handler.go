package accounts

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/aa228058/tgqf/backend/internal/auth"
)

type bindProxyRequest struct {
	ProxyID string `json:"proxy_id"`
}

func (h *Handler) listProxies(w http.ResponseWriter,r *http.Request) {
	u:=auth.UserFromContext(r.Context())
	items,err:=h.service.ListProxies(r.Context(),u.TenantID)
	if err!=nil{writeError(w,http.StatusInternalServerError,"failed to list proxies");return}
	writeJSON(w,http.StatusOK,map[string]any{"proxies":items})
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
		if strings.Contains(err.Error(),"proxy not found"){writeError(w,http.Status404NotFound,err.Error());return}
		writeError(w,http.StatusBadRequest,err.Error());return
	}
	writeJSON(w,http.StatusOK,map[string]string{"proxy_id":input.ProxyID})
}

func (h *Handler) checkProxy(w http.ResponseWriter,r *http.Request) {
	u:=auth.UserFromContext(r.Context())
	ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second);defer cancel()
	if err:=h.service.CheckProxy(ctx,u.TenantID,r.PathValue("id"));err!=nil{
		if strings.Contains(err.Error(),"proxy not found"){writeError(w,http.StatusNotFound,err.Error());return}
		writeJSON(w,http.StatusBadGateway,map[string]any{"reachable":false,"message":"proxy TCP connection failed"})
		return
	}
	writeJSON(w,http.StatusOK,map[string]any{"reachable":true,"check":"tcp"})
}
