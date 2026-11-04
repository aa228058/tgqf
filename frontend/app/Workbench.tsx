"use client";

import { useCallback, useEffect, useState } from "react";
import { AppUser } from "./AppShell";

const API_BASE=(process.env.NEXT_PUBLIC_API_BASE_URL||"").replace(/\/$/,"");
type Account={id:string;phone:string;display_name:string;status:string;last_error:string;created_at:string;updated_at:string};
type AccountPage={accounts:Account[];total:number};
type MessagePage={messages:unknown[];total:number};
type Task={id:string;type:string;title:string;status:string;account_name:string;attempts:number;created_at:string;updated_at:string;last_error?:string};
type TaskPage={tasks:Task[];total:number};
type Summary={total:number;processing:number;expired_leases:number;counts:{type:string;status:string;count:number}[];generated_at:string};
type Worker={instance_id:string;hostname:string;process_id:number;status:string;heartbeat_at:string;started_at:string;stopped_at?:string;stale:boolean};
type Workers={workers:Worker[];total:number;stale:number;stale_after_seconds:number};
type Result={total:number;items:Account[]};

async function get<T>(path:string,token:string):Promise<T>{
 const response=await fetch(`${API_BASE}${path}`,{headers:{Authorization:`Bearer ${token}`},cache:"no-store"});
 if(!response.ok){let message=`请求失败（${response.status}）`;try{const body=await response.json();message=body.error||message}catch{}throw new Error(message)}
 return response.json() as Promise<T>;
}
function time(value?:string){if(!value)return "—";const date=new Date(value);return Number.isNaN(date.getTime())?"—":date.toLocaleString()}
function Stat({label,value,detail}:{label:string;value:string|number;detail:string}){return <article className="workbench-stat"><span>{label}</span><strong>{value}</strong><small>{detail}</small></article>}

export function Workbench({token,user}:{token:string;user:AppUser}){
 const [total,setTotal]=useState(0),[authorized,setAuthorized]=useState(0),[errors,setErrors]=useState(0);
 const [pendingMessages,setPendingMessages]=useState(0),[taskSummary,setTaskSummary]=useState<Summary|null>(null);
 const [recentAccounts,setRecentAccounts]=useState<Account[]>([]),[recentTasks,setRecentTasks]=useState<Task[]>([]);
 const [workers,setWorkers]=useState<Workers|null>(null),[workerError,setWorkerError]=useState("");
 const [error,setError]=useState(""),[busy,setBusy]=useState(false),[updatedAt,setUpdatedAt]=useState("");

 const load=useCallback(async()=>{
  setBusy(true);setError("");setWorkerError("");
  const requests=await Promise.allSettled([
   get<AccountPage>("/api/v1/accounts?limit=5&offset=0",token),
   get<AccountPage>("/api/v1/accounts?limit=1&offset=0&status=authorized",token),
   get<AccountPage>("/api/v1/accounts?limit=1&offset=0&status=error",token),
   get<AccountPage>("/api/v1/accounts?limit=1&offset=0&status=disconnected",token),
   get<MessagePage>("/api/v1/messages?limit=1&offset=0",token),
   get<{summary:Summary}>("/api/v1/tasks/summary",token),
   get<TaskPage>("/api/v1/tasks?limit=5&offset=0",token),
   user.role==="super_admin"?get<Workers>("/api/v1/admin/workers",token):Promise.resolve(null),
  ] as const);
  const labels=["账号列表","已授权账号统计","异常账号统计","断开账号统计","待处理消息","任务统计","最近任务","Worker 状态"];
  const failures=requests.map((r,i)=>r.status==="rejected"?`${labels[i]}：${r.reason instanceof Error?r.reason.message:"加载失败"}`: "").filter(Boolean);
  if(requests[0].status==="fulfilled"){setRecentAccounts(requests[0].value.accounts||[]);setTotal(requests[0].value.total||0)}
  if(requests[1].status==="fulfilled")setAuthorized(requests[1].value.total||0);
  const errorCount=requests[2].status==="fulfilled"?requests[2].value.total:0;
  const disconnectedCount=requests[3].status==="fulfilled"?requests[3].value.total:0;
  if(requests[2].status==="fulfilled"||requests[3].status==="fulfilled")setErrors(errorCount+disconnectedCount);
  if(requests[4].status==="fulfilled")setPendingMessages(requests[4].value.total||0);
  if(requests[5].status==="fulfilled")setTaskSummary(requests[5].value.summary);
  if(requests[6].status==="fulfilled")setRecentTasks(requests[6].value.tasks||[]);
  if(user.role==="super_admin"){
   if(requests[7].status==="fulfilled")setWorkers(requests[7].value);
   else setWorkerError(failures.find(x=>x.startsWith("Worker 状态"))||"Worker 状态暂不可用");
  }
  setError(failures.filter(x=>!x.startsWith("Worker 状态")).join("；"));
  setUpdatedAt(new Date().toLocaleTimeString());
  setBusy(false);
 },[token,user.role]);

 useEffect(()=>{void load()},[load]);

 return <div className="workbench-page">
  <div className="workbench-heading"><div><div className="eyebrow">OVERVIEW</div><h1>工作台</h1><p>展示当前租户真实账号、消息和任务数据。</p></div><div className="workbench-refresh"><span>{updatedAt?`更新于 ${updatedAt}`:"等待首次加载"}</span><button className="secondary" onClick={()=>void load()} disabled={busy}>{busy?"刷新中…":"刷新数据"}</button></div></div>
  {error&&<div className="alert error">{error}</div>}
  <section className="workbench-stats">
   <Stat label="账号总数" value={total} detail="当前租户账号记录"/>
   <Stat label="已授权账号" value={authorized} detail="账号授权状态，不代表实时在线"/>
   <Stat label="异常/断开账号" value={errors} detail="状态为 error 或 disconnected"/>
   <Stat label="待处理消息" value={pendingMessages} detail="尚未标记处理的消息"/>
   <Stat label="执行中任务" value={taskSummary?.processing??"—"} detail={`任务总数 ${taskSummary?.total??"—"}`}/>
   <Stat label="租约超时" value={taskSummary?.expired_leases??"—"} detail="需要 Worker 回收的超时任务"/>
  </section>
  <div className="workbench-columns">
   <section className="panel"><div className="panel-heading"><div><h2>最近账号</h2><p>按账号创建时间排序</p></div><span className="muted">{recentAccounts.length} 条</span></div>
    {recentAccounts.length?<div className="workbench-list">{recentAccounts.map(a=><div className="workbench-row" key={a.id}><span className="workbench-avatar">{(a.display_name||a.phone||"?").slice(0,1).toUpperCase()}</span><div className="workbench-row-main"><strong>{a.display_name||a.phone||"未命名账号"}</strong><small>{a.phone} · {time(a.created_at)}</small></div><span className={`status status-${a.status}`}>{a.status}</span></div>)}</div>:<div className="empty">{busy?"正在加载…":"暂无账号记录"}</div>}
   </section>
   <section className="panel"><div className="panel-heading"><div><h2>最近任务</h2><p>读取统一任务中心真实队列</p></div><span className="muted">{recentTasks.length} 条</span></div>
    {recentTasks.length?<div className="workbench-list">{recentTasks.map(t=><div className="workbench-row" key={t.type+t.id}><span className="workbench-avatar task-avatar">↗</span><div className="workbench-row-main"><strong>{t.title||t.type}</strong><small>{t.account_name||"未关联账号"} · {time(t.created_at)}</small></div><span className={`status status-${t.status}`}>{t.status}</span></div>)}</div>:<div className="empty">{busy?"正在加载…":"暂无任务记录"}</div>}
   </section>
  </div>
  <section className="panel workbench-runtime"><div className="panel-heading"><div><h2>运行状态</h2><p>仅展示后端实际提供的运行状态，不推断账号实时在线情况</p></div></div>
   {user.role==="super_admin"?workers?<div className="worker-grid">{workers.workers.map(w=><div className="worker-card" key={w.instance_id}><div><strong>{w.hostname||w.instance_id}</strong><span className={w.stale?"status status-failed":"status status-joined"}>{w.stale?"心跳超时":w.status}</span></div><small>PID {w.process_id} · 心跳 {time(w.heartbeat_at)}</small></div>)}{workers.workers.length===0&&<p className="empty">暂无 Worker 心跳记录</p>}<div className="worker-summary">Worker 实例 {workers.total} 个 · 心跳超时 {workers.stale} 个 · 超时阈值 {workers.stale_after_seconds} 秒</div></div>:<p className="muted">{workerError||"正在读取 Worker 状态…"}</p>:<p className="muted">当前后端仅向超级管理员开放 Worker 实例监控；租户级 Worker 状态接口尚未提供，因此此处不展示推测状态。</p>}
  </section>
 </div>;
}
