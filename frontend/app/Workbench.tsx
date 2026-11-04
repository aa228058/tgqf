"use client";

import { apiErrorMessage, readAPIJSON } from "./api-response";

import { useCallback, useEffect, useRef, useState } from "react";
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

async function get<T>(path:string,token:string):Promise<T>{
 const response=await fetch(`${API_BASE}${path}`,{headers:{Authorization:`Bearer ${token}`},cache:"no-store"});
 if(!response.ok){let message=`请求失败（${response.status}）`;try{const body=await response.json();message=body.error||message}catch{}throw new Error(message)}
 return readAPIJSON<T>(response);
}
function time(value?:string){if(!value)return "—";const date=new Date(value);return Number.isNaN(date.getTime())?"—":date.toLocaleString()}
const accountStatus:Record<string,string>={authorized:"已授权",error:"异常",disconnected:"已断开",pending:"待授权"};
const taskStatus:Record<string,string>={pending:"等待执行",processing:"执行中",sent:"已发送",failed:"失败",paused:"已暂停",completed:"已完成",queued:"排队中",cancelled:"已取消",request_pending:"等待审批",joined:"已加入",already_member:"已在群内"};
function Stat({label,value,detail}:{label:string;value:string|number;detail:string}){return <article className="workbench-stat"><span>{label}</span><strong>{value}</strong><small>{detail}</small></article>}

export function Workbench({token,user}:{token:string;user:AppUser}){
 const [total,setTotal]=useState(0),[authorized,setAuthorized]=useState(0),[errors,setErrors]=useState(0);
 const [pendingMessages,setPendingMessages]=useState(0),[taskSummary,setTaskSummary]=useState<Summary|null>(null);
 const [recentAccounts,setRecentAccounts]=useState<Account[]>([]),[recentTasks,setRecentTasks]=useState<Task[]>([]);
 const [workers,setWorkers]=useState<Workers|null>(null),[workerError,setWorkerError]=useState("");
 const [error,setError]=useState(""),[busy,setBusy]=useState(false),[hasLoaded,setHasLoaded]=useState(false),[updatedAt,setUpdatedAt]=useState("");
 const requestId=useRef(0);

 const load=useCallback(async()=>{
  const current=++requestId.current;
  setBusy(true);setError("");setWorkerError("");
  try{
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
   if(current!==requestId.current)return;
   const labels=["账号列表","已授权账号统计","异常账号统计","断开账号统计","待处理消息","任务统计","最近任务","Worker 状态"];
   const failures=requests.map((r,i)=>r.status==="rejected"?`${labels[i]}：${r.reason instanceof Error?r.reason.message:"加载失败"}`:"").filter(Boolean);
   if(requests[0].status==="fulfilled"){setRecentAccounts(requests[0].value.accounts||[]);setTotal(requests[0].value.total||0)}
   if(requests[1].status==="fulfilled")setAuthorized(requests[1].value.total||0);
   if(requests[2].status==="fulfilled"||requests[3].status==="fulfilled")setErrors((requests[2].status==="fulfilled"?requests[2].value.total:0)+(requests[3].status==="fulfilled"?requests[3].value.total:0));
   if(requests[4].status==="fulfilled")setPendingMessages(requests[4].value.total||0);
   if(requests[5].status==="fulfilled")setTaskSummary(requests[5].value.summary);
   if(requests[6].status==="fulfilled")setRecentTasks(requests[6].value.tasks||[]);
   if(user.role==="super_admin"){
    if(requests[7].status==="fulfilled")setWorkers(requests[7].value);
    else setWorkerError(failures.find(x=>x.startsWith("Worker 状态"))||"Worker 状态暂不可用");
   }
   setError(failures.filter(x=>!x.startsWith("Worker 状态")).join("；"));
   setUpdatedAt(new Date().toLocaleTimeString());
   setHasLoaded(true);
  }catch(e){
   if(current===requestId.current)setError(e instanceof Error?e.message:"工作台加载失败，请稍后重试");
  }finally{
   if(current===requestId.current)setBusy(false);
  }
 },[token,user.role]);

 useEffect(()=>{void load();const refresh=()=>{if(document.visibilityState==="visible")void load()};const timer=window.setInterval(refresh,30000);document.addEventListener("visibilitychange",refresh);return()=>{requestId.current++;window.clearInterval(timer);document.removeEventListener("visibilitychange",refresh)}},[load]);

 return <div className="workbench-page">
  <div className="workbench-heading"><div><div className="eyebrow">OVERVIEW</div><h1>工作台</h1><p>当前租户账号、消息与任务运行概况</p></div><div className="workbench-refresh"><span className="workbench-update" role="status" aria-live="polite"><i className={busy?"is-refreshing":""}/>{busy?"正在同步数据":updatedAt?`最近更新 ${updatedAt}`:"等待首次加载"}</span><button className="secondary workbench-refresh-button" onClick={()=>void load()} disabled={busy} aria-label="刷新工作台数据">{busy?<><span className="button-spinner" aria-hidden="true"/>同步中</>:"↻ 刷新数据"}</button></div></div>
  {error&&<div className="alert error workbench-alert" role="alert"><span>{error}</span><button className="ghost" onClick={()=>void load()} disabled={busy}>重试</button></div>}
  <section className="workbench-stats" aria-label="关键数据">
   <Stat label="账号总数" value={hasLoaded?total:"—"} detail="当前租户账号记录"/>
   <Stat label="已授权账号" value={hasLoaded?authorized:"—"} detail="授权状态，不代表实时在线"/>
   <Stat label="异常 / 断开账号" value={hasLoaded?errors:"—"} detail="异常与断开账号合计"/>
   <Stat label="待处理消息" value={hasLoaded?pendingMessages:"—"} detail="尚未标记处理的消息"/>
   <Stat label="执行中任务" value={taskSummary?.processing??"—"} detail={taskSummary?`任务总数 ${taskSummary.total}`:"任务统计暂不可用"}/>
   <Stat label="租约超时" value={taskSummary?.expired_leases??"—"} detail="等待 Worker 回收的超时任务"/>
  </section>
  <div className="workbench-columns">
   <section className="panel"><div className="panel-heading"><div><h2>最近账号</h2><p>最近创建的账号记录</p></div><span className="muted">{total} 个账号</span></div>
    {recentAccounts.length?<div className="workbench-list">{recentAccounts.map(a=><div className="workbench-row" key={a.id}><span className="workbench-avatar">{(a.display_name||a.phone||"?").slice(0,1).toUpperCase()}</span><div className="workbench-row-main"><strong>{a.display_name||a.phone||"未命名账号"}</strong><small>{a.phone||"未填写手机号"} · {time(a.created_at)}</small></div><span className={`status status-${a.status}`}>{accountStatus[a.status]||a.status}</span></div>)}</div>:<div className="empty">{busy&&!hasLoaded?"正在读取账号…":hasLoaded?"暂无账号记录":"等待加载账号数据"}</div>}
   </section>
   <section className="panel"><div className="panel-heading"><div><h2>最近任务</h2><p>统一任务队列的最新记录</p></div><span className="muted">{recentTasks.length} 条</span></div>
    {recentTasks.length?<div className="workbench-list">{recentTasks.map(t=><div className="workbench-row" key={t.type+t.id}><span className="workbench-avatar task-avatar">↗</span><div className="workbench-row-main"><strong>{t.title||t.type}</strong><small>{t.account_name||"未关联账号"} · {time(t.created_at)}</small></div><span className={`status status-${t.status}`}>{taskStatus[t.status]||t.status}</span></div>)}</div>:<div className="empty">{busy&&!hasLoaded?"正在读取任务…":hasLoaded?"暂无任务记录":"等待加载任务数据"}</div>}
   </section>
  </div>
  <section className="panel workbench-runtime"><div className="panel-heading"><div><h2>运行状态</h2><p>仅展示后端实际提供的运行状态</p></div></div>
   {user.role==="super_admin"?workers?<div className="worker-grid">{workers.workers.map(w=><div className="worker-card" key={w.instance_id}><div><strong>{w.hostname||w.instance_id}</strong><span className={w.stale?"status status-failed":"status status-joined"}>{w.stale?"心跳超时":w.status}</span></div><small>PID {w.process_id} · 心跳 {time(w.heartbeat_at)}</small></div>)}{workers.workers.length===0&&<p className="empty">暂无 Worker 心跳记录</p>}<div className="worker-summary">Worker 实例 {workers.total} 个 · 心跳超时 {workers.stale} 个 · 超时阈值 {workers.stale_after_seconds} 秒</div></div>:<p className="muted">{workerError||"正在读取 Worker 状态…"}</p>:<p className="muted">当前后端仅向超级管理员开放 Worker 实例监控；租户级 Worker 状态接口尚未提供。</p>}
  </section>
 </div>;
}
