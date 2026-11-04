"use client";

import { useCallback, useEffect, useState } from "react";

const API=(process.env.NEXT_PUBLIC_API_BASE_URL||"").replace(/\/$/,"");
type Correction={id:number;group_id:number;tenant_name:string;submitted_by:string;proposed_fields:{title?:string;username?:string;member_count?:number;is_verified?:boolean};status:string;review_note:string;created_at:string};
type Group={id:number;title:string;chat_id:number;username:string;chat_type:string};
async function request<T>(path:string,token:string,init?:RequestInit):Promise<T>{
 const headers:Record<string,string>={Authorization:`Bearer ${token}`};
 if(!(init?.body instanceof FormData))headers["Content-Type"]="application/json";
 Object.assign(headers,init?.headers||{});
 const response=await fetch(API+path,{...init,headers,cache:"no-store"});
 if(!response.ok){let message=`请求失败（${response.status}）`;try{message=(await response.json()).error||message}catch{}throw new Error(message)}
 return response.json() as Promise<T>;
}
const date=(value:string)=>new Date(value).toLocaleString();

export function AdminModerationPage({token}:{token:string}){
 const [items,setItems]=useState<Correction[]>([]);
 const [groups,setGroups]=useState<Group[]>([]);
 const [status,setStatus]=useState("pending");
 const [sourceID,setSourceID]=useState("");
 const [targetID,setTargetID]=useState("");
 const [reason,setReason]=useState("");
 const [notes,setNotes]=useState<Record<number,string>>({});
 const [busy,setBusy]=useState(false);
 const [error,setError]=useState("");
 const [notice,setNotice]=useState("");
 const load=useCallback(async()=>{
  setBusy(true);setError("");
  try{
   const [corrections,groupResult]=await Promise.all([
    request<{corrections:Correction[]}>(`/api/v1/admin/group-corrections?status=${status}&limit=100&offset=0`,token),
    request<{groups:Group[]}>(`/api/v1/groups?limit=100&offset=0&sort=title`,token)
   ]);
   setItems(corrections.corrections||[]);setGroups(groupResult.groups||[]);
  }catch(e){setError(e instanceof Error?e.message:"管理数据加载失败")}
  finally{setBusy(false)}
 },[token,status]);
 useEffect(()=>{void load()},[load]);
 async function review(item:Correction,decision:"approve"|"reject"){
  setBusy(true);setError("");setNotice("");
  try{
   await request(`/api/v1/admin/group-corrections/${item.id}/review`,token,{method:"POST",body:JSON.stringify({decision,note:notes[item.id]||""})});
   setNotice(decision==="approve"?"群组资料纠错已通过":"群组资料纠错已拒绝");await load();
  }catch(e){setError(e instanceof Error?e.message:"审核操作失败")}
  finally{setBusy(false)}
 }
 async function merge(){
  const source=Number(sourceID),target=Number(targetID);
  if(!Number.isSafeInteger(source)||source<=0||!Number.isSafeInteger(target)||target<=0||source===target){setError("请选择不同的来源群组 ID 和目标群组 ID");return}
  if(reason.trim().length<3){setError("请填写至少 3 个字符的合并原因");return}
  if(!window.confirm(`确定将群组 ${source} 合并到 ${target} 吗？此操作不可撤销。`))return;
  setBusy(true);setError("");setNotice("");
  try{
   await request(`/api/v1/admin/groups/${source}/merge`,token,{method:"POST",body:JSON.stringify({target_group_id:target,reason:reason.trim()})});
   setNotice("公共群组已合并，来源与贡献关系由后端迁移并记录审计");setSourceID("");setTargetID("");setReason("");await load();
  }catch(e){setError(e instanceof Error?e.message:"群组合并失败")}
  finally{setBusy(false)}
 }
 return <div className="settings-page">
  <header className="settings-heading"><div><div className="eyebrow">ADMIN MODERATION</div><h1>公共群组审核</h1><p>仅超级管理员可审核全局群组资料纠错及执行有审计记录的人工合并</p></div><button className="secondary" onClick={()=>void load()} disabled={busy}>刷新</button></header>
  {error&&<div className="alert error" role="alert">{error}</div>}{notice&&<div className="alert success" role="status">{notice}</div>}
  <section className="panel settings-panel">
   <div className="settings-section-heading"><div><h2>群组资料纠错队列</h2><p>租户提交的建议不会直接修改公共总库，必须经管理员审核。</p></div><select value={status} onChange={e=>setStatus(e.target.value)}><option value="pending">待审核</option><option value="approved">已通过</option><option value="rejected">已拒绝</option></select></div>
   <div className="settings-table-wrap"><table className="settings-table"><thead><tr><th>群组 ID</th><th>提交租户 / 用户</th><th>建议字段</th><th>提交时间</th><th>审核备注</th><th>操作</th></tr></thead><tbody>
    {items.map(item=><tr key={item.id}><td>{item.group_id}<small>建议 #{item.id}</small></td><td>{item.tenant_name}<small>{item.submitted_by}</small></td><td><pre>{JSON.stringify(item.proposed_fields,null,2)}</pre></td><td>{date(item.created_at)}</td><td><input value={notes[item.id]||""} maxLength={500} onChange={e=>setNotes(v=>({...v,[item.id]:e.target.value}))} placeholder="审核备注（可选）"/></td><td>{item.status==="pending"?<div className="tenant-actions"><button className="primary" onClick={()=>void review(item,"approve")} disabled={busy}>通过</button><button className="danger-text" onClick={()=>void review(item,"reject")} disabled={busy}>拒绝</button></div>:<span>{item.status}</span>}</td></tr>)}
    {!items.length&&<tr><td colSpan={6} className="empty">{busy?"正在加载…":"当前筛选下没有纠错记录"}</td></tr>}
   </tbody></table></div>
  </section>
  <section className="panel settings-panel">
   <h2>人工合并重复群组</h2><p>只允许明确指定来源和目标，不根据相似名称自动合并；后端会迁移来源/贡献/链接关系并写入审计快照。</p>
   <div className="settings-inline-form">
    <label>来源群组<select value={sourceID} onChange={e=>setSourceID(e.target.value)}><option value="">选择来源群组</option>{groups.map(g=><option key={g.id} value={g.id}>{g.id} · {g.title||g.username||g.chat_id}</option>)}</select></label>
    <label>目标群组<select value={targetID} onChange={e=>setTargetID(e.target.value)}><option value="">选择目标群组</option>{groups.map(g=><option key={g.id} value={g.id}>{g.id} · {g.title||g.username||g.chat_id}</option>)}</select></label>
    <label>合并原因<input value={reason} onChange={e=>setReason(e.target.value)} maxLength={500} placeholder="说明为什么确认是同一群组"/></label>
    <button className="danger-text" onClick={()=>void merge()} disabled={busy||!sourceID||!targetID}>执行合并</button>
   </div>
  </section>
 </div>;
}
