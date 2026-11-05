"use client";

import { apiErrorMessage, readAPIJSON, notifyUser, translateAPIError } from "./api-response";

import { FormEvent, useCallback, useEffect, useRef, useState } from "react";
import { SharedGroupPicker } from "./SharedGroupPicker";

const API_BASE = (process.env.NEXT_PUBLIC_API_BASE_URL || "").replace(/\/$/, "");
type Account = { id: string; phone: string; display_name: string; status: string; has_session: boolean };
type Group = { id: number; title: string; username: string; chat_type: string; member_count: number };
type Link = { id: number; type: string; public_url: string; status: string; private_invite: boolean; group_id?: number };
type TaskAccount = { account_id: string; account_name: string; status: string; application_status: string; attempts: number; last_error: string; updated_at: string; available_at: string };
type Task = {
  id: string; name: string; target_group_id: number; target_link_id: number; status: string; available_at: string;
  authorization_confirmed: boolean; authorization_note: string; authorized_at?: string;
  account_count: number; created_at: string; updated_at: string; accounts?: TaskAccount[];
};
async function api<T>(path: string, token: string, init?: RequestInit): Promise<T> {
  const headers: Record<string,string> = { Authorization: `Bearer ${token}` };
  if (!(init?.body instanceof FormData)) headers["Content-Type"] = "application/json";
  Object.assign(headers, init?.headers || {});
  const response = await fetch(`${API_BASE}${path}`, { ...init, headers, cache: "no-store" });
  if (!response.ok) {
    let message = `请求失败（${response.status}）`;
    message = await apiErrorMessage(response, message);
    throw new Error(message);
  }
  if (["POST","PUT","PATCH","DELETE"].includes((init?.method||"GET").toUpperCase())) notifyUser(path.includes("/bulk")?"批量加群任务已创建为草稿，请检查后手动启动。":path.endsWith("/join-tasks")?"加群任务已创建，请查看任务列表。":"加群任务操作已成功完成。");
  if (response.status === 204) return undefined as T;
  return readAPIJSON<T>(response);
}
const statusName: Record<string,string> = {
  draft:"草稿", queued:"排队中", paused:"已暂停", completed:"已完成",
  failed:"失败", cancelled:"已取消", processing:"执行中", request_pending:"等待审批",
  joined:"已加入", already_member:"已在群内", skipped:"已跳过"
};
const appName: Record<string,string> = {
  not_started:"未申请", request_pending:"等待审批", approved:"已批准",
  declined:"已拒绝", not_required:"无需审批", unknown:"结果未知"
};
const date = (v?:string) => v ? new Date(v).toLocaleString() : "—";

export function JoinTasksPage({ token }: { token: string }) {
  const [tasks,setTasks] = useState<Task[]>([]);
  const [accounts,setAccounts] = useState<Account[]>([]);
  const [groups,setGroups] = useState<Group[]>([]);
  const [links,setLinks] = useState<Link[]>([]);
  const [total,setTotal] = useState(0);
  const [offset,setOffset] = useState(0);
  const [detail,setDetail] = useState<Task|null>(null);
  const [createOpen,setCreateOpen] = useState(false);
  const [name,setName] = useState("");
  const [targetMode,setTargetMode] = useState<"group"|"link"|"invite">("group");
  const [groupID,setGroupID] = useState("");
  const [linkID,setLinkID] = useState("");
  const [invite,setInvite] = useState("");
  const [accountIDs,setAccountIDs] = useState<string[]>([]);
  const [authorized,setAuthorized] = useState(false);
  const [authorizationNote,setAuthorizationNote] = useState("");
  const [busy,setBusy] = useState(false);
  const [loading,setLoading] = useState(false);
  const loadingRef = useRef(false);
  const detailRequest = useRef(0);
  const [error,setError] = useState("");
  const [notice,setNotice] = useState("");

  const load = useCallback(async () => {
    if (loadingRef.current) return;
    loadingRef.current = true;
    setLoading(true);
    setError("");
    try {
      const [taskResult,accountResult,groupResult,linkResult] = await Promise.all([
        api<{tasks:Task[];total:number}>(`/api/v1/join-tasks?limit=50&offset=${offset}`,token),
        api<{accounts:Account[]}>("/api/v1/accounts?limit=100&offset=0&status=authorized",token),
        api<{groups:Group[]}>("/api/v1/groups?limit=100&offset=0&sort=title",token),
        api<{links:Link[]}>("/api/v1/group-links?limit=100&offset=0",token)
      ]);
      setTasks(taskResult.tasks || []);
      setTotal(taskResult.total || 0);
      setAccounts((accountResult.accounts || []).filter(a=>a.status==="authorized" && a.has_session));
      setGroups(groupResult.groups || []);
      setLinks(linkResult.links || []);
    } catch (e) {
      setError(e instanceof Error ? translateAPIError(e.message) : "自动加群数据加载失败");
    } finally {
      loadingRef.current = false;
      setLoading(false);
    }
  },[token,offset]);

  useEffect(()=>{void load();const timer=window.setInterval(()=>{if(document.visibilityState==="visible")void load()},10000);return()=>window.clearInterval(timer)},[load]);
  useEffect(()=>{
    if(!detail)return;
    let active=true;
    const refreshDetail=async()=>{if(document.visibilityState!=="visible")return;try{const result=await api<{task:Task}>(`/api/v1/join-tasks/${detail.id}`,token);if(active)setDetail(result.task)}catch(e){if(active)setError(e instanceof Error ? translateAPIError(e.message) :"任务详情刷新失败")}};
    const timer=window.setInterval(()=>void refreshDetail(),10000);
    return()=>{active=false;window.clearInterval(timer)};
  },[detail?.id,token]);

  function openCreate() {
    setName("");setTargetMode("group");setGroupID("");setLinkID("");setInvite("");
    setAccountIDs([]);setAuthorized(false);setAuthorizationNote("");setCreateOpen(true);
  }
  function toggleAccount(id:string) {
    setAccountIDs(old=>old.includes(id)?old.filter(x=>x!==id):old.length>=20?old:[...old,id]);
  }
  async function createTask(e:FormEvent) {
    e.preventDefault();
    if (!authorized || accountIDs.length===0) return;
    setBusy(true);setError("");setNotice("");
    const body:Record<string,unknown> = {
      name, account_ids:accountIDs, authorization_confirmed:true,
      authorization_note:authorizationNote
    };
    if (targetMode==="group") body.target_group_id=Number(groupID);
    if (targetMode==="link") body.target_link_id=Number(linkID);
    if (targetMode==="invite") body.private_invite_link=invite;
    try {
      await api("/api/v1/join-tasks",token,{method:"POST",body:JSON.stringify(body)});
      setCreateOpen(false);setNotice("加群任务已创建。请检查目标和授权记录后启动任务。");await load();
    } catch(e) {
      setError(e instanceof Error ? translateAPIError(e.message) :"创建加群任务失败");
    } finally {setBusy(false)}
  }
  async function action(task:Task,op:"start"|"pause"|"resume"|"cancel"|"retry") {
    const labels:Record<string,string>={start:"启动",pause:"暂停",resume:"恢复",cancel:"取消",retry:"重试"};
    if(op==="cancel"&&!window.confirm(`确定取消任务「${task.name}」？`))return;
    setBusy(true);setError("");setNotice("");
    try {
      await api(`/api/v1/join-tasks/${task.id}/${op}`,token,{method:"POST"});
      setNotice(`任务已${labels[op]}`);await load();
      if(detail?.id===task.id){const result=await api<{task:Task}>(`/api/v1/join-tasks/${task.id}`,token);setDetail(result.task)}
    } catch(e) {setError(e instanceof Error ? translateAPIError(e.message) :"任务操作失败")}
    finally {setBusy(false)}
  }
  async function openDetail(task:Task) {
    const sequence=++detailRequest.current;
    setDetail(task);
    try {const result=await api<{task:Task}>(`/api/v1/join-tasks/${task.id}`,token);if(sequence===detailRequest.current)setDetail(result.task)}
    catch(e){if(sequence===detailRequest.current)setError(e instanceof Error ? translateAPIError(e.message) :"任务详情加载失败")}
  }
  const targetLabel=(task:Task)=>{
    if(task.target_group_id){const g=groups.find(x=>x.id===task.target_group_id);return g?g.title:`公共群组 #${task.target_group_id}`}
    if(task.target_link_id){const l=links.find(x=>x.id===task.target_link_id);return l?(l.public_url||"私密邀请链接"):`群链接 #${task.target_link_id}`}
    return "私密邀请链接";
  };
  return <section className="join-page">
    <header className="join-heading"><div><div className="eyebrow">GROUP JOIN AUTOMATION</div><h1>自动加群</h1><p>从全系统共享群组库或本租户发现的邀请链接创建任务；账号执行结果逐个记录。</p></div><div className="join-heading-actions"><button className="secondary" onClick={()=>void load()} disabled={busy||loading}>{loading?"同步中…":"刷新"}</button><button className="primary" onClick={openCreate}>新建加群任务</button></div></header>
    {error&&<div className="alert error">{error}</div>}{notice&&<div className="alert success">{notice}</div>}
    <div className="join-notice"><strong>执行边界</strong><span>每个任务都必须明确确认授权。Telegram FloodWait 会按服务器要求冷却；不通过切换账号或代理绕过限制。审批请求会保持待审批并自动同步可确认的结果。</span></div>
    <section className="panel">
      <div className="panel-heading"><div><h2>加群任务</h2><p>任务总数 {total} · {loading?"正在同步…":"每 10 秒自动同步"}</p></div></div>
      <div className="join-task-list">{tasks.map(task=><article className="join-task-card" key={task.id}>
        <button className="join-task-main" onClick={()=>void openDetail(task)}><span className="join-task-title">{task.name}</span><span className={`status status-${task.status}`}>{statusName[task.status]||task.status}</span><span className="join-task-target">{targetLabel(task)}</span><small>{task.account_count} 个账号 · 创建于 {date(task.created_at)}</small>{task.status==="queued"&&task.available_at&&new Date(task.available_at).getTime()>Date.now()&&<small>任务预计下次可执行：{date(task.available_at)}</small>}</button>
        <div className="join-task-actions">
          {task.status==="draft"&&<button disabled={busy} className="primary" onClick={()=>void action(task,"start")}>启动</button>}
          {task.status==="queued"&&<button disabled={busy} className="secondary" onClick={()=>void action(task,"pause")}>暂停</button>}
          {task.status==="paused"&&<button disabled={busy} className="secondary" onClick={()=>void action(task,"resume")}>恢复</button>}
          {task.status==="failed"&&<button disabled={busy} className="primary" onClick={()=>void action(task,"retry")}>重试失败账号</button>}
          {["draft","queued","paused"].includes(task.status)&&<button disabled={busy} className="danger-text" onClick={()=>void action(task,"cancel")}>取消</button>}
        </div>
      </article>)}{!tasks.length&&<div className="empty">{loading?"正在加载任务…":error?"加载失败，请检查提示后重试":"暂无加群任务"}</div>}</div>
      <footer className="pagination"><span>共 {total} 条</span><div><button className="secondary" disabled={!offset||busy} onClick={()=>setOffset(Math.max(0,offset-50))}>上一页</button><span>第 {Math.floor(offset/50)+1} / {Math.max(1,Math.ceil(total/50))} 页</span><button className="secondary" disabled={offset+50>=total||busy} onClick={()=>setOffset(offset+50)}>下一页</button></div></footer>
    </section>
    {createOpen&&<div className="modal-backdrop"><form className="account-modal join-editor" onSubmit={createTask}>
      <div className="modal-heading"><div><h2>新建加群任务</h2><p>创建时记录授权，任务创建后仍需手动启动。</p></div><button type="button" className="ghost" onClick={()=>setCreateOpen(false)}>关闭</button></div>
      <label>任务名称<input required maxLength={120} value={name} onChange={e=>setName(e.target.value)} placeholder="例如：行业群组加入任务"/></label>
      <label>目标来源<select value={targetMode} onChange={e=>{setTargetMode(e.target.value as typeof targetMode);setGroupID("");setLinkID("");setInvite("")}}><option value="group">共享群组库（公开用户名）</option><option value="link">本租户发现的群链接</option><option value="invite">手动私密邀请链接</option></select></label>
      {targetMode==="group"&&<div className="join-group-picker-field"><label>选择共享群组</label><SharedGroupPicker token={token} value={groupID} onChange={id=>setGroupID(id)} disabled={busy}/><small>支持按名称、username、群组类型搜索并分页；仅显示可通过公开 username 访问的目标。</small></div>}
      {targetMode==="link"&&<label>选择本租户发现的链接<select required value={linkID} onChange={e=>setLinkID(e.target.value)}><option value="">选择链接</option>{links.filter(l=>l.status!=="invalid").map(l=><option key={l.id} value={l.id}>{l.type==="invite"?"私密邀请":"公开链接"} · {l.public_url||`邀请 #${l.id}`} · {l.status}</option>)}</select><small>仅允许使用本租户采集来源关联的链接。</small></label>}
      {targetMode==="invite"&&<label>私密邀请链接<input required value={invite} onChange={e=>setInvite(e.target.value)} placeholder="https://t.me/+…"/><small>邀请 hash 仅以加密形式保存，不会明文写入任务表。</small></label>}
      <fieldset className="join-account-picker"><legend>分配账号（最多 20 个）</legend><div className="join-account-list">{accounts.map(account=><label key={account.id}><input type="checkbox" checked={accountIDs.includes(account.id)} onChange={()=>toggleAccount(account.id)}/><span>{account.display_name||account.phone||account.id}<small>{account.phone} · {account.id.slice(0,8)}…</small></span></label>)}</div>{!accounts.length&&<small>当前没有已授权且存在 Session 的账号。</small>}<small>已选 {accountIDs.length}/20</small></fieldset>
      <label>授权说明（可选）<textarea rows={2} maxLength={500} value={authorizationNote} onChange={e=>setAuthorizationNote(e.target.value)} placeholder="记录本次加入的授权来源或业务目的"/></label>
      <label className="join-authorization"><input type="checkbox" checked={authorized} onChange={e=>setAuthorized(e.target.checked)}/><span>我确认已获得加入目标群组所需的授权，并理解 Telegram 平台限制及 FloodWait 冷却要求。</span></label>
      <button className="primary" disabled={busy||!authorized||!accountIDs.length||(targetMode==="group"&&!groupID)||(targetMode==="link"&&!linkID)||(targetMode==="invite"&&!invite.trim())}>创建任务（创建后需手动启动）</button>
    </form></div>}
    {detail&&<div className="modal-backdrop" onClick={()=>{detailRequest.current++;setDetail(null)}}><section className="account-modal join-detail" onClick={e=>e.stopPropagation()}><div className="modal-heading"><div><h2>{detail.name}</h2><p>{targetLabel(detail)} · {statusName[detail.status]||detail.status}</p></div><button className="ghost" onClick={()=>{detailRequest.current++;setDetail(null)}}>关闭</button></div><div className="join-detail-meta"><span>授权确认：{detail.authorization_confirmed?"已确认":"未确认"}</span><span>授权时间：{date(detail.authorized_at)}</span><span>账号数：{detail.account_count}</span><span>任务下次可执行：{date(detail.available_at)}</span></div>{detail.authorization_note&&<p className="join-auth-note">授权说明：{detail.authorization_note}</p>}<h3>账号执行结果</h3><div className="join-account-results">{(detail.accounts||[]).map(account=><article key={account.account_id}><div><strong>{account.account_name||account.account_id}</strong><small>{account.account_id}</small></div><span className={`status status-${account.status}`}>{statusName[account.status]||account.status}</span><span>{appName[account.application_status]||account.application_status}</span><span>尝试 {account.attempts}</span>{account.status==="queued"&&<small>下次可执行：{date(account.available_at)}</small>}{account.last_error&&<small className="join-error">{translateAPIError(account.last_error)}</small>}</article>)}{!detail.accounts?.length&&<div className="empty">暂无账号执行结果</div>}</div></section></div>}
  </section>;
}
