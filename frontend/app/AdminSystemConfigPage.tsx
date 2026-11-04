"use client";

import {useCallback,useEffect,useState} from "react";

const API=(process.env.NEXT_PUBLIC_API_BASE_URL||"").replace(/\/$/,"");
type ConfigValue=boolean|number;
type ConfigItem={key:string;value:ConfigValue;description:string;updated_at:string};
const labels:Record<string,string>={registration_enabled:"开放租户注册",default_account_limit:"新租户默认账号额度",auth_session_ttl_hours:"登录会话有效期（小时）"};

async function request<T>(path:string,token:string,init?:RequestInit):Promise<T>{
 const response=await fetch(API+path,{...init,headers:{Authorization:"Bearer "+token,"Content-Type":"application/json",...(init?.headers||{})},cache:"no-store"});
 if(!response.ok){let message=`请求失败（${response.status}）`;try{message=(await response.json()).error||message}catch{}throw new Error(message)}
 return response.json() as Promise<T>;
}

export function AdminSystemConfigPage({token}:{token:string}){
 const [items,setItems]=useState<ConfigItem[]>([]);
 const [draft,setDraft]=useState<Record<string,ConfigValue>>({});
 const [loading,setLoading]=useState(true);
 const [saving,setSaving]=useState("");
 const [error,setError]=useState("");
 const [notice,setNotice]=useState("");
 const load=useCallback(async()=>{
  setLoading(true);setError("");
  try{const result=await request<{configs:ConfigItem[]}>("/api/v1/admin/system-config",token);setItems(result.configs||[]);setDraft(Object.fromEntries((result.configs||[]).map(item=>[item.key,item.value])))}
  catch(e){setError(e instanceof Error?e.message:"系统配置加载失败")}
  finally{setLoading(false)}
 },[token]);
 useEffect(()=>{void load()},[load]);
 async function save(item:ConfigItem){
  setSaving(item.key);setError("");setNotice("");
  try{const result=await request<{config:ConfigItem}>("/api/v1/admin/system-config/"+encodeURIComponent(item.key),token,{method:"PUT",body:JSON.stringify({value:draft[item.key]})});setItems(current=>current.map(value=>value.key===item.key?result.config:value));setNotice(labels[item.key]+"已保存并立即生效")}
  catch(e){setError(e instanceof Error?e.message:"配置保存失败")}
  finally{setSaving("")}
 }
 return <section className="panel settings-panel">
  <div className="settings-section-heading"><div><h2>系统配置中心</h2><p>仅超级管理员可修改。白名单配置会校验类型和范围，保存后立即生效并写入审计日志。</p></div><button className="secondary" onClick={()=>void load()} disabled={loading||!!saving}>刷新</button></div>
  {error&&<div className="alert error">{error}</div>}{notice&&<div className="alert success">{notice}</div>}
  {loading?<p>正在加载系统配置…</p>:<div className="settings-config-list">{items.map(item=><div className="settings-config-row" key={item.key}>
   <div className="settings-config-copy"><strong>{labels[item.key]||item.key}</strong><p>{item.description}</p><small>配置键：<code>{item.key}</code> · 最近更新：{new Date(item.updated_at).toLocaleString()}</small></div>
   <div className="settings-config-control">
    {typeof item.value==="boolean"?<label className="settings-toggle"><input type="checkbox" checked={Boolean(draft[item.key])} onChange={event=>setDraft(current=>({...current,[item.key]:event.target.checked}))}/><span>{draft[item.key]?"开启":"关闭"}</span></label>:<input type="number" min={item.key==="default_account_limit"?1:1} max={item.key==="default_account_limit"?10000:720} value={Number(draft[item.key]??item.value)} onChange={event=>setDraft(current=>({...current,[item.key]:Number(event.target.value)}))}/>}
    <button className="primary" disabled={!!saving||draft[item.key]===item.value} onClick={()=>void save(item)}>{saving===item.key?"保存中…":"保存"}</button>
   </div>
  </div>)}</div>}
  <div className="alert">数据库密码、Redis 密码、Telegram API Hash、Session 加密密钥等敏感凭据仅通过服务器环境变量或密钥管理设施配置；本页面不会读取或回显其明文。</div>
 </section>;
}
