package messages

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	mux.Handle("GET /api/v1/tasks",h.auth.Authenticated(http.HandlerFunc(h.unifiedTasks)))
	mux.Handle("GET /api/v1/tasks/summary",h.auth.Authenticated(http.HandlerFunc(h.unifiedTaskSummary)))
	mux.Handle("POST /api/v1/tasks/{type}/{id}/{action}",h.auth.Authenticated(http.HandlerFunc(h.unifiedTaskAction)))
	mux.Handle("POST /api/v1/tasks/bulk/{type}/{action}",h.auth.Authenticated(http.HandlerFunc(h.unifiedTaskBulkAction)))
	h.groupCorrectionRoutes(mux)
	mux.Handle("GET /api/v1/keyword-rules",h.auth.Authenticated(http.HandlerFunc(h.keywordRules)))
	mux.Handle("POST /api/v1/keyword-rules",h.auth.Authenticated(http.HandlerFunc(h.keywordRules)))
	mux.Handle("PUT /api/v1/keyword-rules/{id}",h.auth.Authenticated(http.HandlerFunc(h.keywordRule)))
	mux.Handle("DELETE /api/v1/keyword-rules/{id}",h.auth.Authenticated(http.HandlerFunc(h.keywordRule)))
	mux.Handle("GET /api/v1/keyword-rules/matches",h.auth.Authenticated(http.HandlerFunc(h.keywordMatches)))
	mux.Handle("GET /api/v1/keyword-actions",h.auth.Authenticated(http.HandlerFunc(h.keywordActions)))
	mux.Handle("GET /api/v1/keyword-media-assets",h.auth.Authenticated(http.HandlerFunc(h.keywordMediaAssets)))
	mux.Handle("POST /api/v1/keyword-media-assets",h.auth.Authenticated(http.HandlerFunc(h.keywordMediaAssets)))
	mux.Handle("GET /api/v1/keyword-media-assets/{id}",h.auth.Authenticated(http.HandlerFunc(h.keywordMediaAsset)))
	mux.Handle("DELETE /api/v1/keyword-media-assets/{id}",h.auth.Authenticated(http.HandlerFunc(h.keywordMediaAsset)))
	mux.Handle("POST /api/v1/keyword-actions/{id}/{action}",h.auth.Authenticated(http.HandlerFunc(h.keywordActionControl)))
	mux.Handle("GET /api/v1/join-tasks",h.auth.Authenticated(http.HandlerFunc(h.joinTasks)))
	mux.Handle("POST /api/v1/join-tasks",h.auth.Authenticated(http.HandlerFunc(h.joinTasks)))
	mux.Handle("GET /api/v1/join-tasks/{id}",h.auth.Authenticated(http.HandlerFunc(h.joinTask)))
	mux.Handle("POST /api/v1/join-tasks/{id}/{action}",h.auth.Authenticated(http.HandlerFunc(h.joinTaskAction)))
	mux.Handle("GET /api/v1/messages",h.auth.Authenticated(http.HandlerFunc(h.list)))
	mux.Handle("GET /api/v1/messages/events",h.auth.Authenticated(http.HandlerFunc(h.events)))
	mux.Handle("GET /api/v1/messages/chats",h.auth.Authenticated(http.HandlerFunc(h.listChats)))
	mux.Handle("GET /api/v1/groups",h.auth.Authenticated(http.HandlerFunc(h.listGroupLibrary)))
	mux.Handle("GET /api/v1/groups/{id}",h.auth.Authenticated(http.HandlerFunc(h.getGroupLibraryItem)))
	mux.Handle("GET /api/v1/groups/export.csv",h.auth.Authenticated(http.HandlerFunc(h.exportGroupLibraryCSV)))
	mux.Handle("POST /api/v1/groups/import.csv",h.auth.Authenticated(http.HandlerFunc(h.importGroupCorrectionsCSV)))
	mux.Handle("GET /api/v1/group-links",h.auth.Authenticated(http.HandlerFunc(h.listGroupLinks)))
	mux.Handle("GET /api/v1/group-links/stats",h.auth.Authenticated(http.HandlerFunc(h.groupLinkStats)))
	mux.Handle("GET /api/v1/group-links/settings",h.auth.Authenticated(http.HandlerFunc(h.getLinkSettings)))
	mux.Handle("PUT /api/v1/group-links/settings",h.auth.Authenticated(http.HandlerFunc(h.updateLinkSettings)))
	mux.Handle("GET /api/v1/groups/{id}/sources",h.auth.Authenticated(http.HandlerFunc(h.listGroupSources)))
	mux.Handle("GET /api/v1/messages/settings",h.auth.Authenticated(http.HandlerFunc(h.getSettings)))
	mux.Handle("PUT /api/v1/messages/settings",h.auth.Authenticated(http.HandlerFunc(h.updateSettings)))
	mux.Handle("PATCH /api/v1/messages/{id}/processed",h.auth.Authenticated(http.HandlerFunc(h.markProcessed)))
	mux.Handle("POST /api/v1/messages/{id}/reply",h.auth.Authenticated(http.HandlerFunc(h.createReply)))
	mux.Handle("POST /api/v1/messages/{id}/contact",h.auth.Authenticated(http.HandlerFunc(h.createContactReply)))
	mux.Handle("GET /api/v1/message-replies/{id}",h.auth.Authenticated(http.HandlerFunc(h.getReply)))
	mux.Handle("POST /api/v1/message-replies/{id}/resume",h.auth.Authenticated(http.HandlerFunc(h.resumeReply)))
}

func (h *Handler) unifiedTaskBulkAction(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	taskType, action := r.PathValue("type"), r.PathValue("action")
	var body struct { IDs []string `json:"ids"` }
	if err := decodeMessageJSON(w, r, 1<<20, &body); err != nil {
		messageJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid bulk task body"})
		return
	}
	ids, err := normalizeBulkTaskIDs(taskType, action, body.IDs)
	if err != nil {
		messageJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	results := make([]BulkTaskActionResult, 0, len(ids))
	succeeded := 0
	for _, id := range ids {
		result := BulkTaskActionResult{ID: id}
		if err := h.service.ChangeUnifiedTaskStatus(r.Context(), user.TenantID, taskType, id, action); err != nil {
			result.Error = err.Error()
		} else {
			result.Success = true
			succeeded++
		}
		results = append(results, result)
	}
	status := http.StatusOK
	if succeeded != len(ids) { status = http.StatusMultiStatus }
	messageJSON(w, status, map[string]any{
		"type": taskType, "action": action, "total": len(ids),
		"succeeded": succeeded, "failed": len(ids)-succeeded, "results": results,
	})
}

func (h *Handler) unifiedTaskSummary(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	summary, err := h.service.GetUnifiedTaskSummary(r.Context(), user.TenantID)
	if err != nil {
		messageJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load task summary"})
		return
	}
	messageJSON(w, http.StatusOK, map[string]any{"summary": summary})
}

func (h *Handler) unifiedTasks(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.service.ListUnifiedTasks(r.Context(), user.TenantID, UnifiedTaskFilter{
		Status: r.URL.Query().Get("status"),
		Type: r.URL.Query().Get("type"),
		AccountID: r.URL.Query().Get("account_id"),
		Limit: limit,
		Offset: offset,
	})
	if err != nil {
		messageJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	messageJSON(w, http.StatusOK, map[string]any{"tasks": items, "total": total, "limit": normalizeLimit(limit), "offset": max(0, offset)})
}


func (h *Handler) unifiedTaskAction(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	taskType, id, action := r.PathValue("type"), r.PathValue("id"), r.PathValue("action")
	if err := h.service.ChangeUnifiedTaskStatus(r.Context(), user.TenantID, taskType, id, action); err != nil {
		status := http.StatusConflict
		if err.Error() == "invalid task type" || err.Error() == "unsupported task action" || err.Error() == "invalid task" {
			status = http.StatusBadRequest
		}
		messageJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	messageJSON(w, http.StatusOK, map[string]any{"type": taskType, "id": id, "status": "updated"})
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

func(h *Handler)createContactReply(w http.ResponseWriter,r *http.Request) {
	var body struct {
		Phone string `json:"phone"`
		FirstName string `json:"first_name"`
		LastName string `json:"last_name"`
	}
	if err:=decodeMessageJSON(w,r,8<<10,&body);err!=nil {messageJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid contact body"});return}
	user:=auth.UserFromContext(r.Context())
	job,err:=h.service.CreateContactReply(r.Context(),user.TenantID,r.PathValue("id"),body.Phone,body.FirstName,body.LastName,r.Header.Get("Idempotency-Key"))
	if err!=nil {
		status:=http.StatusBadRequest
		if err.Error()=="message not found" {status=http.StatusNotFound}
		if strings.HasPrefix(err.Error(),"Idempotency-Key was already used") { status=http.StatusConflict }
		messageJSON(w,status,map[string]string{"error":err.Error()});return
	}
	messageJSON(w,http.StatusAccepted,map[string]any{"reply":job})
}

func(h *Handler)createReply(w http.ResponseWriter,r *http.Request){
	var body struct{Text string `json:"text"`}
	if err:=decodeMessageJSON(w,r,16<<10,&body);err!=nil{messageJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid reply body"});return}
	user:=auth.UserFromContext(r.Context())
	job,err:=h.service.CreateReply(r.Context(),user.TenantID,r.PathValue("id"),body.Text,r.Header.Get("Idempotency-Key"))
	if err!=nil{
		status:=http.StatusBadRequest
		if err.Error()=="message not found"{status=http.StatusNotFound}
		if strings.HasPrefix(err.Error(),"Idempotency-Key was already used") { status=http.StatusConflict }
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

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	cursor := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	if cursor == "" {
		cursor = strings.TrimSpace(r.URL.Query().Get("last_event_id"))
	}
	if cursor == "" {
		cursor = "$"
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "retry: 3000\n: connected\n\n")
	flusher.Flush()

	for {
		if err := r.Context().Err(); err != nil {
			return
		}
		events, err := h.service.ReadMessageEvents(r.Context(), user.TenantID, cursor, 15*time.Second, 100)
		if err != nil {
			return
		}
		if len(events) == 0 {
			_, _ = fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
			continue
		}
		for _, event := range events {
			if _, err := fmt.Fprintf(w, "id: %s\nevent: message\ndata: %s\n\n", event.ID, event.Payload); err != nil {
				return
			}
			cursor = event.ID
			flusher.Flush()
		}
	}
}

func (h *Handler) getLinkSettings(w http.ResponseWriter,r *http.Request) {
	user:=auth.UserFromContext(r.Context())
	settings,err:=h.service.GetLinkCollectionSettings(r.Context(),user.TenantID)
	if err!=nil{messageJSON(w,http.StatusInternalServerError,map[string]string{"error":"failed to load link collection settings"});return}
	messageJSON(w,http.StatusOK,map[string]any{"settings":settings})
}

func (h *Handler) updateLinkSettings(w http.ResponseWriter,r *http.Request) {
	var settings LinkCollectionSettings
	if err:=decodeMessageJSON(w,r,16<<10,&settings);err!=nil{messageJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid link collection settings"});return}
	user:=auth.UserFromContext(r.Context())
	updated,err:=h.service.UpdateLinkCollectionSettings(r.Context(),user.TenantID,settings)
	if err!=nil{messageJSON(w,http.StatusBadRequest,map[string]string{"error":"could not update link collection settings"});return}
	messageJSON(w,http.StatusOK,map[string]any{"settings":updated})
}


func (h *Handler) groupLinkStats(w http.ResponseWriter, r *http.Request) {
    user := auth.UserFromContext(r.Context())
    stats, err := h.service.GetGroupLinkStats(r.Context(), user.TenantID)
    if err != nil {
        messageJSON(w, http.StatusInternalServerError, map[string]string{"error":"failed to load group link statistics"})
        return
    }
    messageJSON(w, http.StatusOK, map[string]any{"stats":stats})
}

func (h *Handler) listGroupLinks(w http.ResponseWriter,r *http.Request) {
	user:=auth.UserFromContext(r.Context())
	limit,_:=strconv.Atoi(r.URL.Query().Get("limit"));offset,_:=strconv.Atoi(r.URL.Query().Get("offset"))
	items,total,err:=h.service.ListGroupLinks(r.Context(),user.TenantID,r.URL.Query().Get("type"),r.URL.Query().Get("status"),limit,offset)
	if err!=nil {
		status:=http.StatusInternalServerError
		if errors.Is(err,ErrInvalidGroupLibraryQuery){status=http.StatusBadRequest}
		messageJSON(w,status,map[string]string{"error":"failed to load group links"});return
	}
	messageJSON(w,http.StatusOK,map[string]any{"links":items,"total":total,"limit":normalizeLimit(limit),"offset":max(0,offset)})
}

func (h *Handler) listGroupSources(w http.ResponseWriter,r *http.Request) {
	user:=auth.UserFromContext(r.Context())
	id,err:=strconv.ParseInt(r.PathValue("id"),10,64)
	if err!=nil||id<=0 { messageJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid group id"});return }
	items,err:=h.service.ListGroupSources(r.Context(),user.TenantID,id)
	if err!=nil { messageJSON(w,http.StatusInternalServerError,map[string]string{"error":"failed to load group sources"});return }
	messageJSON(w,http.StatusOK,map[string]any{"sources":items})
}


func (h *Handler) exportGroupLibraryCSV(w http.ResponseWriter,r *http.Request) {
	user:=auth.UserFromContext(r.Context())
	filter:=GroupLibraryFilter{ChatType:r.URL.Query().Get("chat_type"),Query:r.URL.Query().Get("q"),Sort:r.URL.Query().Get("sort")}
	if raw:=strings.TrimSpace(r.URL.Query().Get("verified"));raw!=""{v,err:=strconv.ParseBool(raw);if err!=nil{http.Error(w,"invalid verified filter",http.StatusBadRequest);return};filter.Verified=&v}
	parse:=func(key string)(int,error){raw:=strings.TrimSpace(r.URL.Query().Get(key));if raw==""{return 0,nil};return strconv.Atoi(raw)}
	var err error
	if filter.MinMembers,err=parse("min_members");err!=nil{http.Error(w,"invalid min_members",http.StatusBadRequest);return}
	if filter.MaxMembers,err=parse("max_members");err!=nil{http.Error(w,"invalid max_members",http.StatusBadRequest);return}
	if filter.MinSources,err=parse("min_sources");err!=nil{http.Error(w,"invalid min_sources",http.StatusBadRequest);return}
	if filter.MinContributors,err=parse("min_contributors");err!=nil{http.Error(w,"invalid min_contributors",http.StatusBadRequest);return}
	if _,_,filterErr:=normalizeGroupLibraryFilters(filter.ChatType,filter.Query);filterErr!=nil{http.Error(w,"invalid group filters",http.StatusBadRequest);return}
	if filterErr:=validateGroupLibraryFilter(filter);filterErr!=nil{http.Error(w,"invalid group filters",http.StatusBadRequest);return}
	maxRows:=10000
	if raw:=r.URL.Query().Get("max_rows");raw!=""{v,e:=strconv.Atoi(raw);if e!=nil||v<1||v>10000{http.Error(w,"max_rows must be between 1 and 10000",http.StatusBadRequest);return};maxRows=v}
	var output bytes.Buffer
	writer:=csv.NewWriter(&output);_ = writer.Write([]string{"id","telegram_chat_id","title","chat_type","username","member_count","is_verified","default_permissions","permissions_observed_at","source_count","contributor_count","last_seen_at"})
	for offset:=0;offset<maxRows;offset+=100 {
		limit:=100;if offset+limit>maxRows{limit=maxRows-offset}
		items,_,err:=h.service.ListGroupLibraryFiltered(r.Context(),user.TenantID,filter,limit,offset)
		if err!=nil{messageJSON(w,http.StatusInternalServerError,map[string]string{"error":"failed to export public groups"});return}
		for _,item:=range items{_ = writer.Write([]string{strconv.FormatInt(item.ID,10),strconv.FormatInt(item.ChatID,10),item.Title,item.ChatType,item.Username,strconv.Itoa(item.MemberCount),strconv.FormatBool(item.IsVerified),string(item.DefaultPermissions),func()string{if item.PermissionsObservedAt==nil{return ""};return item.PermissionsObservedAt.UTC().Format(time.RFC3339)}(),strconv.Itoa(item.SourceCount),strconv.Itoa(item.ContributorCount),item.LastSeenAt.UTC().Format(time.RFC3339)})}
		if len(items)<limit{break}
	}
	writer.Flush();if err:=writer.Error();err!=nil{messageJSON(w,http.StatusInternalServerError,map[string]string{"error":"failed to encode public groups"});return}
	w.Header().Set("Content-Type","text/csv; charset=utf-8");w.Header().Set("Content-Disposition",`attachment; filename="public-groups.csv"`);w.WriteHeader(http.StatusOK);_,_=w.Write(output.Bytes())
}

func (h *Handler) importGroupCorrectionsCSV(w http.ResponseWriter,r *http.Request) {
	user:=auth.UserFromContext(r.Context())
	if user.Role!="tenant_admin"{messageJSON(w,http.StatusForbidden,map[string]string{"error":"tenant_admin_required"});return}
	r.Body=http.MaxBytesReader(w,r.Body,2<<20)
	reader:=csv.NewReader(r.Body);reader.FieldsPerRecord=-1;reader.TrimLeadingSpace=true
	header,err:=reader.Read();if err!=nil{messageJSON(w,http.StatusBadRequest,map[string]string{"error":"CSV header is required"});return}
	columns:=map[string]int{};for i,name:=range header{key:=strings.ToLower(strings.TrimSpace(name));if _,exists:=columns[key];exists{messageJSON(w,http.StatusBadRequest,map[string]string{"error":"duplicate CSV column: "+key});return};columns[key]=i}
	if _,ok:=columns["group_id"];!ok{messageJSON(w,http.StatusBadRequest,map[string]string{"error":"CSV requires group_id column"});return}
	results:=[]map[string]any{};created:=0;failed:=0
	for rowNo:=2;rowNo<=1001;rowNo++{
		row,readErr:=reader.Read();if readErr==io.EOF{break};if readErr!=nil{messageJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid CSV data"});return}
		if len(row)!=len(header){results=append(results,map[string]any{"row":rowNo,"success":false,"error":"column count mismatch"});failed++;continue}
		get:=func(name string)string{idx,ok:=columns[name];if !ok{return ""};return strings.TrimSpace(row[idx])}
		groupID,parseErr:=strconv.ParseInt(get("group_id"),10,64);if parseErr!=nil||groupID<=0{results=append(results,map[string]any{"row":rowNo,"success":false,"error":"invalid group_id"});failed++;continue}
		fields:=GroupCorrectionFields{}
		if v:=get("title");v!=""{fields.Title=&v}
		if v:=get("username");v!=""{fields.Username=&v}
		if v:=get("member_count");v!=""{n,e:=strconv.Atoi(v);if e!=nil{results=append(results,map[string]any{"row":rowNo,"success":false,"error":"invalid member_count"});failed++;continue};fields.MemberCount=&n}
		if v:=get("is_verified");v!=""{b,e:=strconv.ParseBool(v);if e!=nil{results=append(results,map[string]any{"row":rowNo,"success":false,"error":"invalid is_verified"});failed++;continue};fields.IsVerified=&b}
		item,submitErr:=h.service.SubmitGroupCorrection(r.Context(),user.TenantID,user.ID,groupID,fields)
		if submitErr!=nil{results=append(results,map[string]any{"row":rowNo,"group_id":groupID,"success":false,"error":submitErr.Error()});failed++;continue}
		created++;results=append(results,map[string]any{"row":rowNo,"group_id":groupID,"success":true,"correction_id":item.ID,"status":item.Status})
	}
	if len(results)==1000{if _,extraErr:=reader.Read();extraErr!=io.EOF{messageJSON(w,http.StatusBadRequest,map[string]string{"error":"CSV exceeds 1000 data rows"});return}}
	messageJSON(w,http.StatusOK,map[string]any{"total":len(results),"submitted":created,"failed":failed,"results":results})
}


func (h *Handler) getGroupLibraryItem(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		messageJSON(w, http.StatusBadRequest, map[string]string{"error":"invalid group id"})
		return
	}
	item, err := h.service.GetGroupLibraryItem(r.Context(), user.TenantID, id)
	if errors.Is(err, ErrGroupLibraryNotFound) {
		messageJSON(w, http.StatusNotFound, map[string]string{"error":"group not found"})
		return
	}
	if err != nil {
		messageJSON(w, http.StatusInternalServerError, map[string]string{"error":"failed to load group"})
		return
	}
	messageJSON(w, http.StatusOK, map[string]any{"group":item})
}

func parseGroupLibraryPagination(rawLimit, rawOffset string) (int, int, error) {
	limit, offset := 50, 0
	if strings.TrimSpace(rawLimit) != "" {
		value, err := strconv.Atoi(strings.TrimSpace(rawLimit))
		if err != nil || value < 1 || value > 100 {
			return 0, 0, ErrInvalidGroupLibraryQuery
		}
		limit = value
	}
	if strings.TrimSpace(rawOffset) != "" {
		value, err := strconv.Atoi(strings.TrimSpace(rawOffset))
		if err != nil || value < 0 || value > 10000000 {
			return 0, 0, ErrInvalidGroupLibraryQuery
		}
		offset = value
	}
	return limit, offset, nil
}

func (h *Handler) listGroupLibrary(w http.ResponseWriter, r *http.Request) {
	user:=auth.UserFromContext(r.Context())
	limit,offset,paginationErr:=parseGroupLibraryPagination(r.URL.Query().Get("limit"),r.URL.Query().Get("offset"))
	if paginationErr!=nil { messageJSON(w,http.StatusBadRequest,map[string]string{"error":paginationErr.Error()});return }
	filter:=GroupLibraryFilter{ChatType:r.URL.Query().Get("chat_type"),Query:r.URL.Query().Get("q"),Sort:r.URL.Query().Get("sort")}
	parseInt:=func(key string)(int,error){raw:=strings.TrimSpace(r.URL.Query().Get(key));if raw==""{return 0,nil};v,err:=strconv.Atoi(raw);if err!=nil{return 0,ErrInvalidGroupLibraryQuery};return v,nil}
	var err error
	if filter.MinMembers,err=parseInt("min_members");err!=nil{messageJSON(w,http.StatusBadRequest,map[string]string{"error":err.Error()});return}
	if filter.MaxMembers,err=parseInt("max_members");err!=nil{messageJSON(w,http.StatusBadRequest,map[string]string{"error":err.Error()});return}
	if filter.MinSources,err=parseInt("min_sources");err!=nil{messageJSON(w,http.StatusBadRequest,map[string]string{"error":err.Error()});return}
	if filter.MinContributors,err=parseInt("min_contributors");err!=nil{messageJSON(w,http.StatusBadRequest,map[string]string{"error":err.Error()});return}
	if raw:=strings.TrimSpace(r.URL.Query().Get("verified"));raw!=""{v,parseErr:=strconv.ParseBool(raw);if parseErr!=nil{messageJSON(w,http.StatusBadRequest,map[string]string{"error":"verified must be true or false"});return};filter.Verified=&v}
	items,total,err:=h.service.ListGroupLibraryFiltered(r.Context(),user.TenantID,filter,limit,offset)
	if err!=nil{status:=http.StatusInternalServerError;if errors.Is(err,ErrInvalidGroupLibraryQuery){status=http.StatusBadRequest};messageJSON(w,status,map[string]string{"error":err.Error()});return}
	messageJSON(w,http.StatusOK,map[string]any{"groups":items,"total":total,"limit":normalizeLimit(limit),"offset":max(0,offset)})
}
