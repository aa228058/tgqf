"use client";

import { apiErrorMessage, readAPIJSON, translateAPIError } from "./api-response";

import { FormEvent, useCallback, useEffect, useRef, useState } from "react";

const API_BASE=(process.env.NEXT_PUBLIC_API_BASE_URL||"").replace(/\/$/,"");
type AlbumItem={asset_id:string;media_type:string;caption:string};
type ActionStep={action_type:string;text?:string;reply_text_by_chat_type?:Record<string,string>;contact_phone?:string;contact_first_name?:string;contact_last_name?:string;media_asset_id?:string;media_type?:string;media_caption?:string;media_album_items?:AlbumItem[]};
type Rule={
 id:string;name:string;match_mode:string;keywords:string[];exclude_terms:string[];
 keyword_groups:string[][];group_operator:string;chat_types:string[];account_ids:string[];
 chat_ids:number[];media_types:string[];chat_titles:string[];chat_usernames:string[];
 sender_ids:number[];sender_usernames:string[];sender_names:string[];
 reply_text:string;reply_text_by_chat_type:Record<string,string>;action_type:string;
 enabled:boolean;case_sensitive:boolean;contact_phone?:string;contact_first_name?:string;
 contact_last_name?:string;media_asset_id?:string;media_type?:string;media_caption?:string;
 media_album_items?:AlbumItem[];action_steps?:ActionStep[];
};
type Match={id:number;rule_id:string;rule_name:string;account_id:string;chat_id:number;message_id:number;matched_terms:string[];matched_at:string};
type StepResult={step_index:number;action_type:string;status:string;telegram_message_ids?:number[];telegram_call_id?:number;error?:string;completed_at:string};
type Action={id:string;keyword_match_id:number;rule_name:string;account_id:string;chat_id:number;message_text:string;action_type:string;status:string;attempts:number;last_error:string;available_at:string;created_at:string;telegram_message_id?:number;telegram_message_ids:number[];voice_call_state?:string;telegram_call_id?:number;action_steps?:ActionStep[];current_step_index:number;step_results?:StepResult[]};
type Asset={id:string;file_name:string;mime_type:string;file_size:number};
type Account={id:string;phone:string;display_name:string;status:string};
async function api<T>(path:string,token:string,init?:RequestInit):Promise<T>{
 const headers:Record<string,string>={Authorization:`Bearer ${token}`};
 if(!(init?.body instanceof FormData))headers["Content-Type"]="application/json";
 Object.assign(headers,init?.headers||{});
 const response=await fetch(`${API_BASE}${path}`,{...init,headers,cache:"no-store"});
 if(!response.ok){let message=`请求失败（${response.status}）`;message=await apiErrorMessage(response,message);throw new Error(message)}
 if(response.status===204)return undefined as T;
 return readAPIJSON<T>(response);
}
const date=(v:string)=>new Date(v).toLocaleString();
const splitList=(v:string)=>v.split(/[,，\n]/).map(x=>x.trim()).filter(Boolean);
const splitIDs=(v:string)=>splitList(v).map(Number).filter(x=>Number.isSafeInteger(x)&&x!==0);
const emptyRule=():Rule=>({
 id:"",name:"",match_mode:"any",keywords:[],exclude_terms:[],keyword_groups:[],group_operator:"any",
 chat_types:["private","group"],account_ids:[],chat_ids:[],media_types:[],chat_titles:[],chat_usernames:[],
 sender_ids:[],sender_usernames:[],sender_names:[],reply_text:"",reply_text_by_chat_type:{},
 action_type:"text",enabled:true,case_sensitive:false,contact_phone:"",contact_first_name:"",
 contact_last_name:"",media_asset_id:"",media_type:"photo",media_caption:"",media_album_items:[],
 action_steps:[{action_type:"text",text:"",reply_text_by_chat_type:{}}]
});

export function KeywordMonitor({token}:{token:string}){
 const [rules,setRules]=useState<Rule[]>([]),[matches,setMatches]=useState<Match[]>([]),[actions,setActions]=useState<Action[]>([]);
 const [assets,setAssets]=useState<Asset[]>([]),[accounts,setAccounts]=useState<Account[]>([]);
 const [tab,setTab]=useState<"rules"|"matches"|"actions">("rules");
 const [draft,setDraft]=useState<Rule|null>(null),[busy,setBusy]=useState(false),[error,setError]=useState(""),[notice,setNotice]=useState("");
 const [keywordText,setKeywordText]=useState(""),[excludeText,setExcludeText]=useState(""),[groupText,setGroupText]=useState("");
 const [chatIDsText,setChatIDsText]=useState(""),[mediaTypesText,setMediaTypesText]=useState("");
 const [chatTitlesText,setChatTitlesText]=useState(""),[chatUsernamesText,setChatUsernamesText]=useState("");
 const [senderIDsText,setSenderIDsText]=useState(""),[senderUsernamesText,setSenderUsernamesText]=useState(""),[senderNamesText,setSenderNamesText]=useState("");
 const [privateReplyText,setPrivateReplyText]=useState(""),[groupReplyText,setGroupReplyText]=useState("");
 const [status,setStatus]=useState("");
 const [matchOffset,setMatchOffset]=useState(0),[actionOffset,setActionOffset]=useState(0);
 const [matchTotal,setMatchTotal]=useState(0),[actionTotal,setActionTotal]=useState(0);
 const loadSequence=useRef(0),previewSequence=useRef(0),operationLock=useRef(false);
 const [loading,setLoading]=useState(false),[previewLoading,setPreviewLoading]=useState(false);
 const [preview,setPreview]=useState<{asset:Asset;url:string}|null>(null);
 const load=useCallback(async()=>{
  const sequence=++loadSequence.current;
  setLoading(true);
  setError("");
  try{
   const [r,m,a,media,accountResult]=await Promise.all([
    api<{rules:Rule[]}>("/api/v1/keyword-rules",token),
    api<{matches:Match[];total:number}>(`/api/v1/keyword-rules/matches?limit=50&offset=${matchOffset}`,token),
    api<{actions:Action[];total:number}>(`/api/v1/keyword-actions?limit=50&offset=${actionOffset}${status?`&status=${encodeURIComponent(status)}`:""}`,token),
    api<{assets:Asset[]}>("/api/v1/keyword-media-assets",token),
    api<{accounts:Account[]}>("/api/v1/accounts?limit=100&offset=0&status=authorized",token)
   ]);
   if(sequence===loadSequence.current){setRules(r.rules||[]);setMatches(m.matches||[]);setMatchTotal(m.total||0);setActions(a.actions||[]);setActionTotal(a.total||0);setAssets(media.assets||[]);setAccounts(accountResult.accounts||[])}
  }catch(e){if(sequence===loadSequence.current)setError(e instanceof Error ? translateAPIError(e.message) :"关键词监控数据加载失败")}
  finally{if(sequence===loadSequence.current)setLoading(false)}
 },[token,matchOffset,actionOffset,status]);
 useEffect(()=>{void load();const timer=window.setInterval(()=>{if(document.visibilityState==="visible")void load()},15000);return()=>{loadSequence.current++;window.clearInterval(timer)}},[load]);
 useEffect(()=>()=>{if(preview?.url)URL.revokeObjectURL(preview.url)},[preview?.url]);
 function edit(rule?:Rule){
  const base=rule?{...emptyRule(),...rule,action_steps:rule.action_steps?.length?rule.action_steps:[{action_type:rule.action_type,text:rule.reply_text,reply_text_by_chat_type:rule.reply_text_by_chat_type,contact_phone:rule.contact_phone,contact_first_name:rule.contact_first_name,contact_last_name:rule.contact_last_name,media_asset_id:rule.media_asset_id,media_type:rule.media_type,media_caption:rule.media_caption,media_album_items:rule.media_album_items}]}:emptyRule();
  setDraft(base);
  setKeywordText((base.keywords||[]).join(", "));
  setExcludeText((base.exclude_terms||[]).join(", "));
  setGroupText((base.keyword_groups||[]).map(group=>group.join(", ")).join("\n"));
  setChatIDsText((base.chat_ids||[]).join(", "));
  setMediaTypesText((base.media_types||[]).join(", "));
  setChatTitlesText((base.chat_titles||[]).join(", "));
  setChatUsernamesText((base.chat_usernames||[]).join(", "));
  setSenderIDsText((base.sender_ids||[]).join(", "));
  setSenderUsernamesText((base.sender_usernames||[]).join(", "));
  setSenderNamesText((base.sender_names||[]).join(", "));
  setPrivateReplyText(base.reply_text_by_chat_type?.private||"");
  setGroupReplyText(base.reply_text_by_chat_type?.group||"");
 }
 async function save(e:FormEvent){
  e.preventDefault();if(!draft)return;
  if(operationLock.current)return;operationLock.current=true;setBusy(true);setError("");setNotice("");
  const groups=groupText.split(/\n/).map(line=>splitList(line)).filter(group=>group.length>0);
  const payload:Rule={
   ...draft,
   keywords:splitList(keywordText),exclude_terms:splitList(excludeText),
   keyword_groups:groups,chat_ids:splitIDs(chatIDsText),media_types:splitList(mediaTypesText),
   chat_titles:splitList(chatTitlesText),chat_usernames:splitList(chatUsernamesText),
   sender_ids:splitIDs(senderIDsText),sender_usernames:splitList(senderUsernamesText),sender_names:splitList(senderNamesText),
   reply_text_by_chat_type:{private:privateReplyText,group:groupReplyText},
   action_steps:(draft.action_steps||[]).map(step=>step.action_type==="text"?{...step,reply_text_by_chat_type:step.reply_text_by_chat_type||{}}:step)
  };
  try{
   const result=await api<{rule:Rule}>(draft.id?`/api/v1/keyword-rules/${draft.id}`:"/api/v1/keyword-rules",token,{method:draft.id?"PUT":"POST",body:JSON.stringify(payload)});
   setRules(old=>draft.id?old.map(x=>x.id===draft.id?result.rule:x):[result.rule,...old]);setDraft(null);setNotice("关键词规则已保存");await load();
  }catch(e){setError(e instanceof Error ? translateAPIError(e.message) :"规则保存失败")}
  finally{operationLock.current=false;setBusy(false)}
 }
 async function remove(rule:Rule){
  if(!window.confirm(`确定删除规则「${rule.name}」？`))return;
  if(operationLock.current)return;operationLock.current=true;setBusy(true);setError("");setNotice("");try{await api(`/api/v1/keyword-rules/${rule.id}`,token,{method:"DELETE"});setRules(old=>old.filter(x=>x.id!==rule.id));setNotice("规则已删除")}
  catch(e){setError(e instanceof Error ? translateAPIError(e.message) :"删除规则失败")}finally{operationLock.current=false;setBusy(false)}
 }
 async function control(action:Action,op:"pause"|"resume"|"retry"){
  if(operationLock.current)return;operationLock.current=true;setBusy(true);setError("");setNotice("");
  try{await api(`/api/v1/keyword-actions/${action.id}/${op}`,token,{method:"POST"});setNotice("动作任务状态已更新");await load()}
  catch(e){setError(e instanceof Error ? translateAPIError(e.message) :"动作任务操作失败")}finally{operationLock.current=false;setBusy(false)}
 }
 async function openAssetPreview(asset:Asset){
  const sequence=++previewSequence.current;
  setPreviewLoading(true);setError("");
  try{
   const response=await fetch(`${API_BASE}/api/v1/keyword-media-assets/${asset.id}`,{headers:{Authorization:`Bearer ${token}`},cache:"no-store"});
   if(!response.ok)throw new Error("素材预览加载失败");
   const blob=await response.blob();
   if(sequence!==previewSequence.current)return;
   setPreview(old=>{if(old?.url)URL.revokeObjectURL(old.url);return {asset,url:URL.createObjectURL(blob)}});
  }catch(e){if(sequence===previewSequence.current)setError(e instanceof Error ? translateAPIError(e.message) :"素材预览加载失败")}
  finally{if(sequence===previewSequence.current)setPreviewLoading(false)}
 }
 function closeAssetPreview(){previewSequence.current++;if(preview)URL.revokeObjectURL(preview.url);setPreview(null)}
 async function deleteAsset(asset:Asset){
  if(!window.confirm(`确定删除素材「${asset.file_name}」？若规则正在引用，后端会拒绝删除。`))return;
  if(operationLock.current)return;operationLock.current=true;setBusy(true);setError("");setNotice("");
  try{await api(`/api/v1/keyword-media-assets/${asset.id}`,token,{method:"DELETE"});setNotice("媒体素材已删除");await load()}
  catch(e){setError(e instanceof Error ? translateAPIError(e.message) :"素材删除失败")}
  finally{operationLock.current=false;setBusy(false)}
 }
 async function uploadAsset(file:File){
  if(operationLock.current)return;
  if(file.size===0){setError("不能上传空文件");return}
  const form=new FormData();form.append("file",file);operationLock.current=true;setBusy(true);setError("");setNotice("");
  try{await api("/api/v1/keyword-media-assets",token,{method:"POST",body:form});setNotice("媒体素材已上传");await load()}
  catch(e){setError(e instanceof Error ? translateAPIError(e.message) :"媒体素材上传失败")}finally{operationLock.current=false;setBusy(false)}
 }
 function updateStep(index:number,patch:Partial<ActionStep>){
  setDraft(current=>current?{...current,action_steps:(current.action_steps||[]).map((step,i)=>i===index?{...step,...patch}:step)}:current);
 }
 function addStep(){
  setDraft(current=>current?{...current,action_steps:[...(current.action_steps||[]),{action_type:"text",text:"",reply_text_by_chat_type:{}}]}:current);
 }
 function moveStep(index:number,direction:-1|1){
  setDraft(current=>{if(!current)return current;const steps=[...(current.action_steps||[])];const target=index+direction;if(target<0||target>=steps.length)return current;[steps[index],steps[target]]=[steps[target],steps[index]];return {...current,action_steps:steps}});
 }
 function removeStep(index:number){
  setDraft(current=>current?{...current,action_steps:(current.action_steps||[]).filter((_,i)=>i!==index)}:current);
 }
 const parsedGroups=groupText.split(/\n/).map(line=>splitList(line)).filter(group=>group.length>0);
 const invalidGroups=parsedGroups.length>10||parsedGroups.some(group=>group.length>20)||splitList(keywordText).length>20;

 return <section className="keyword-monitor">
  <div className="keyword-toolbar"><div><div className="eyebrow">LIVE MESSAGE AUTOMATION</div><h1>监控回复</h1><p>实时新消息关键词匹配、规则组合、命中追踪及自动动作执行。</p></div><button className="secondary" onClick={()=>void load()} disabled={busy||loading}>{loading?"同步中…":"↻ 刷新数据"}</button></div>
  {error&&<div className="alert error">{error}</div>}{notice&&<div className="alert success">{notice}</div>}
  <div className="keyword-tabs" role="tablist" aria-label="监控回复模块"><button className={tab==="rules"?"active":""} role="tab" aria-selected={tab==="rules"} onClick={()=>setTab("rules")}>规则配置（{rules.length}）</button><button className={tab==="matches"?"active":""} role="tab" aria-selected={tab==="matches"} onClick={()=>setTab("matches")}>命中记录（{matchTotal}）</button><button className={tab==="actions"?"active":""} role="tab" aria-selected={tab==="actions"} onClick={()=>setTab("actions")}>动作任务（{actionTotal}）</button></div>
  <div className="sr-only" role="status" aria-live="polite">{loading?"正在同步监控数据":busy?"正在处理操作":""}</div>
  {tab==="rules"&&<>
   <div className="keyword-toolbar"><strong>监控规则</strong><button className="primary" onClick={()=>edit()}>新建规则</button></div>
   <div className="keyword-rule-list">{rules.map(rule=><article className="keyword-card" key={rule.id}>
    <div><strong>{rule.name}</strong><span className={rule.enabled?"keyword-state enabled":"keyword-state"}>{rule.enabled?"已启用":"已停用"}</span>
     <p>关键词：{(rule.keywords||[]).join("、")||"组合/媒体规则"} · 模式：{rule.match_mode} · 动作：{rule.action_type}</p>
     <small>分组：{rule.keyword_groups?.length||0} 组（{rule.group_operator==="all"?"组间 AND":"组间 OR"}） · 会话：{(rule.chat_types||[]).join("、")} · 账号过滤：{rule.account_ids?.length||0}</small>
    </div>
    <div className="keyword-actions"><button className="secondary" onClick={()=>edit(rule)}>编辑</button><button className="secondary danger" onClick={()=>void remove(rule)} disabled={busy}>删除</button></div>
   </article>)}{!rules.length&&<div className="empty">{loading?"正在加载规则…":error?"加载失败，请检查提示后重试":"尚未配置关键词规则"}</div>}</div>
   <div className="keyword-assets"><strong>媒体素材库</strong><label className="secondary">上传素材<input type="file" hidden accept="image/*,video/*,audio/*,.pdf,.doc,.docx" onChange={e=>{const file=e.target.files?.[0];if(file)void uploadAsset(file);e.currentTarget.value=""}}/></label><div className="keyword-asset-list">{assets.map(a=><span key={a.id}>{a.file_name} · {(a.file_size/1024).toFixed(0)} KB <button className="secondary" disabled={busy||previewLoading} onClick={()=>void openAssetPreview(a)}>{previewLoading?"加载中…":"预览"}</button><button className="danger-text" disabled={busy} onClick={()=>void deleteAsset(a)}>删除</button></span>)}</div></div>
  </>}
  {tab==="matches"&&<div className="keyword-table-wrap"><table className="keyword-table"><thead><tr><th>规则</th><th>账号</th><th>Chat ID</th><th>消息 ID</th><th>命中词</th><th>时间</th></tr></thead><tbody>{matches.map(m=><tr key={m.id}><td data-label="规则">{m.rule_name}</td><td data-label="账号">{m.account_id.slice(0,8)}…</td><td data-label="Chat ID">{m.chat_id}</td><td data-label="消息 ID">{m.message_id}</td><td data-label="命中词">{m.matched_terms.join("、")}</td><td data-label="命中时间">{date(m.matched_at)}</td></tr>)}</tbody></table>{!matches.length&&<div className="empty">{loading?"正在同步命中记录…":error?"加载失败，请检查提示后重试":"暂无命中记录"}</div>}<div className="groups-pagination"><span>第 {Math.floor(matchOffset/50)+1} 页 · 共 {matchTotal} 条</span><div><button type="button" className="secondary" disabled={!matchOffset||loading} onClick={()=>setMatchOffset(v=>Math.max(0,v-50))}>上一页</button><button type="button" className="secondary" disabled={matchOffset+50>=matchTotal||loading} onClick={()=>setMatchOffset(v=>v+50)}>下一页</button></div></div></div>}
  {tab==="actions"&&<><div className="keyword-filter"><label>任务状态<select value={status} onChange={e=>{setStatus(e.target.value);setActionOffset(0)}}><option value="">全部状态</option>{["pending","processing","sent","failed","paused"].map(x=><option key={x}>{x}</option>)}</select></label></div><div className="keyword-rule-list">{actions.map(a=><article className="keyword-card" key={a.id}><div><strong>{a.rule_name} · {a.action_type}</strong><span className="keyword-state">{a.status}</span><p>Chat {a.chat_id} · 尝试 {a.attempts} 次 · {date(a.created_at)}</p>{a.last_error&&<small className={a.last_error.includes("uncertain")?"keyword-error":"muted"}>{translateAPIError(a.last_error)}</small>}{(a.action_steps?.length||0)>0&&<div className="keyword-step-progress"><strong>动作进度：{a.status==="sent"?a.action_steps!.length:Math.min(a.current_step_index+1,a.action_steps!.length)}/{a.action_steps!.length}</strong>{a.action_steps!.map((step,i)=>{const result=a.step_results?.find(x=>x.step_index===i);return <small key={i}>第 {i+1} 步 · {step.action_type} · {result?.status==="sent"?"已完成":i===a.current_step_index?(a.status==="paused"?"已暂停":"执行中/待执行"):"等待中"}{result?.telegram_message_ids?.length?" · 消息 "+result.telegram_message_ids.join(","):""}{result?.telegram_call_id?" · Call "+result.telegram_call_id:""}</small>})}</div>}{a.telegram_message_ids?.length>0&&<small>Telegram 消息 ID：{a.telegram_message_ids.join(", ")}</small>}{a.voice_call_state&&<small>语音呼叫状态：{a.voice_call_state} · Call ID：{a.telegram_call_id||"—"}</small>}</div><div className="keyword-actions">{(a.status==="pending"||a.status==="failed")&&<button className="secondary" disabled={busy} onClick={()=>void control(a,"pause")}>暂停</button>}{a.status==="paused"&&<button className="secondary" disabled={busy} onClick={()=>void control(a,"resume")}>恢复（核验后）</button>}{a.status==="failed"&&<button className="primary" disabled={busy} onClick={()=>void control(a,"retry")}>重试</button>}</div></article>)}{!actions.length&&<div className="empty">{loading?"正在同步动作任务…":error?"加载失败，请检查提示后重试":"暂无符合条件的动作任务"}</div>}<div className="groups-pagination"><span>第 {Math.floor(actionOffset/50)+1} 页 · 共 {actionTotal} 条</span><div><button type="button" className="secondary" disabled={!actionOffset||loading} onClick={()=>setActionOffset(v=>Math.max(0,v-50))}>上一页</button><button type="button" className="secondary" disabled={actionOffset+50>=actionTotal||loading} onClick={()=>setActionOffset(v=>v+50)}>下一页</button></div></div></div></>}
  {preview&&<div className="modal-backdrop" onClick={closeAssetPreview}><section className="account-modal keyword-preview" onClick={e=>e.stopPropagation()}><div className="modal-heading"><div><h2>{preview.asset.file_name}</h2><p>{preview.asset.mime_type} · {(preview.asset.file_size/1024).toFixed(0)} KB</p></div><button type="button" className="ghost" onClick={closeAssetPreview}>关闭</button></div>{preview.asset.mime_type.startsWith("image/")?<img src={preview.url} alt={preview.asset.file_name}/>:preview.asset.mime_type.startsWith("video/")?<video src={preview.url} controls/>:preview.asset.mime_type.startsWith("audio/")?<audio src={preview.url} controls/>:preview.asset.mime_type==="application/pdf"?<iframe src={preview.url} title={preview.asset.file_name}/>:<a href={preview.url} download={preview.asset.file_name}>下载素材</a>}</section></div>}
  {draft&&<div className="modal-backdrop"><form className="account-modal keyword-editor" onSubmit={save}>
   <div className="modal-heading"><div><h2>{draft.id?"编辑关键词规则":"新建关键词规则"}</h2><p>规则只匹配启用后的实时新消息，不回溯历史消息。</p></div><button type="button" className="ghost" onClick={()=>setDraft(null)}>关闭</button></div>
   <label>规则名称<input required maxLength={80} value={draft.name} onChange={e=>setDraft({...draft,name:e.target.value})}/></label>
   <label>基础关键词（逗号分隔）<textarea rows={2} value={keywordText} onChange={e=>setKeywordText(e.target.value)} placeholder="例如：招聘, 合作, 优惠；使用分组时可留空"/></label>
   <label>排除词（逗号分隔）<input value={excludeText} onChange={e=>setExcludeText(e.target.value)} placeholder="例如：广告, 违规"/></label>
   <div className="keyword-editor-grid"><label>匹配模式<select value={draft.match_mode} onChange={e=>setDraft({...draft,match_mode:e.target.value})}><option value="any">任意关键词</option><option value="all">全部关键词</option><option value="exact">完整文本</option><option value="regex">正则表达式</option></select></label><label>分组间关系<select value={draft.group_operator} onChange={e=>setDraft({...draft,group_operator:e.target.value})}><option value="any">任意分组命中（OR）</option><option value="all">所有分组都命中（AND）</option></select></label></div>
   <label>关键词分组（每行一组，组内逗号分隔）<textarea rows={3} value={groupText} onChange={e=>setGroupText(e.target.value)} placeholder={"例如：\n投资, 融资\n招聘, 兼职"} /><small>组内按上方匹配模式判断；组与组之间按 OR/AND 判断。最多 10 组，每组最多 20 项。{invalidGroups&&<strong className="keyword-error">关键词或分组数量超出限制。</strong>}</small></label>
   <div className="keyword-sequence-heading"><div><strong>自动动作链</strong><small>按顺序逐步执行；每步结果持久化后才会进入下一步。失败或结果不确定时会暂停后续步骤。</small></div><button type="button" className="secondary" onClick={addStep} disabled={(draft.action_steps||[]).length>=10}>＋ 添加步骤</button></div>
   {(draft.action_steps||[]).map((step,index)=><section className="keyword-card keyword-step-editor" key={index}>
    <div className="keyword-step-title"><strong>第 {index+1} 步</strong><div className="keyword-actions"><button type="button" className="secondary" disabled={index===0} onClick={()=>moveStep(index,-1)}>上移</button><button type="button" className="secondary" disabled={index===(draft.action_steps||[]).length-1} onClick={()=>moveStep(index,1)}>下移</button><button type="button" className="secondary danger" disabled={(draft.action_steps||[]).length<=1} onClick={()=>removeStep(index)}>删除</button></div></div>
    <label>动作类型<select value={step.action_type} onChange={e=>updateStep(index,{action_type:e.target.value,text:"",reply_text_by_chat_type:{},contact_phone:"",contact_first_name:"",contact_last_name:"",media_asset_id:"",media_type:"photo",media_caption:"",media_album_items:[]})}><option value="text">发送文本</option><option value="contact">发送联系人卡片</option><option value="media">发送媒体素材</option><option value="media_album">发送媒体相册</option><option value="voice_call">发起 Telegram 原生语音呼叫（仅私聊）</option></select></label>
    {step.action_type==="text"&&<><label>默认文本<textarea rows={3} maxLength={4096} value={step.text||""} onChange={e=>updateStep(index,{text:e.target.value})}/></label><div className="keyword-editor-grid"><label>私聊文案<textarea rows={2} maxLength={4096} value={step.reply_text_by_chat_type?.private||""} onChange={e=>updateStep(index,{reply_text_by_chat_type:{...(step.reply_text_by_chat_type||{}),private:e.target.value}})}/></label><label>群组文案<textarea rows={2} maxLength={4096} value={step.reply_text_by_chat_type?.group||""} onChange={e=>updateStep(index,{reply_text_by_chat_type:{...(step.reply_text_by_chat_type||{}),group:e.target.value}})}/></label></div></>}
    {step.action_type==="contact"&&<div className="keyword-editor-grid"><label>手机号<input required value={step.contact_phone||""} onChange={e=>updateStep(index,{contact_phone:e.target.value})}/></label><label>名字<input required value={step.contact_first_name||""} onChange={e=>updateStep(index,{contact_first_name:e.target.value})}/></label><label>姓氏<input value={step.contact_last_name||""} onChange={e=>updateStep(index,{contact_last_name:e.target.value})}/></label></div>}
    {step.action_type==="media"&&<><label>媒体素材<select required value={step.media_asset_id||""} onChange={e=>updateStep(index,{media_asset_id:e.target.value})}><option value="">选择已上传素材</option>{assets.map(asset=><option key={asset.id} value={asset.id}>{asset.file_name} · {asset.mime_type}</option>)}</select></label><div className="keyword-editor-grid"><label>Telegram 媒体类型<select value={step.media_type||"photo"} onChange={e=>updateStep(index,{media_type:e.target.value})}>{["photo","video","document","audio","voice","video_note"].map(x=><option key={x}>{x}</option>)}</select></label><label>媒体说明<input maxLength={1024} value={step.media_caption||""} onChange={e=>updateStep(index,{media_caption:e.target.value})}/></label></div></>}
    {step.action_type==="media_album"&&<label>相册素材（2-10 项）<select multiple size={Math.min(8,Math.max(3,assets.length))} value={(step.media_album_items||[]).map(x=>x.asset_id)} onChange={e=>{const ids=Array.from(e.target.selectedOptions).map(x=>x.value);updateStep(index,{media_album_items:ids.map(id=>{const asset=assets.find(x=>x.id===id);return {asset_id:id,media_type:asset?.mime_type.startsWith("image/")?"photo":asset?.mime_type.startsWith("video/")?"video":"document",caption:""}})})}}>{assets.map(asset=><option key={asset.id} value={asset.id}>{asset.file_name} · {asset.mime_type}</option>)}</select><small>已选 {(step.media_album_items||[]).length} 项；照片/视频可混合，文档需全部为文档。</small></label>}
    {step.action_type==="voice_call"&&<div className="join-notice"><strong>原生呼叫</strong><span>仅私聊中向消息发送者发起呼叫，不自动接听或绕过隐私设置。</span></div>}
   </section>)}
   {(draft.action_steps||[]).length>=10&&<small>动作链最多 10 步。</small>}
   <fieldset className="keyword-filter-set"><legend>匹配范围</legend>
    <div className="keyword-checks"><label><input type="checkbox" checked={draft.chat_types.includes("private")} onChange={e=>setDraft({...draft,chat_types:e.target.checked?[...draft.chat_types,"private"]:draft.chat_types.filter(x=>x!=="private")})}/>私聊</label><label><input type="checkbox" checked={draft.chat_types.includes("group")} onChange={e=>setDraft({...draft,chat_types:e.target.checked?[...draft.chat_types,"group"]:draft.chat_types.filter(x=>x!=="group")})}/>群组</label><label><input type="checkbox" checked={draft.chat_types.includes("channel")} disabled={(draft.action_steps||[]).some(step=>step.action_type!=="text")} onChange={e=>setDraft({...draft,chat_types:e.target.checked?[...draft.chat_types,"channel"]:draft.chat_types.filter(x=>x!=="channel")})}/>频道（仅文本）</label><label><input type="checkbox" checked={draft.case_sensitive} onChange={e=>setDraft({...draft,case_sensitive:e.target.checked})}/>区分大小写</label></div>
    {draft.action_type==="voice_call"&&<small>语音呼叫规则只能选择私聊。</small>}
    <label>限定账号（不选表示所有已接入账号）<div className="keyword-account-picker">{accounts.map(account=><label key={account.id}><input type="checkbox" checked={draft.account_ids.includes(account.id)} onChange={e=>setDraft({...draft,account_ids:e.target.checked?[...draft.account_ids,account.id]:draft.account_ids.filter(x=>x!==account.id)})}/><span>{account.display_name||account.phone||account.id}</span></label>)}</div></label>
    <div className="keyword-editor-grid"><label>Chat ID（逗号分隔）<input value={chatIDsText} onChange={e=>setChatIDsText(e.target.value)} placeholder="-100123…, 12345"/></label><label>媒体类型（逗号分隔）<input value={mediaTypesText} onChange={e=>setMediaTypesText(e.target.value)} placeholder="photo, video, document"/></label></div>
    <div className="keyword-editor-grid"><label>聊天名称包含（逗号分隔）<input value={chatTitlesText} onChange={e=>setChatTitlesText(e.target.value)}/></label><label>聊天 username（逗号分隔）<input value={chatUsernamesText} onChange={e=>setChatUsernamesText(e.target.value)}/></label></div>
    <div className="keyword-editor-grid"><label>发送者 ID（逗号分隔）<input value={senderIDsText} onChange={e=>setSenderIDsText(e.target.value)}/></label><label>发送者 username（逗号分隔）<input value={senderUsernamesText} onChange={e=>setSenderUsernamesText(e.target.value)}/></label></div>
    <label>发送者名称（逗号分隔）<input value={senderNamesText} onChange={e=>setSenderNamesText(e.target.value)}/></label>
   </fieldset>
   <button className="primary" disabled={busy||invalidGroups||!draft.chat_types.length||!(draft.action_steps||[]).length||(draft.action_steps||[]).length>10||((draft.chat_types.includes("channel"))&&(draft.action_steps||[]).some(step=>step.action_type!=="text"))||(draft.action_steps||[]).some(step=>step.action_type==="text"&&!step.text?.trim()&&!Object.values(step.reply_text_by_chat_type||{}).some(v=>v.trim()))||(draft.action_steps||[]).some(step=>step.action_type==="contact"&&(!step.contact_phone?.trim()||!step.contact_first_name?.trim()))||(draft.action_steps||[]).some(step=>step.action_type==="media"&&!step.media_asset_id)||(draft.action_steps||[]).some(step=>step.action_type==="media_album"&&((step.media_album_items||[]).length<2||(step.media_album_items||[]).length>10))||(draft.action_steps||[]).some(step=>step.action_type==="voice_call"&&(draft.chat_types.length!==1||!draft.chat_types.includes("private")) )||(draft.action_type==="voice_call"&&(draft.chat_types.length!==1||!draft.chat_types.includes("private")))||(draft.action_type==="media"&&!draft.media_asset_id)||(draft.action_type==="media_album"&&((draft.media_album_items||[]).length<2||(draft.media_album_items||[]).length>10))}>保存规则</button>
  </form></div>}
 </section>;
}
