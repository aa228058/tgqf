package messages

import (
    "net/http"
    "strconv"

    "github.com/aa228058/tgqf/backend/internal/auth"
)

func (h *Handler) joinTasks(w http.ResponseWriter,r *http.Request) {
    user:=auth.UserFromContext(r.Context())
    switch r.Method {
    case http.MethodGet:
        limit,_:=strconv.Atoi(r.URL.Query().Get("limit"));offset,_:=strconv.Atoi(r.URL.Query().Get("offset"))
        items,total,err:=h.service.ListJoinTasks(r.Context(),user.TenantID,limit,offset)
        if err!=nil{messageJSON(w,http.StatusInternalServerError,map[string]string{"error":"failed to load join tasks"});return}
        messageJSON(w,http.StatusOK,map[string]any{"tasks":items,"total":total,"limit":normalizeLimit(limit),"offset":max(0,offset)})
    case http.MethodPost:
        var input JoinTaskInput
        if err:=decodeMessageJSON(w,r,32<<10,&input);err!=nil{messageJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid join task body"});return}
        created,err:=h.service.CreateJoinTask(r.Context(),user.TenantID,user.ID,input)
        if err!=nil{status:=http.StatusBadRequest;if err.Error()=="tenant subscription inactive"{status=http.StatusForbidden};messageJSON(w,status,map[string]string{"error":err.Error()});return}
        messageJSON(w,http.StatusCreated,map[string]any{"task":created,"execution_enabled":false,"notice":"Task is saved for review; Telegram join execution is not enabled until the MTProto join and approval flow is verified."})
    default:messageJSON(w,http.StatusMethodNotAllowed,map[string]string{"error":"method not allowed"})
    }
}

func (h *Handler) joinTask(w http.ResponseWriter,r *http.Request) {
    user:=auth.UserFromContext(r.Context());id:=r.PathValue("id")
    if r.Method!=http.MethodGet{messageJSON(w,http.StatusMethodNotAllowed,map[string]string{"error":"method not allowed"});return}
    detail,err:=h.service.GetJoinTask(r.Context(),user.TenantID,id)
    if err!=nil{messageJSON(w,http.StatusNotFound,map[string]string{"error":"join task not found"});return}
    messageJSON(w,http.StatusOK,map[string]any{"task":detail,"execution_enabled":false})
}

func (h *Handler) joinTaskAction(w http.ResponseWriter,r *http.Request) {
    user:=auth.UserFromContext(r.Context());id:=r.PathValue("id")
    var err error
    switch r.PathValue("action") {
    case "start":err=h.service.StartJoinTask(r.Context(),user.TenantID,id)
    case "pause":err=h.service.PauseJoinTask(r.Context(),user.TenantID,id)
    case "resume":err=h.service.ResumeJoinTask(r.Context(),user.TenantID,id)
    case "cancel":err=h.service.CancelJoinTask(r.Context(),user.TenantID,id)
    default:messageJSON(w,http.StatusNotFound,map[string]string{"error":"unknown task action"});return
    }
    if err!=nil{status:=http.StatusConflict;if err.Error()=="join task not found"{status=http.StatusNotFound};if err.Error()=="tenant subscription inactive"{status=http.StatusForbidden};messageJSON(w,status,map[string]string{"error":err.Error()});return}
    status:="updated"
    switch r.PathValue("action"){case "start":status="queued";case "pause":status="paused";case "resume":status="queued";case "cancel":status="cancelled"}
    messageJSON(w,http.StatusOK,map[string]string{"status":status})
}

