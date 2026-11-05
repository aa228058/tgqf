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
        messageJSON(w,http.StatusOK,map[string]any{"tasks":items,"total":total,"limit":normalizeLimit(limit),"offset":max(0,offset),"execution_enabled":true})
    case http.MethodPost:
        var input JoinTaskInput
        if err:=decodeMessageJSON(w,r,32<<10,&input);err!=nil{messageJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid join task body"});return}
        created,err:=h.service.CreateJoinTask(r.Context(),user.TenantID,user.ID,input)
        if err!=nil{status:=http.StatusBadRequest;if err.Error()=="tenant subscription inactive"{status=http.StatusForbidden};messageJSON(w,status,map[string]string{"error":err.Error()});return}
        messageJSON(w,http.StatusCreated,map[string]any{"task":created,"execution_enabled":true,"notice":"Public username and public-link targets are executed by the worker after explicit authorization. Public targets and encrypted private invite links are executed by the worker after explicit authorization. Telegram join requests remain pending until the worker synchronizes approval or decline status."})
    default:messageJSON(w,http.StatusMethodNotAllowed,map[string]string{"error":"method not allowed"})
    }
}

type bulkJoinTaskTarget struct {
 TargetGroupID int64 `json:"target_group_id,omitempty"`
 TargetLinkID int64 `json:"target_link_id,omitempty"`
 Label string `json:"label"`
}
type bulkJoinTaskRequest struct {
 NamePrefix string `json:"name_prefix"`
 Targets []bulkJoinTaskTarget `json:"targets"`
 AccountIDs []string `json:"account_ids"`
 AuthorizationConfirmed bool `json:"authorization_confirmed"`
 AuthorizationNote string `json:"authorization_note"`
}
type bulkJoinTaskResult struct {
 TargetGroupID int64 `json:"target_group_id,omitempty"`
 TargetLinkID int64 `json:"target_link_id,omitempty"`
 Label string `json:"label"`
 TaskID string `json:"task_id,omitempty"`
 Status string `json:"status"`
 Error string `json:"error,omitempty"`
}

func (h *Handler) bulkJoinTasks(w http.ResponseWriter,r *http.Request) {
 user:=auth.UserFromContext(r.Context())
 var input bulkJoinTaskRequest
 if err:=decodeMessageJSON(w,r,1<<20,&input);err!=nil { messageJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid bulk join task body"});return }
 input.NamePrefix=strings.TrimSpace(input.NamePrefix)
 input.AuthorizationNote=strings.TrimSpace(input.AuthorizationNote)
 if len(input.NamePrefix)<1||len(input.NamePrefix)>100||len(input.AuthorizationNote)>500||!input.AuthorizationConfirmed||len(input.Targets)<1||len(input.Targets)>500||len(input.AccountIDs)<1||len(input.AccountIDs)>20 {
  messageJSON(w,http.StatusBadRequest,map[string]string{"error":"bulk join requires 1-500 targets, 1-20 accounts, a task name and explicit authorization"});return
 }
 seen:=map[string]bool{}
 results:=make([]bulkJoinTaskResult,0,len(input.Targets))
 succeeded:=0
 for _,target:=range input.Targets {
  label:=strings.TrimSpace(target.Label)
  result:=bulkJoinTaskResult{TargetGroupID:target.TargetGroupID,TargetLinkID:target.TargetLinkID,Label:label}
  if (target.TargetGroupID>0)==(target.TargetLinkID>0) {
   result.Status="failed";result.Error="each target must contain exactly one group ID or link ID";results=append(results,result);continue
  }
  key:=fmt.Sprintf("g:%d",target.TargetGroupID)
  if target.TargetLinkID>0 { key=fmt.Sprintf("l:%d",target.TargetLinkID) }
  if seen[key] { result.Status="failed";result.Error="duplicate target";results=append(results,result);continue }
  seen[key]=true
  name:=input.NamePrefix+" - "+label
  if label=="" { name=fmt.Sprintf("%s - %s",input.NamePrefix,key) }
  if len([]rune(name))>120 { name=string([]rune(name)[:120]) }
  task,err:=h.service.CreateJoinTask(r.Context(),user.TenantID,user.ID,JoinTaskInput{
   Name:name,TargetGroupID:target.TargetGroupID,TargetLinkID:target.TargetLinkID,
   AccountIDs:input.AccountIDs,AuthorizationConfirmed:input.AuthorizationConfirmed,
   AuthorizationNote:input.AuthorizationNote,
  })
  if err!=nil { result.Status="failed";result.Error=err.Error() } else { result.Status="created";result.TaskID=task.ID;succeeded++ }
  results=append(results,result)
 }
 status:=http.StatusCreated
 if succeeded!=len(results) { status=http.StatusMultiStatus }
 if succeeded==0 { status=http.StatusBadRequest }
 messageJSON(w,status,map[string]any{"total":len(results),"succeeded":succeeded,"failed":len(results)-succeeded,"results":results,"execution_enabled":true,"notice":"Created tasks remain drafts and must be reviewed and started manually."})
}

func (h *Handler) joinTask(w http.ResponseWriter,r *http.Request) {
    user:=auth.UserFromContext(r.Context());id:=r.PathValue("id")
    if r.Method!=http.MethodGet{messageJSON(w,http.StatusMethodNotAllowed,map[string]string{"error":"method not allowed"});return}
    detail,err:=h.service.GetJoinTask(r.Context(),user.TenantID,id)
    if err!=nil{messageJSON(w,http.StatusNotFound,map[string]string{"error":"join task not found"});return}
    messageJSON(w,http.StatusOK,map[string]any{"task":detail,"execution_enabled":true})
}

func (h *Handler) joinTaskAction(w http.ResponseWriter,r *http.Request) {
    user:=auth.UserFromContext(r.Context());id:=r.PathValue("id")
    var err error
    switch r.PathValue("action") {
    case "start":err=h.service.StartJoinTask(r.Context(),user.TenantID,id)
    case "pause":err=h.service.PauseJoinTask(r.Context(),user.TenantID,id)
    case "resume":err=h.service.ResumeJoinTask(r.Context(),user.TenantID,id)
    case "cancel":err=h.service.CancelJoinTask(r.Context(),user.TenantID,id)
    case "retry":err=h.service.RetryJoinTask(r.Context(),user.TenantID,id)
    default:messageJSON(w,http.StatusNotFound,map[string]string{"error":"unknown task action"});return
    }
    if err!=nil{status:=http.StatusConflict;if err.Error()=="join task not found"{status=http.StatusNotFound};if err.Error()=="tenant subscription inactive"{status=http.StatusForbidden};messageJSON(w,status,map[string]string{"error":err.Error()});return}
    status:="updated"
    switch r.PathValue("action"){case "start":status="queued";case "pause":status="paused";case "resume":status="queued";case "cancel":status="cancelled";case "retry":status="queued"}
    messageJSON(w,http.StatusOK,map[string]any{"status":status,"execution_enabled":true})
}

