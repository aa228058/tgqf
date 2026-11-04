"use client";

import { readAPIJSON } from "./api-response";
import {useCallback,useEffect,useRef,useState} from "react";

const API=(process.env.NEXT_PUBLIC_API_BASE_URL||"").replace(/\/$/,"");
type Alert={id:number;key:string;severity:string;status:string;title:string;opened_at:string;last_seen_at:string};
type WorkerInstance={instance_id:string;hostname:string;process_id:number;status:string;started_at:string;heartbeat_at:string;stale:boolean};
type Health={checked_at:string;redis:{status:string;ping_latency_ms:number};queues:{incoming_stream_length:number;incoming_pending:number;message_outbox_pending:number};host:{cpu_percent:number;goroutines:number;heap_alloc_bytes:number;heap_in_use_bytes:number;sys_bytes:number;disk_total_bytes:number;disk_free_bytes:number;disk_used_percent:number};database:{status:string;open_connections:number;in_use_connections:number;idle_connections:number;max_open_connections:number;ping_latency_ms:number};workers:{total:number;running:number;stale:number;stopped:number;instances:WorkerInstance[]};accounts:{total:number;authorized:number;disconnected:number;error:number;pending:number};proxies:{total:number;failed_checks_15m:number;successful_checks_15m:number}};
const bytes=(n:number)=>n>=1024**3?(n/1024**3).toFixed(1)+" GB":n>=1024**2?(n/1024**2).toFixed(0)+" MB":n+" B";
function Metric({label,value,detail,status}:{label:string;value:string|number;detail?:string;status?:"ok"|"warn"|"bad"}){return <article className="monitor-metric"><small>{label}</small><strong>{value}</strong>{detail&&<span>{detail}</span>}{status&&<i className={"monitor-status "+status}>{status==="ok"?"正常":status==="warn"?"关注":"异常"}</i>}</article>}
export function AdminSystemHealthPage({token}:{token:string}){
 const [health,setHealth]=useState<Health|null>(null),[alerts,setAlerts]=useState<Alert[]>([]),[status,setStatus]=useState("loading"),[error,setError]=useState(""),[updated,setUpdated]=useState("");
 const requestSequence=useRef(0);
 const load=useCallback(async()=>{
  const sequence=++requestSequence.current;
  try{
   const response=await fetch(API+"/api/v1/admin/system-health",{headers:{Authorization:"Bearer "+token},cache:"no-store"});
   const result=await readAPIJSON<{health:Health;status:string}>(response);
   if(!response.ok)throw new Error(result.error||"监控数据获取失败");
   if(sequence!==requestSequence.current)return;
   setHealth(result.health);setStatus(result.status);setUpdated(new Date().toLocaleTimeString());setError("");
   const alertResponse=await fetch(API+"/api/v1/admin/system-alerts",{headers:{Authorization:"Bearer "+token},cache:"no-store"});
   if(sequence!==requestSequence.current)return;
   if(alertResponse.ok){const alertResult=await readAPIJSON<{alerts:Alert[]}>(alertResponse);if(sequence===requestSequence.current)setAlerts(alertResult.alerts||[])}
   else setError("运行指标已更新，但告警列表暂时不可用");
  }catch(e){if(sequence===requestSequence.current)setError(e instanceof Error?e.message:"监控数据获取失败")}
 },[token]);
 useEffect(()=>{
  void load();
  const refresh=()=>{if(document.visibilityState==="visible")void load()};
  const timer=window.setInterval(refresh,15000);
  document.addEventListener("visibilitychange",refresh);
  return()=>{requestSequence.current++;window.clearInterval(timer);document.removeEventListener("visibilitychange",refresh)};
 },[load]);
 if(!health)return <section className="panel"><h2>系统运行监控</h2>{error?<p className="alert error">{error}</p>:<p>正在读取运行指标…</p>}<button className="secondary" onClick={()=>void load()}>立即刷新</button></section>;
 return <section className="panel monitor-panel">
  <div className="settings-section-heading"><div><h2>系统运行监控</h2><p>每 15 秒自动刷新 · 最近更新 {updated}</p></div><button className="secondary" onClick={()=>void load()}>立即刷新</button></div>
  {error&&<div className="alert error">{error}</div>}
  <div className="monitor-grid">
   <Metric label="整体状态" value={status==="healthy"?"运行正常":"存在告警"} detail={"采样时间 "+new Date(health.checked_at).toLocaleString()} status={status==="healthy"?"ok":"warn"}/>
   <Metric label="PostgreSQL" value={health.database.status==="ok"?"连接正常":"连接异常"} detail={health.database.ping_latency_ms+" ms · "+health.database.in_use_connections+"/"+health.database.max_open_connections+" 活跃连接"} status={health.database.status==="ok"?"ok":"bad"}/>
   <Metric label="Redis" value={health.redis.status==="ok"?"连接正常":"连接异常"} detail={health.redis.ping_latency_ms+" ms · 入站 Stream "+health.queues.incoming_stream_length+" 条"} status={health.redis.status==="ok"?"ok":"bad"}/>
   <Metric label="Worker" value={health.workers.running+" 运行中"} detail={health.workers.stale+" 心跳超时 · "+health.workers.stopped+" 已停止"} status={health.workers.stale?"bad":"ok"}/>
   <Metric label="磁盘使用率" value={health.host.disk_total_bytes?health.host.disk_used_percent.toFixed(1)+"%":"不可用"} detail={health.host.disk_total_bytes?bytes(health.host.disk_free_bytes)+" 可用 / "+bytes(health.host.disk_total_bytes):"当前运行环境未提供磁盘指标"} status={health.host.disk_used_percent>=90?"bad":health.host.disk_used_percent>=85?"warn":"ok"}/>
   <Metric label="Telegram 账号" value={health.accounts.authorized+" / "+health.accounts.total+" 已授权"} detail={health.accounts.disconnected+" 断开 · "+health.accounts.pending+" 待授权 · "+health.accounts.error+" 错误"} status={health.accounts.error?"warn":"ok"}/>
   <Metric label="代理连接检查" value={health.proxies.total+" 条代理"} detail={"近 15 分钟成功 "+health.proxies.successful_checks_15m+" 次 · 失败 "+health.proxies.failed_checks_15m+" 次"} status={health.proxies.failed_checks_15m?"warn":"ok"}/>
   <Metric label="Go Heap" value={bytes(health.host.heap_alloc_bytes)} detail={"使用中 "+bytes(health.host.heap_in_use_bytes)+" · 系统分配 "+bytes(health.host.sys_bytes)}/>
   <Metric label="CPU 使用率" value={health.host.cpu_percent.toFixed(1)+"%"} detail="基于 /proc/stat 的采样值"/>
   <Metric label="队列积压" value={health.queues.incoming_pending+" pending"} detail={"消息 Outbox "+health.queues.message_outbox_pending+" 条待发布 · Stream 总量 "+health.queues.incoming_stream_length}/>
   <Metric label="Goroutines" value={health.host.goroutines} detail="API 进程当前协程数"/>
  </div>
  <div className="monitor-alerts"><div className="settings-section-heading"><div><h3>Worker 实例明细</h3><p>按心跳新鲜度排序；45 秒未更新的运行实例标记为超时。</p></div><strong>{health.workers.instances?.length||0} 个实例</strong></div>{!health.workers.instances?.length?<p className="monitor-footnote">暂无 Worker 心跳记录。</p>:<div className="monitor-alert-list">{health.workers.instances.map(worker=><article key={worker.instance_id} className="monitor-alert-row"><div><strong>{worker.hostname||"未知主机"} · PID {worker.process_id}</strong><small>{worker.instance_id}</small><small>启动 {new Date(worker.started_at).toLocaleString()} · 最近心跳 {new Date(worker.heartbeat_at).toLocaleString()}</small></div><span className={"monitor-status "+(worker.stale?"bad":worker.status==="running"?"ok":"warn")}>{worker.stale?"心跳超时":worker.status==="running"?"运行中":"已停止"}</span></article>)}</div>}</div>
  <div className="monitor-alerts"><div className="settings-section-heading"><div><h3>告警中心</h3><p>状态变化时生成一条活动告警；指标恢复后自动标记已恢复，重复采样不会重复创建告警。</p></div><strong>{alerts.filter(item=>item.status==="active").length} 条活动告警</strong></div>{alerts.length===0?<p className="monitor-footnote">目前没有告警记录。</p>:<div className="monitor-alert-list">{alerts.map(alert=><article key={alert.id} className="monitor-alert-row"><div><strong>{alert.title}</strong><small>{alert.key} · {new Date(alert.opened_at).toLocaleString()}</small></div><span className={"monitor-status "+(alert.status==="active"?"bad":"ok")}>{alert.status==="active"?"处理中":"已恢复"}</span></article>)}</div>}</div>
  <p className="monitor-footnote">指标范围：API 进程 CPU/内存、容器可见磁盘、PostgreSQL 连接池、Redis、Worker 心跳、账号状态、代理检查历史和消息队列积压。邮件、短信、Telegram 主动推送通知暂未配置；告警目前保存在系统告警中心。</p>
 </section>;
}
