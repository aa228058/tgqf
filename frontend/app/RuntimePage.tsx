"use client";

import { apiErrorMessage, readAPIJSON } from "./api-response";

import { useCallback,useEffect,useRef,useState } from "react";
const API=(process.env.NEXT_PUBLIC_API_BASE_URL||"").replace(/\/$/,"");
type Account={id:string;phone:string;display_name:string;status:string;last_error:string;updated_at:string};
type Proxy={id:string;name:string;protocol:string;host:string;port:number;is_default:boolean;has_password:boolean};
type Check={id:string;check_type:string;status:string;latency_ms:number;message:string;checked_at:string};
type Worker={instance_id:string;hostname:string;process_id:number;status:string;heartbeat_at:string;started_at:string;stopped_at?:string;stale:boolean};
type Tenant={id:string;name:string;status:string;account_limit:number;expires_at?:string;created_at:string};
type Summary={total:number;processing:number;expired_leases:number;counts:{type:string;status:string;count:number}[]};
async function get<T>(path:string,token:string):Promise<T>{const r=await fetch(API+path,{headers:{Authorization:`Bearer ${token}`},cache:"no-store"});if(!r.ok){let m=`请求失败（${r.status}）`;try{m=(await r.json()).error||m}catch{}throw Error(m)}return readAPIJSON<T>(r)}
const date=(s?:string)=>s?new Date(s).toLocaleString():"—";
function State({value}:{value:string}){return <span className={`status status-${value==="active"||value==="authorized"||value==="ok"||value==="success"||value==="running"?"joined":value==="disabled"||value==="stale"||value==="failed"||value==="error"?"failed":"pending"}`}>{value}</span>}
export function RuntimePage({token,user}:{token:string;user:{role:string}}){
 const admin=user.role==="super_admin";
 const [workers,setWorkers]=useState<Worker[]>([]),[tenants,setTenants]=useState<Tenant[]>([]);
 const [accounts,setAccounts]=useState<Account[]>([]),[proxies,setProxies]=useState<Proxy[]>([]);
 const [checks,setChecks]=useState<Check[]>([]),[selectedProxy,setSelectedProxy]=useState("");
 const [summary,setSummary]=useState<Summary|null>(null),[loading,setLoading]=useState(false),[error,setError]=useState(""),[updated,setUpdated]=useState("");
 const loadSequence=useRef(0),checkSequence=useRef(0);
 const load=useCallback(async()=>{
  const sequence=++loadSequence.current;
  setLoading(true);setError("");
  const failures:string[]=[];
  if(admin){
   const [workerResult,tenantResult]=await Promise.allSettled([
    get<{workers:Worker[]}>("/api/v1/admin/workers?limit=100",token),
    get<{tenants:Tenant[]}>("/api/v1/admin/tenants?limit=100",token)
   ]);
   if(workerResult.status==="fulfilled")setWorkers(workerResult.value.workers||[]);else failures.push("Worker 心跳："+String(workerResult.reason));
   if(tenantResult.status==="fulfilled")setTenants(tenantResult.value.tenants||[]);else failures.push("租户状态："+String(tenantResult.reason));
   setSummary(null);
  }else{
   const [accountResult,proxyResult,summaryResult]=await Promise.allSettled([
    get<{accounts:Account[]}>("/api/v1/accounts?limit=100&offset=0",token),
    get<{proxies:Proxy[]}>("/api/v1/proxies",token),
    get<{summary:Summary}>("/api/v1/tasks/summary",token)
   ]);
   if(accountResult.status==="fulfilled")setAccounts(accountResult.value.accounts||[]);else failures.push("账号状态："+String(accountResult.reason));
   if(proxyResult.status==="fulfilled"){const list=proxyResult.value.proxies||[];setProxies(list);setSelectedProxy(current=>current||list[0]?.id||"")}else failures.push("代理配置："+String(proxyResult.reason));
   if(summaryResult.status==="fulfilled")setSummary(summaryResult.value.summary);else failures.push("任务状态："+String(summaryResult.reason));
  }
  if(sequence!==loadSequence.current)return;
  setError(failures.join("；"));setUpdated(new Date().toLocaleTimeString());setLoading(false);
 },[token,admin]);
 useEffect(()=>{void load();const refresh=()=>{if(document.visibilityState==="visible")void load()};const timer=window.setInterval(refresh,20000);document.addEventListener("visibilitychange",refresh);return()=>{loadSequence.current++;window.clearInterval(timer);document.removeEventListener("visibilitychange",refresh)}},[load]);
 useEffect(()=>{
  if(admin||!selectedProxy){setChecks([]);return}
  let active=true;const sequence=++checkSequence.current;setChecks([]);
  void get<{checks:Check[]}>(`/api/v1/proxies/${selectedProxy}/checks?limit=50`,token).then(r=>{if(active&&sequence===checkSequence.current)setChecks(r.checks||[])}).catch(e=>{if(active&&sequence===checkSequence.current)setError(e instanceof Error?e.message:"代理检测历史加载失败")});
  return()=>{active=false;checkSequence.current++};
 },[admin,selectedProxy,token]);
 const accountCounts=accounts.reduce<Record<string,number>>((acc,a)=>{acc[a.status]=(acc[a.status]||0)+1;return acc},{});
 return <div className="runtime-page">
  <header className="runtime-heading"><div><div className="eyebrow">RUNTIME & HEALTH</div><h1>运行状态与日志</h1><p>{admin?"系统级 Worker 心跳和租户运行状态":"当前租户账号、代理检测与任务运行状态"}</p></div><div className="runtime-refresh"><span>{updated?`更新于 ${updated}`:"尚未加载"}</span><button className="secondary" onClick={()=>void load()} disabled={loading}>{loading?"同步中…":"↻ 刷新状态"}</button></div></header>
  {error&&<div className="alert error">{error}</div>}
  <section className="runtime-stats">
   <article><small>任务总数</small><strong>{summary?.total??"—"}</strong></article>
   <article><small>执行中任务</small><strong>{summary?.processing??"—"}</strong></article>
   <article><small>超时租约</small><strong>{summary?.expired_leases??"—"}</strong></article>
   {admin?<><article><small>Worker 实例</small><strong>{workers.length}</strong></article><article><small>Worker 心跳超时</small><strong>{workers.filter(w=>w.stale).length}</strong></article><article><small>租户数量</small><strong>{tenants.length}</strong></article></>:<><article><small>账号数量</small><strong>{accounts.length}</strong></article><article><small>代理数量</small><strong>{proxies.length}</strong></article><article><small>异常账号</small><strong>{(accountCounts.error||0)+(accountCounts.disconnected||0)}</strong></article></>}
  </section>
  {admin?<div className="runtime-columns">
   <section className="panel"><div className="panel-heading"><div><h2>Worker 心跳</h2><p>心跳阈值由后端定义为 45 秒</p></div></div>{workers.length?<div className="runtime-list">{workers.map(w=><article key={w.instance_id}><div><strong>{w.hostname||w.instance_id}</strong><State value={w.stale?"stale":w.status}/></div><small>实例 {w.instance_id} · PID {w.process_id}</small><small>最近心跳：{date(w.heartbeat_at)}</small><small>启动时间：{date(w.started_at)}</small></article>)}</div>:<div className="empty">{loading?"正在读取 Worker 心跳…":error?"加载失败，请检查提示后重试":"暂无 Worker 心跳记录"}</div>}</section>
   <section className="panel"><div className="panel-heading"><div><h2>租户运行状态</h2><p>租户状态与账号额度</p></div></div>{tenants.length?<div className="runtime-list">{tenants.map(t=><article key={t.id}><div><strong>{t.name||t.id}</strong><State value={t.status}/></div><small>账号额度：{t.account_limit}</small><small>到期时间：{date(t.expires_at)}</small><small>创建时间：{date(t.created_at)}</small></article>)}</div>:<div className="empty">{loading?"正在读取租户…":error?"加载失败，请检查提示后重试":"暂无租户"}</div>}</section>
  </div>:<div className="runtime-columns">
   <section className="panel"><div className="panel-heading"><div><h2>账号运行状态</h2><p>当前租户账号记录和最近错误</p></div></div>{accounts.length?<div className="runtime-list">{accounts.map(a=><article key={a.id}><div><strong>{a.display_name||a.phone||a.id}</strong><State value={a.status}/></div><small>{a.phone} · 更新于 {date(a.updated_at)}</small>{a.last_error&&<small className="runtime-error">{a.last_error}</small>}</article>)}</div>:<div className="empty">{loading?"正在读取账号…":error?"加载失败，请检查提示后重试":"暂无账号记录"}</div>}</section>
   <section className="panel"><div className="panel-heading"><div><h2>代理检测历史</h2><p>查看已保存的 TCP/Telegram 代理检测结果</p></div></div><select className="runtime-proxy-select" value={selectedProxy} onChange={e=>setSelectedProxy(e.target.value)}><option value="">选择代理</option>{proxies.map(p=><option value={p.id} key={p.id}>{p.name} · {p.host}:{p.port}</option>)}</select>{selectedProxy?checks.length?<div className="runtime-list">{checks.map(c=><article key={c.id}><div><strong>{c.check_type}</strong><State value={c.status}/></div><small>{date(c.checked_at)} · {c.latency_ms} ms</small><small>{c.message||"—"}</small></article>)}</div>:<div className="empty">{error?"检测历史加载失败，请查看上方提示":"正在读取检测历史或暂无记录"}</div>:<div className="empty">请选择代理查看检测历史</div>}</section>
  </div>}
  <section className="panel runtime-note"><h2>日志数据说明</h2><p>此页面只展示后端已持久化并开放的运行状态与代理检测记录。当前版本没有通用应用日志查询 API，因此不会伪造 Worker 日志、Telegram 原始日志或实时在线状态；需要完整日志检索时应由后端增加受权限控制的日志查询接口。</p></section>
 </div>;
}
