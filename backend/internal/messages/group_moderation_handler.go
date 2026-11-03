package messages

import (
    "errors"
    "net/http"
    "strconv"

    "github.com/aa228058/tgqf/backend/internal/auth"
)

type reviewGroupCorrectionRequest struct {
    Decision string `json:"decision"`
    Note string `json:"note"`
}
type mergePublicGroupRequest struct {
    TargetGroupID int64 `json:"target_group_id"`
    Reason string `json:"reason"`
}

func (h *Handler) groupCorrectionRoutes(mux *http.ServeMux) {
    mux.Handle("GET /api/v1/groups/corrections",h.auth.Authenticated(http.HandlerFunc(h.listTenantGroupCorrections)))
    mux.Handle("POST /api/v1/groups/{id}/corrections",h.auth.Authenticated(http.HandlerFunc(h.submitGroupCorrection)))
    mux.Handle("GET /api/v1/admin/group-corrections",h.auth.Authenticated(http.HandlerFunc(h.listAdminGroupCorrections)))
    mux.Handle("POST /api/v1/admin/group-corrections/{id}/review",h.auth.Authenticated(http.HandlerFunc(h.reviewGroupCorrection)))
    mux.Handle("POST /api/v1/admin/groups/{id}/merge",h.auth.Authenticated(http.HandlerFunc(h.mergePublicGroups)))
}

func (h *Handler) listTenantGroupCorrections(w http.ResponseWriter,r *http.Request) {
    user:=auth.UserFromContext(r.Context())
    if user.Role!="tenant_admin" {messageJSON(w,http.StatusForbidden,map[string]string{"error":"tenant_admin_required"});return}
    limit,_:=strconv.Atoi(r.URL.Query().Get("limit"));offset,_:=strconv.Atoi(r.URL.Query().Get("offset"))
    items,total,err:=h.service.ListTenantGroupCorrections(r.Context(),user.TenantID,limit,offset)
    if err!=nil {messageJSON(w,http.StatusInternalServerError,map[string]string{"error":"failed to load group corrections"});return}
    messageJSON(w,http.StatusOK,map[string]any{"corrections":items,"total":total,"limit":normalizeLimit(limit),"offset":max(0,offset)})
}

func (h *Handler) submitGroupCorrection(w http.ResponseWriter,r *http.Request) {
    user:=auth.UserFromContext(r.Context())
    if user.Role!="tenant_admin" {messageJSON(w,http.StatusForbidden,map[string]string{"error":"tenant_admin_required"});return}
    groupID,err:=strconv.ParseInt(r.PathValue("id"),10,64)
    if err!=nil||groupID<=0 {messageJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid group id"});return}
    var request struct { ProposedFields GroupCorrectionFields `json:"proposed_fields"` }
    if err:=decodeMessageJSON(w,r,16*1024,&request);err!=nil {messageJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid request body"});return}
    item,err:=h.service.SubmitGroupCorrection(r.Context(),user.TenantID,user.ID,groupID,request.ProposedFields)
    if err!=nil {
        status:=http.StatusInternalServerError
        if errors.Is(err,ErrInvalidGroupCorrection){status=http.StatusBadRequest}
        if errors.Is(err,ErrGroupCorrectionNotFound){status=http.StatusNotFound}
        messageJSON(w,status,map[string]string{"error":err.Error()});return
    }
    messageJSON(w,http.StatusCreated,map[string]any{"correction":item,"status":"pending_review"})
}

func (h *Handler) listAdminGroupCorrections(w http.ResponseWriter,r *http.Request) {
    user:=auth.UserFromContext(r.Context())
    if user.Role!="super_admin" {messageJSON(w,http.StatusForbidden,map[string]string{"error":"super_admin_required"});return}
    limit,_:=strconv.Atoi(r.URL.Query().Get("limit"));offset,_:=strconv.Atoi(r.URL.Query().Get("offset"))
    items,total,err:=h.service.ListAdminGroupCorrections(r.Context(),r.URL.Query().Get("status"),limit,offset)
    if err!=nil {
        status:=http.StatusInternalServerError;if errors.Is(err,ErrInvalidGroupCorrection){status=http.StatusBadRequest}
        messageJSON(w,status,map[string]string{"error":"failed to load moderation queue"});return
    }
    messageJSON(w,http.StatusOK,map[string]any{"corrections":items,"total":total,"limit":normalizeLimit(limit),"offset":max(0,offset)})
}

func (h *Handler) reviewGroupCorrection(w http.ResponseWriter,r *http.Request) {
    user:=auth.UserFromContext(r.Context())
    if user.Role!="super_admin" {messageJSON(w,http.StatusForbidden,map[string]string{"error":"super_admin_required"});return}
    id,err:=strconv.ParseInt(r.PathValue("id"),10,64)
    if err!=nil||id<=0 {messageJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid correction id"});return}
    var request reviewGroupCorrectionRequest
    if err:=decodeMessageJSON(w,r,8*1024,&request);err!=nil {messageJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid request body"});return}
    if err:=h.service.ReviewGroupCorrection(r.Context(),user.ID,id,request.Decision,request.Note);err!=nil {
        status:=http.StatusInternalServerError
        if errors.Is(err,ErrInvalidGroupCorrection){status=http.StatusBadRequest}
        if errors.Is(err,ErrGroupCorrectionNotFound){status=http.StatusNotFound}
        messageJSON(w,status,map[string]string{"error":err.Error()});return
    }
    messageJSON(w,http.StatusOK,map[string]string{"status":"reviewed"})
}

func (h *Handler) mergePublicGroups(w http.ResponseWriter,r *http.Request) {
    user:=auth.UserFromContext(r.Context())
    if user.Role!="super_admin" {messageJSON(w,http.StatusForbidden,map[string]string{"error":"super_admin_required"});return}
    sourceID,err:=strconv.ParseInt(r.PathValue("id"),10,64)
    if err!=nil||sourceID<=0 {messageJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid source group id"});return}
    var request mergePublicGroupRequest
    if err:=decodeMessageJSON(w,r,8*1024,&request);err!=nil {messageJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid request body"});return}
    if err:=h.service.MergePublicGroups(r.Context(),user.ID,sourceID,request.TargetGroupID,request.Reason);err!=nil {
        status:=http.StatusInternalServerError
        if errors.Is(err,ErrGroupMergeConflict){status=http.StatusBadRequest}
        messageJSON(w,status,map[string]string{"error":err.Error()});return
    }
    messageJSON(w,http.StatusOK,map[string]any{"status":"merged","source_group_id":sourceID,"target_group_id":request.TargetGroupID})
}
