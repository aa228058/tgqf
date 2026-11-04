"use client";

import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { KeywordMonitor } from "./KeywordMonitor";

const API_BASE=(process.env.NEXT_PUBLIC_API_BASE_URL||"").replace(/\/$/,"");
type Chat={account_id:string;account_name:string;chat_id:number;chat_title:string;chat_type:string;unprocessed_count:number;last_message:string;last_message_date:string;chat_username:string;member_count:number;is_verified:boolean};
type Message={id:string;account_id:string;account_name:string;chat_id:number;message_id:number;chat_type:string;chat_title:string;sender_id:number;reply_to_message_id?:number;text:string;media_type:string;grouped_id?:number;message_date:string;processed_at?:string};
type MediaPreviewItem={url:string;type:string;messageID:string;fileName:string};
type Page<T>={total:number;messages?:T[];chats?:T[];limit:number;offset:number};
type Settings={enabled:boolean;include_all_accounts:boolean;account_ids:string[];chat_ids:number[];include_text:boolean;include_media:boolean;keywords:string[]};
type Account={id:string;phone:string;display_name:string;status:string};
type Reply={id:string;status:string;last_error?:string;telegram_message_id?:number};

async function request<T>(path:string,token:string,init?:RequestInit):Promise<T>{
 const headers:Record<string,string>={Authorization:`Bearer ${token}`};
 if(!(init?.body instanceof FormData))headers["Content-Type"]="application/json";
 Object.assign(headers,init?.headers||{});
 const response=await fetch(`${API_BASE}${path}`,{...init,headers,cache:"no-store"});
 if(!response.ok){let message=`请求失败（${response.status}）`;try{message=(await response.json()).error||message}catch{}throw new Error(message)}
 if(response.status===204)return undefined as T;
 return response.json() as Promise<T>;
}
const fmt=(v:string)=>new Date(v).toLocaleString();

export function MessagesPage({token}:{token:string}){
 const [chats,setChats]=useState<Chat[]>([]),[chatTotal,setChatTotal]=useState(0),[accounts,setAccounts]=useState<Account[]>([]);
 const [selectedChat,setSelectedChat]=useState<Chat|null>(null),[messages,setMessages]=useState<Message[]>([]),[messageTotal,setMessageTotal]=useState(0);
 const [query,setQuery]=useState(""),[debouncedQuery,setDebouncedQuery]=useState(""),[chatQuery,setChatQuery]=useState(""),[mediaType,setMediaType]=useState(""),[includeProcessed,setIncludeProcessed]=useState(false);
 const messageLoadSequence=useRef(0);
 useEffect(()=>{const timer=window.setTimeout(()=>setDebouncedQuery(query),260);return()=>window.clearTimeout(timer)},[query]);
 const [offset,setOffset]=useState(0),[busy,setBusy]=useState(false),[error,setError]=useState(""),[notice,setNotice]=useState("");
 const [settings,setSettings]=useState<Settings|null>(null),[settingsOpen,setSettingsOpen]=useState(false),[settingsBusy,setSettingsBusy]=useState(false);
 const [replyText,setReplyText]=useState<Record<string,string>>({}),[contactMessage,setContactMessage]=useState<string|null>(null);
 const [mediaPreview,setMediaPreview]=useState<{items:MediaPreviewItem[];activeIndex:number}|null>(null);
 const [keywordOpen,setKeywordOpen]=useState(false);
 const [contact,setContact]=useState({phone:"",first_name:"",last_name:""}),[replyJobs,setReplyJobs]=useState<Record<string,Reply>>({});

 const loadChats=useCallback(async()=>{
  const result=await request<Page<Chat>>("/api/v1/messages/chats?limit=100&offset=0",token);
  setChats(result.chats||[]);setChatTotal(result.total||0);
  if(selectedChat&&!((result.chats||[]).some(c=>c.account_id===selectedChat.account_id&&c.chat_id===selectedChat.chat_id)))setSelectedChat(null);
 },[token,selectedChat]);
 const loadAccounts=useCallback(async()=>{
  const result=await request<{accounts:Account[]}>("/api/v1/accounts?limit=100&offset=0",token);setAccounts(result.accounts||[]);
 },[token]);
 const loadMessages=useCallback(async()=>{
  const sequence=++messageLoadSequence.current;
  if(!selectedChat){setMessages([]);setMessageTotal(0);setBusy(false);return}
  setBusy(true);setError("");
  try{
   const params=new URLSearchParams({limit:"50",offset:String(offset),account_id:selectedChat.account_id,chat_id:String(selectedChat.chat_id),chat_type:selectedChat.chat_type,include_processed:String(includeProcessed)});
   if(debouncedQuery.trim())params.set("q",debouncedQuery.trim());
   if(mediaType)params.set("media_type",mediaType);
   const result=await request<Page<Message>>(`/api/v1/messages?${params}`,token);
   if(sequence===messageLoadSequence.current){setMessages(result.messages||[]);setMessageTotal(result.total||0)}
  }catch(e){if(sequence===messageLoadSequence.current)setError(e instanceof Error?e.message:"消息加载失败")}
  finally{if(sequence===messageLoadSequence.current)setBusy(false)}
 },[token,selectedChat,offset,includeProcessed,debouncedQuery,mediaType]);
 useEffect(()=>{void Promise.all([loadChats(),loadAccounts()]).catch(e=>setError(e instanceof Error?e.message:"消息初始化失败"))},[loadChats,loadAccounts]);
 useEffect(()=>{void loadMessages()},[loadMessages]);
 useEffect(()=>{
  const timer=window.setInterval(()=>{if(document.visibilityState==="visible"){void loadChats();void loadMessages()}},15000);
  return()=>window.clearInterval(timer);
 },[loadChats,loadMessages]);
 const visibleChats=useMemo(()=>chats.filter(c=>!chatQuery||[c.chat_title,c.account_name,c.chat_username,String(c.chat_id)].some(v=>v.toLowerCase().includes(chatQuery.toLowerCase()))),[chats,chatQuery]);

 async function openSettings(){
  setSettingsBusy(true);setError("");
  try{const result=await request<{settings:Settings}>("/api/v1/messages/settings",token);setSettings(result.settings);setSettingsOpen(true)}
  catch(e){setError(e instanceof Error?e.message:"消息采集设置读取失败")}
  finally{setSettingsBusy(false)}
 }
 async function saveSettings(event:FormEvent){
  event.preventDefault();if(!settings)return;setSettingsBusy(true);setError("");
  try{const result=await request<{settings:Settings}>("/api/v1/messages/settings",token,{method:"PUT",body:JSON.stringify(settings)});setSettings(result.settings);setSettingsOpen(false);setNotice("消息采集设置已保存")}
  catch(e){setError(e instanceof Error?e.message:"消息采集设置保存失败")}
  finally{setSettingsBusy(false)}
 }
 async function previewMedia(message:Message){
  setBusy(true);setError("");
  const created:MediaPreviewItem[]=[];
  let totalBytes=0;
  try{
   let album=[message];
   if(message.grouped_id){
    const result=await request<{messages:Message[]}>(`/api/v1/messages/${message.id}/album`,token);
    album=(result.messages||[]).filter(item=>item.media_type);
   }
   for(const item of album){
    const response=await fetch(`${API_BASE}/api/v1/messages/${item.id}/media`,{headers:{Authorization:`Bearer ${token}`},cache:"no-store"});
    if(!response.ok)throw new Error(`消息 ${item.message_id} 的媒体读取失败`);
    const type=response.headers.get("Content-Type")||"application/octet-stream";
    const disposition=response.headers.get("Content-Disposition")||"";
    const nameMatch=disposition.match(/filename="?([^";]+)"?/i);
    const blob=await response.blob();
    totalBytes+=blob.size;
    if(totalBytes>100*1024*1024)throw new Error("相册预览总大小超过 100 MiB，请逐条查看媒体");
    created.push({url:URL.createObjectURL(blob),type,messageID:item.id,fileName:nameMatch?.[1]||`telegram-media-${item.message_id}`});
   }
   if(!created.length)throw new Error("该消息没有可预览的媒体内容");
   setMediaPreview(old=>{if(old)old.items.forEach(item=>URL.revokeObjectURL(item.url));return {items:created,activeIndex:0}});
  }catch(e){
   created.forEach(item=>URL.revokeObjectURL(item.url));
   setError(e instanceof Error?e.message:"媒体预览失败");
  }finally{setBusy(false)}
 }
 function closeMediaPreview(){
  if(mediaPreview)mediaPreview.items.forEach(item=>URL.revokeObjectURL(item.url));
  setMediaPreview(null);
 }
 function moveMediaPreview(step:number){
  setMediaPreview(current=>current?{...current,activeIndex:(current.activeIndex+step+current.items.length)%current.items.length}:current);
 }
 useEffect(()=>()=>{if(mediaPreview)mediaPreview.items.forEach(item=>URL.revokeObjectURL(item.url))},[mediaPreview]);

 async function markProcessed(message:Message){
  setBusy(true);
  try{await request(`/api/v1/messages/${message.id}/processed`,token,{method:"PATCH"});setNotice("消息已标记处理");await Promise.all([loadMessages(),loadChats()])}
  catch(e){setError(e instanceof Error?e.message:"标记处理失败")}
  finally{setBusy(false)}
 }
 async function sendReply(message:Message,event:FormEvent){
  event.preventDefault();const text=replyText[message.id]?.trim();if(!text)return;
  setBusy(true);setError("");
  try{
   const result=await request<{reply:Reply}>(`/api/v1/messages/${message.id}/reply`,token,{method:"POST",headers:{"Idempotency-Key":crypto.randomUUID()},body:JSON.stringify({text})});
   setReplyJobs(v=>({...v,[message.id]:result.reply}));setReplyText(v=>({...v,[message.id]:""}));setNotice("回复任务已加入队列");
  }catch(e){setError(e instanceof Error?e.message:"创建回复任务失败")}
  finally{setBusy(false)}
 }
 async function sendContact(message:Message,event:FormEvent){
  event.preventDefault();setBusy(true);setError("");
  try{
   const result=await request<{reply:Reply}>(`/api/v1/messages/${message.id}/contact`,token,{method:"POST",headers:{"Idempotency-Key":crypto.randomUUID()},body:JSON.stringify(contact)});
   setReplyJobs(v=>({...v,[message.id]:result.reply}));setContactMessage(null);setContact({phone:"",first_name:"",last_name:""});setNotice("联系人卡片发送任务已加入队列");
  }catch(e){setError(e instanceof Error?e.message:"联系人卡片任务创建失败")}
  finally{setBusy(false)}
 }
 async function refreshReply(messageID:string,jobID:string){
  try{const result=await request<{reply:Reply}>(`/api/v1/message-replies/${jobID}`,token);setReplyJobs(v=>({...v,[messageID]:result.reply}))}
  catch(e){setError(e instanceof Error?e.message:"回复任务状态读取失败")}
 }
 async function resumeReply(messageID:string,jobID:string){
  try{await request(`/api/v1/message-replies/${jobID}/resume`,token,{method:"POST"});await refreshReply(messageID,jobID);setNotice("回复任务已恢复")}
  catch(e){setError(e instanceof Error?e.message:"恢复回复任务失败")}
 }
 async function selectChat(chat:Chat){setSelectedChat(chat);setMessages([]);setMessageTotal(0);setOffset(0);setQuery("");setDebouncedQuery("");setMediaType("");setIncludeProcessed(false)}
 function updateSettings<K extends keyof Settings>(key:K,value:Settings[K]){setSettings(s=>s?{...s,[key]:value}:s)}
 function toggleSettingID(key:"account_ids"|"chat_ids",value:string|number,checked:boolean){
  if(!settings)return;
  if(key==="account_ids"){const id=String(value);updateSettings(key,checked?[...settings.account_ids,id]:settings.account_ids.filter(x=>x!==id))}
  else{const id=Number(value);updateSettings(key,checked?[...settings.chat_ids,id]:settings.chat_ids.filter(x=>x!==id))}
 }

 return <div className="messages-page">
  <div className="messages-heading"><div><div className="eyebrow">MESSAGES</div><h1>消息管理</h1><p>按账号和会话查看实时采集消息，支持处理、回复和联系人卡片。</p></div><div className="messages-heading-actions"><button className="secondary" onClick={()=>void Promise.all([loadChats(),loadMessages()])} disabled={busy}>刷新</button><button className="secondary" onClick={()=>setKeywordOpen(v=>!v)}>{keywordOpen?"返回消息":"关键词监控"}</button><button className="secondary" onClick={()=>void openSettings()} disabled={settingsBusy}>采集设置</button></div></div>
  {error&&<div className="alert error">{error}</div>}{notice&&<div className="alert success">{notice}</div>}
  {keywordOpen?<KeywordMonitor token={token}/>:<div className="messages-layout">
   <aside className="chat-sidebar"><div className="chat-sidebar-heading"><strong>会话列表</strong><span>{chatTotal}</span></div><input className="chat-search" value={chatQuery} onChange={e=>setChatQuery(e.target.value)} placeholder="搜索会话、账号或用户名"/>
    <div className="chat-list">{visibleChats.map(chat=><button key={chat.account_id+":"+chat.chat_id} className={selectedChat?.account_id===chat.account_id&&selectedChat.chat_id===chat.chat_id?"chat-item active":"chat-item"} onClick={()=>void selectChat(chat)}><div className="chat-avatar">{(chat.chat_title||chat.account_name||"?").slice(0,1).toUpperCase()}</div><div className="chat-item-content"><strong>{chat.chat_title||String(chat.chat_id)}</strong><small>{chat.account_name} · {chat.chat_type}</small><small className="chat-last-message">{chat.last_message||"无文本内容"}</small></div>{chat.unprocessed_count>0&&<span className="chat-unread">{chat.unprocessed_count}</span>}</button>)}{!visibleChats.length&&<div className="empty">暂无会话</div>}</div>
   </aside>
   <section className="message-pane">
    {!selectedChat?<div className="message-empty"><div>▤</div><h2>选择一个会话</h2><p>从左侧会话列表选择账号和聊天，查看该会话已采集的消息。</p></div>:<>
     <div className="message-pane-heading"><div><h2>{selectedChat.chat_title||String(selectedChat.chat_id)}</h2><p>{selectedChat.account_name} · {selectedChat.chat_type} · Chat ID {selectedChat.chat_id}</p></div><span>{selectedChat.unprocessed_count} 条待处理</span></div>
     <div className="message-filters"><input value={query} onChange={e=>{setQuery(e.target.value);setOffset(0)}} placeholder="搜索消息文本"/><select value={mediaType} onChange={e=>{setMediaType(e.target.value);setOffset(0)}}><option value="">全部媒体</option><option value="photo">图片</option><option value="video">视频</option><option value="document">文件</option><option value="voice">语音</option><option value="audio">音频</option><option value="sticker">贴纸</option><option value="animation">动画</option></select><label><input type="checkbox" checked={includeProcessed} onChange={e=>{setIncludeProcessed(e.target.checked);setOffset(0)}}/>包含已处理</label><span>共 {messageTotal} 条</span></div>
     <div className="message-list">{messages.map(message=><article className={message.processed_at?"message-card processed":"message-card"} key={message.id}><div className="message-card-meta"><span>{message.account_name} · {message.sender_id}</span><time>{fmt(message.message_date)}</time></div><div className="message-card-content">{message.text||<span className="muted">（无文本内容）</span>}{message.media_type&&<><span className="media-chip">{message.media_type}</span><button className="media-preview-trigger" onClick={()=>void previewMedia(message)} disabled={busy}>预览媒体{message.grouped_id?"（相册）":""}</button></>}</div><div className="message-card-actions">{message.processed_at?<span className="processed-label">已处理 · {fmt(message.processed_at)}</span>:<button className="secondary" onClick={()=>void markProcessed(message)} disabled={busy}>标记已处理</button>}<button className="secondary" onClick={()=>setContactMessage(contactMessage===message.id?null:message.id)}>发送联系人卡片</button></div>
      {contactMessage===message.id&&<form className="contact-composer" onSubmit={e=>void sendContact(message,e)}><input value={contact.phone} onChange={e=>setContact(v=>({...v,phone:e.target.value}))} placeholder="联系人手机号" required/><input value={contact.first_name} onChange={e=>setContact(v=>({...v,first_name:e.target.value}))} placeholder="名字" required/><input value={contact.last_name} onChange={e=>setContact(v=>({...v,last_name:e.target.value}))} placeholder="姓氏"/><button className="primary" disabled={busy}>发送原生联系人卡片</button></form>}
      <form className="reply-composer" onSubmit={e=>void sendReply(message,e)}><textarea value={replyText[message.id]||""} onChange={e=>setReplyText(v=>({...v,[message.id]:e.target.value}))} placeholder="输入回复内容…" rows={2}/><button className="primary" disabled={busy||!(replyText[message.id]||"").trim()}>发送回复</button></form>
      {replyJobs[message.id]&&<div className="reply-job-status"><span>回复任务：{replyJobs[message.id].status}</span>{replyJobs[message.id].last_error&&<small>{replyJobs[message.id].last_error}</small>}<button onClick={()=>void refreshReply(message.id,replyJobs[message.id].id)}>刷新状态</button>{["paused","failed"].includes(replyJobs[message.id].status)&&<button onClick={()=>void resumeReply(message.id,replyJobs[message.id].id)}>恢复任务</button>}</div>}
     </article>)}{!messages.length&&<div className="empty">{busy?"正在加载消息…":"该会话暂无符合条件的消息"}</div>}</div>
     <div className="message-pagination"><button className="secondary" disabled={offset===0||busy} onClick={()=>setOffset(Math.max(0,offset-50))}>上一页</button><span>第 {Math.floor(offset/50)+1} / {Math.max(1,Math.ceil(messageTotal/50))} 页</span><button className="secondary" disabled={offset+50>=messageTotal||busy} onClick={()=>setOffset(offset+50)}>下一页</button></div>
    </>}
   </section>
  </div>}
  {mediaPreview&&<div className="modal-backdrop media-preview-backdrop" onMouseDown={e=>{if(e.target===e.currentTarget)closeMediaPreview()}}><section className="media-preview-modal"><div className="modal-heading"><div><h2>消息媒体预览</h2><p>通过当前租户授权账号安全读取；{mediaPreview.activeIndex+1}/{mediaPreview.items.length} 项 · 不公开 Telegram 文件地址。</p></div><button type="button" className="ghost" onClick={closeMediaPreview}>关闭</button></div><div className="media-preview-content">{mediaPreview.items[mediaPreview.activeIndex].type.startsWith("image/")&&mediaPreview.items[mediaPreview.activeIndex].type!=="image/svg+xml"?<img src={mediaPreview.items[mediaPreview.activeIndex].url} alt={mediaPreview.items[mediaPreview.activeIndex].fileName}/>:mediaPreview.items[mediaPreview.activeIndex].type.startsWith("video/")?<video src={mediaPreview.items[mediaPreview.activeIndex].url} controls/>:mediaPreview.items[mediaPreview.activeIndex].type.startsWith("audio/")?<audio src={mediaPreview.items[mediaPreview.activeIndex].url} controls/>:mediaPreview.items[mediaPreview.activeIndex].type==="application/pdf"?<iframe src={mediaPreview.items[mediaPreview.activeIndex].url} title={mediaPreview.items[mediaPreview.activeIndex].fileName}/>:<a href={mediaPreview.items[mediaPreview.activeIndex].url} download={mediaPreview.items[mediaPreview.activeIndex].fileName}>下载 {mediaPreview.items[mediaPreview.activeIndex].fileName}</a>}</div>{mediaPreview.items.length>1&&<div className="media-preview-navigation"><button className="secondary" onClick={()=>moveMediaPreview(-1)}>上一项</button><button className="secondary" onClick={()=>moveMediaPreview(1)}>下一项</button></div>}</section></div>}
  {settingsOpen&&settings&&<div className="modal-backdrop"><form className="account-modal message-settings" onSubmit={saveSettings}><div className="modal-heading"><div><h2>消息采集设置</h2><p>设置只影响后续实时消息，不回溯历史消息。</p></div><button type="button" className="ghost" onClick={()=>setSettingsOpen(false)}>关闭</button></div>
   <label className="setting-toggle"><input type="checkbox" checked={settings.enabled} onChange={e=>updateSettings("enabled",e.target.checked)}/>启用消息采集</label>
   <label className="setting-toggle"><input type="checkbox" checked={settings.include_all_accounts} onChange={e=>updateSettings("include_all_accounts",e.target.checked)}/>采集全部账号</label>
   {!settings.include_all_accounts&&<div className="settings-checklist"><strong>指定账号</strong>{accounts.map(a=><label key={a.id}><input type="checkbox" checked={settings.account_ids.includes(a.id)} onChange={e=>toggleSettingID("account_ids",a.id,e.target.checked)}/>{a.display_name||a.phone} · {a.phone}</label>)}</div>}
   <label className="setting-toggle"><input type="checkbox" checked={settings.include_text} onChange={e=>updateSettings("include_text",e.target.checked)}/>采集文本消息</label>
   <label className="setting-toggle"><input type="checkbox" checked={settings.include_media} onChange={e=>updateSettings("include_media",e.target.checked)}/>采集媒体消息</label>
   <div className="settings-checklist"><strong>关键词过滤（留空表示不过滤）</strong><input value={settings.keywords.join(", ")} onChange={e=>updateSettings("keywords",e.target.value.split(",").map(x=>x.trim()).filter(Boolean))} placeholder="关键词，逗号分隔"/></div>
   <div className="settings-checklist"><strong>指定 Chat ID（留空表示全部）</strong><input value={settings.chat_ids.join(", ")} onChange={e=>updateSettings("chat_ids",e.target.value.split(",").map(x=>Number(x.trim())).filter(x=>Number.isSafeInteger(x)&&x!==0))} placeholder="负数群组 ID，逗号分隔"/></div>
   <button className="primary" disabled={settingsBusy}>保存采集设置</button>
  </form></div>}
 </div>;
}
