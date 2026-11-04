"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";

const API_BASE=(process.env.NEXT_PUBLIC_API_BASE_URL||"").replace(/\/$/,"");
type Rule={
 id:string;name:string;match_mode:string;keywords:string[];exclude_terms:string[];
 chat_types:string[];account_ids:string[];chat_ids:number[];media_types:string[];
 reply_text:string;action_type:string;enabled:boolean;case_sensitive:boolean;contact_phone?:string;contact_first_name?:string;contact_last_name?:string;media_asset_id?:string;media_type?:string;media_caption?:string;
 keyword_groups:string[][];group_operator:string;reply_text_by_chat_type:Record<string,string>;
};
type Match={id:number;rule_id:string;rule_name:string;account_id:string;chat_id:number;message_id:number;matched_terms:string[];matched_at:string};
type Action={id:string;keyword_match_id:number;rule_name:string;account_id:string;chat_id:number;message_text:string;action_type:string;status:string;attempts:number;last_error:string;available_at:string;created_at:string;telegram_message_id?:number;telegram_message_ids:number[]};
type Asset={id:string;file_name:string;mime_type:string;file_size:number};
async function api<T>(path:string,token:string,init?:RequestInit):Promise<T>{
 const headers:Record<string,string>={Authorization:`Bearer ${token}`};
 if(!(init?.body instanceof FormData))headers["Content-Type"]="application/json";
 Object.assign(headers,init?.headers||{});
 const response=await fetch(`${API_BASE}${path}`,{...init,headers,cache:"no-store"});
 if(!response.ok){let message=`请求失败（${response.status}）`;try{message=(await response.json()).error||message}catch{}throw new Error(message)}
 if(response.status===204)return undefined as T;
 return response.json() as Promise<T>;
}
const date=(v:string)=>new Date(v).toLocaleString();
const emptyRule=():Rule=>({id:"",name:"",match_mode:"any",keywords:[],exclude_terms:[],chat_types:["private","group"],account_ids:[],chat_ids:[],media_types:[],reply_text:"",action_type:"text",contact_phone:"",contact_first_name:"",contact_last_name:"",media_asset_id:"",media_type:"photo",media_caption:"",enabled:true,case_sensitive:false,keyword_groups:[],group_operator:"any",reply_text_by_chat_type:{}});

export function KeywordMonitor({token}:{token:string}){
 const [rules,setRules]=useState<Rule[]>([]),[matches,setMatches]=useState<Match[]>([]),[actions,setActions]=useState<Action[]>([]);
 const [assets,setAssets]=useState<Asset[]>([]),[tab,setTab]=useState<"rules"|"matches"|"actions">("rules");
 const [draft,setDraft]=useState<Rule|null>(null),[busy,setBusy]=useState(false),[error,setError]=useState(""),[notice,setNotice]=useState("");
 const [keywordText,setKeywordText]=useState(""),[excludeText,setExcludeText]=useState("");
 const [status,setStatus]=useState("");
 const load=useCallback(async()=>{
  setError("");
  try{
   const [r,m,a,media]=await Promise.all([
    api<{rules:Rule[]}>("/api/v1/keyword-rules",token),
    api<{matches:Match[]}>("/api/v1/keyword-rules/matches?limit=50&offset=0",token),
    api<{actions:Action[]}>("/api/v1/keyword-actions?limit=50&offset=0",token),
    api<{assets:Asset[]}>("/api/v1/keyword-media-assets",token)
   ]);
   setRules(r.rules||[]);setMatches(m.matches||[]);setActions(a.actions||[]);setAssets(media.assets||[]);
  }catch(e){setError(e instanceof Error?e.message:"关键词监控数据加载失败")}
 },[token]);
 useEffect(()=>{void load();const timer=window.setInterval(()=>void load(),15000);return()=>window.clearInterval(timer)},[load]);
 function edit(rule?:Rule){const value=rule?{...rule}:emptyRule();setDraft(value);setKeywordText((value.keywords||[]).join(", "));setExcludeText((value.exclude_terms||[]).join(", "))}
 async function save(e:FormEvent){
  e.preventDefault();if(!draft)return;
  setBusy(true);setError("");
  const payload={...draft,keywords:keywordText.split(/[,，\n]/).map(x=>x.trim()).filter(Boolean),exclude_terms:excludeText.split(/[,，\n]/).map(x=>x.trim()).filter(Boolean)};
  try{
   const result=await api<{rule:Rule}>(draft.id?`/api/v1/keyword-rules/${draft.id}`:"/api/v1/keyword-rules",token,{method:draft.id?"PUT":"POST",body:JSON.stringify(payload)});
   setRules(old=>draft.id?old.map(x=>x.id===draft.id?result.rule:x):[result.rule,...old]);setDraft(null);setNotice("关键词规则已保存");await load();
  }catch(e){setError(e instanceof Error?e.message:"规则保存失败")}
  finally{setBusy(false)}
 }
 async function remove(rule:Rule){
  if(!window.confirm(`确定删除规则「${rule.name}」？`))return;
  setBusy(true);try{await api(`/api/v1/keyword-rules/${rule.id}`,token,{method:"DELETE"});setRules(old=>old.filter(x=>x.id!==rule.id));setNotice("规则已删除")}
  catch(e){setError(e instanceof Error?e.message:"删除规则失败")}finally{setBusy(false)}
 }
 async function control(action:Action,op:"pause"|"resume"|"retry"){
  setBusy(true);setError("");
  try{await api(`/api/v1/keyword-actions/${action.id}/${op}`,token,{method:"POST"});setNotice("动作任务状态已更新");await load()}
  catch(e){setError(e instanceof Error?e.message:"动作任务操作失败")}finally{setBusy(false)}
 }
 async function uploadAsset(file:File){
  const form=new FormData();form.append("file",file);
  setBusy(true);
  try{await api("/api/v1/keyword-media-assets",token,{method:"POST",body:form});setNotice("媒体素材已上传");await load()}
  catch(e){setError(e instanceof Error?e.message:"媒体素材上传失败")}finally{setBusy(false)}
 }
 const shownActions=actions.filter(x=>!status||x.status===status);
 return <section className="keyword-monitor">
  <div className="keyword-toolbar"><div><h2>关键词监控</h2><p>规则匹配、命中追踪与自动动作执行；每个租户的数据相互隔离。</p></div><button className="secondary" onClick={()=>void load()} disabled={busy}>刷新数据</button></div>
  {error&&<div className="alert error">{error}</div>}{notice&&<div className="alert success">{notice}</div>}
  <div className="keyword-tabs">
   <button className={tab==="rules"?"active":""} onClick={()=>setTab("rules")}>规则配置（{rules.length}）</button>
   <button className={tab==="matches"?"active":""} onClick={()=>setTab("matches")}>命中记录（{matches.length}）</button>
   <button className={tab==="actions"?"active":""} onClick={()=>setTab("actions")}>动作任务（{actions.length}）</button>
  </div>
  {tab==="rules"&&<>
   <div className="keyword-toolbar"><strong>监控规则</strong><button className="primary" onClick={()=>edit()}>新建规则</button></div>
   <div className="keyword-rule-list">{rules.map(rule=><article className="keyword-card" key={rule.id}>
    <div><strong>{rule.name}</strong><span className={rule.enabled?"keyword-state enabled":"keyword-state"}>{rule.enabled?"已启用":"已停用"}</span><p>关键词：{(rule.keywords||[]).join("、")||"媒体类型匹配"} · 模式：{rule.match_mode} · 动作：{rule.action_type}</p><small>排除词：{(rule.exclude_terms||[]).join("、")||"无"} · 会话：{(rule.chat_types||[]).join("、")}</small></div>
    <div className="keyword-actions"><button className="secondary" onClick={()=>edit(rule)}>编辑</button><button className="secondary danger" onClick={()=>void remove(rule)} disabled={busy}>删除</button></div>
   </article>)}{!rules.length&&<div className="empty">尚未配置关键词规则</div>}</div>
   <div className="keyword-assets"><strong>媒体素材库</strong><label className="secondary">上传素材<input type="file" hidden accept="image/*,video/*,audio/*,.pdf,.doc,.docx" onChange={e=>{const file=e.target.files?.[0];if(file)void uploadAsset(file);e.currentTarget.value=""}}/></label><div>{assets.map(a=><span key={a.id}>{a.file_name} · {(a.file_size/1024).toFixed(0)} KB</span>)}</div></div>
  </>}
  {tab==="matches"&&<div className="keyword-table-wrap"><table className="keyword-table"><thead><tr><th>规则</th><th>账号</th><th>Chat ID</th><th>消息 ID</th><th>命中词</th><th>时间</th></tr></thead><tbody>{matches.map(m=><tr key={m.id}><td>{m.rule_name}</td><td>{m.account_id.slice(0,8)}…</td><td>{m.chat_id}</td><td>{m.message_id}</td><td>{m.matched_terms.join("、")}</td><td>{date(m.matched_at)}</td></tr>)}</tbody></table>{!matches.length&&<div className="empty">暂无命中记录</div>}</div>}
  {tab==="actions"&&<><div className="keyword-filter"><label>任务状态<select value={status} onChange={e=>setStatus(e.target.value)}><option value="">全部状态</option>{["pending","processing","sent","failed","paused"].map(x=><option key={x}>{x}</option>)}</select></label></div><div className="keyword-rule-list">{shownActions.map(a=><article className="keyword-card" key={a.id}><div><strong>{a.rule_name} · {a.action_type}</strong><span className="keyword-state">{a.status}</span><p>Chat {a.chat_id} · 尝试 {a.attempts} 次 · {date(a.created_at)}</p>{a.last_error&&<small className="keyword-error">{a.last_error}</small>}{a.telegram_message_ids?.length>0&&<small>Telegram 消息 ID：{a.telegram_message_ids.join(", ")}</small>}</div><div className="keyword-actions">{a.status==="pending"||a.status==="failed"?<button className="secondary" disabled={busy} onClick={()=>void control(a,"pause")}>暂停</button>:null}{a.status==="paused"?<button className="secondary" disabled={busy} onClick={()=>void control(a,"resume")}>恢复</button>:null}{a.status==="failed"?<button className="primary" disabled={busy} onClick={()=>void control(a,"retry")}>重试</button>:null}</div></article>)}{!shownActions.length&&<div className="empty">暂无符合条件的动作任务</div>}</div></>}
  {draft&&<div className="modal-backdrop"><form className="account-modal keyword-editor" onSubmit={save}><div className="modal-heading"><div><h2>{draft.id?"编辑关键词规则":"新建关键词规则"}</h2><p>保存后由实时消息监听器匹配新消息。</p></div><button type="button" className="ghost" onClick={()=>setDraft(null)}>关闭</button></div>
   <label>规则名称<input required maxLength={80} value={draft.name} onChange={e=>setDraft({...draft,name:e.target.value})}/></label>
   <label>关键词（逗号分隔）<textarea rows={3} value={keywordText} onChange={e=>setKeywordText(e.target.value)} placeholder="例如：招聘, 合作, 优惠"/></label>
   <label>排除词（逗号分隔）<input value={excludeText} onChange={e=>setExcludeText(e.target.value)} placeholder="例如：广告, 违规"/></label>
   <div className="keyword-editor-grid"><label>匹配模式<select value={draft.match_mode} onChange={e=>setDraft({...draft,match_mode:e.target.value})}><option value="any">任意关键词</option><option value="all">全部关键词</option><option value="exact">完整文本</option><option value="regex">正则表达式</option></select></label><label>动作类型<select value={draft.action_type} onChange={e=>{const action=e.target.value;setDraft({...draft,action_type:action,reply_text:"",contact_phone:"",contact_first_name:"",contact_last_name:"",media_asset_id:"",media_type:"photo",media_caption:"",chat_types:action==="text"?draft.chat_types:draft.chat_types.filter(x=>x!=="channel")})}}><option value="text">自动文本回复</option><option value="contact">发送联系人</option><option value="media">发送媒体素材</option></select></label></div>
   {draft.action_type==="text"&&<label>自动回复内容<textarea rows={3} maxLength={4096} value={draft.reply_text} onChange={e=>setDraft({...draft,reply_text:e.target.value})} placeholder="匹配成功后回复的文本"/></label>}
   {draft.action_type==="contact"&&<div className="keyword-editor-grid"><label>联系人手机号<input required value={draft.contact_phone||""} onChange={e=>setDraft({...draft,contact_phone:e.target.value})}/></label><label>联系人名字<input required value={draft.contact_first_name||""} onChange={e=>setDraft({...draft,contact_first_name:e.target.value})}/></label><label>联系人姓氏<input value={draft.contact_last_name||""} onChange={e=>setDraft({...draft,contact_last_name:e.target.value})}/></label></div>}
   {draft.action_type==="media"&&<><label>媒体素材<select required value={draft.media_asset_id||""} onChange={e=>setDraft({...draft,media_asset_id:e.target.value})}><option value="">选择已上传素材</option>{assets.map(a=><option key={a.id} value={a.id}>{a.file_name} · {a.mime_type}</option>)}</select></label><div className="keyword-editor-grid"><label>Telegram 媒体类型<select value={draft.media_type||"photo"} onChange={e=>setDraft({...draft,media_type:e.target.value})}>{["photo","video","document","audio","voice","video_note"].map(x=><option key={x}>{x}</option>)}</select></label><label>媒体说明<input maxLength={1024} value={draft.media_caption||""} onChange={e=>setDraft({...draft,media_caption:e.target.value})}/></label></div></>}
   <div className="keyword-checks"><label><input type="checkbox" checked={draft.chat_types.includes("private")} onChange={e=>setDraft({...draft,chat_types:e.target.checked?[...draft.chat_types,"private"]:draft.chat_types.filter(x=>x!=="private")})}/>私聊</label><label><input type="checkbox" checked={draft.chat_types.includes("group")} onChange={e=>setDraft({...draft,chat_types:e.target.checked?[...draft.chat_types,"group"]:draft.chat_types.filter(x=>x!=="group")})}/>群组</label><label><input type="checkbox" checked={draft.chat_types.includes("channel")} onChange={e=>setDraft({...draft,chat_types:e.target.checked?[...draft.chat_types,"channel"]:draft.chat_types.filter(x=>x!=="channel")})}/>频道（仅文本）</label><label><input type="checkbox" checked={draft.case_sensitive} onChange={e=>setDraft({...draft,case_sensitive:e.target.checked})}/>区分大小写</label><label><input type="checkbox" checked={draft.enabled} onChange={e=>setDraft({...draft,enabled:e.target.checked})}/>启用规则</label></div>
   <button className="primary" disabled={busy||!draft.chat_types.length||(draft.action_type!=="text"&&draft.chat_types.includes("channel"))||(draft.action_type==="media"&&!draft.media_asset_id)}>保存规则</button>
  </form></div>}
 </section>;
}
