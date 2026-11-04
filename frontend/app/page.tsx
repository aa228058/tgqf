"use client";

import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AppShell, AppUser } from "./AppShell";
import { Workbench } from "./Workbench";
import { AccountsPage } from "./AccountsPage";
import { MessagesPage } from "./MessagesPage";
import { GroupsPage } from "./GroupsPage";
import { RuntimePage } from "./RuntimePage";
import { SettingsPage } from "./SettingsPage";
import { KeywordMonitor } from "./KeywordMonitor";
import { JoinTasksPage } from "./JoinTasksPage";
import { AdminConsole } from "./AdminConsole";

type Task = {
  id: string; group_id?: string; type: string; title: string; status: string;
  account_id: string; account_name: string; attempts: number; scheduled_at: string;
  lease_until?: string; last_error?: string; can_pause: boolean; can_resume: boolean;
  can_retry: boolean; can_cancel: boolean; created_at: string; updated_at: string;
};
type Summary = { total: number; processing: number; expired_leases: number; counts: { type: string; status: string; count: number }[]; generated_at: string };
type TaskResponse = { tasks: Task[]; total: number; limit: number; offset: number };
const API_BASE = (process.env.NEXT_PUBLIC_API_BASE_URL || "").replace(/\/$/, "");
const typeNames: Record<string, string> = { message_reply: "消息回复", keyword_action: "关键词自动处理", profile_update: "账号资料修改", group_join: "自动加群" };
const statusNames: Record<string, string> = { pending: "等待执行", processing: "执行中", sent: "已发送", failed: "失败", paused: "已暂停", completed: "已完成", cancelled: "已取消", queued: "排队中", request_pending: "等待审批", joined: "已加入", already_member: "已在群内", skipped: "已跳过", draft: "草稿" };

async function request<T>(path: string, token: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, { ...init, headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json", ...(init?.headers || {}) }, cache: "no-store" });
  if (!response.ok) {
    let message = `请求失败（${response.status}）`;
    try { const body = await response.json(); message = body.error || message; } catch {}
    throw new Error(message);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

export default function Home() {
  const [token, setToken] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [loginMode, setLoginMode] = useState<"tenant" | "admin">("tenant");
  const [signedIn, setSignedIn] = useState(false);
  const [user, setUser] = useState<AppUser | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [summary, setSummary] = useState<Summary | null>(null);
  const [total, setTotal] = useState(0);
  const [status, setStatus] = useState("");
  const [type, setType] = useState("");
  const [accountID, setAccountID] = useState("");
  const [query, setQuery] = useState("");
  const [offset, setOffset] = useState(0);
  const [selected, setSelected] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const requestSequence = useRef(0);
  const actionLock = useRef(false);
  const loginLock = useRef(false);
  const limit = 30;

  const load = useCallback(async (currentToken: string, quiet = false) => {
    const sequence = ++requestSequence.current;
    if (!quiet) setBusy(true);
    try {
      const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
      if (status) params.set("status", status);
      if (type) params.set("type", type);
      if (accountID.trim()) params.set("account_id", accountID.trim());
      if (query.trim()) params.set("q", query.trim());
      const [taskResult, summaryResult] = await Promise.allSettled([
        request<TaskResponse>(`/api/v1/tasks?${params}`, currentToken),
        request<{ summary: Summary }>("/api/v1/tasks/summary", currentToken),
      ]);
      if (sequence !== requestSequence.current) return;
      const failures: string[] = [];
      if (taskResult.status === "fulfilled") {
        setTasks(taskResult.value.tasks || []);
        setTotal(taskResult.value.total || 0);
      } else failures.push(taskResult.reason instanceof Error ? taskResult.reason.message : "任务列表加载失败");
      if (summaryResult.status === "fulfilled") setSummary(summaryResult.value.summary);
      else failures.push(summaryResult.reason instanceof Error ? summaryResult.reason.message : "任务统计加载失败");
      setError(failures.join("；"));
    } catch (e) {
      if (sequence === requestSequence.current) setError(e instanceof Error ? e.message : "加载任务失败");
    } finally {
      if (sequence === requestSequence.current && !quiet) setBusy(false);
    }
  }, [offset, status, type, accountID, query]);

  useEffect(() => {
    const saved = window.localStorage.getItem("tgqf_access_token");
    if (!saved) return;
    setToken(saved);
    setSignedIn(true);
    void fetch(`${API_BASE}/api/v1/auth/me`, { headers: { Authorization: `Bearer ${saved}` }, cache: "no-store" })
      .then(async response => {
        if (!response.ok) throw new Error("会话已失效");
        setUser(await response.json() as AppUser);
      })
      .catch(() => {
        window.localStorage.removeItem("tgqf_access_token");
        setToken("");
        setSignedIn(false);
        setUser(null);
      });
  }, []);
  useEffect(() => { if (signedIn && token && user?.role !== "super_admin") void load(token); }, [signedIn, token, user?.role, load]);
  useEffect(() => { setSelected([]); }, [offset, status, type, accountID, query]);
  useEffect(() => {
    if (!signedIn || !token || user?.role === "super_admin") return;
    const refresh = () => { if (document.visibilityState === "visible" && !actionLock.current) void load(token, true); };
    const timer = window.setInterval(refresh, 10000);
    document.addEventListener("visibilitychange", refresh);
    return () => {
      requestSequence.current++;
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", refresh);
    };
  }, [signedIn, token, user?.role, load]);

  async function login(event: FormEvent) {
    event.preventDefault(); if (loginLock.current) return; loginLock.current = true; setBusy(true); setError("");
    try {
      const response = await fetch(`${API_BASE}/api/v1/auth/login`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ username, password, login_type: loginMode, device_label: "tg-account-manager-web" }) });
      const body = await response.json();
      if (!response.ok) throw new Error(body.error || "登录失败");
      window.localStorage.setItem("tgqf_access_token", body.access_token);
      setToken(body.access_token); setUser(body.user as AppUser); setSignedIn(true); setPassword("");
    } catch (e) { setError(e instanceof Error ? e.message : "登录失败"); }
    finally { loginLock.current = false; setBusy(false); }
  }
  function logout() {
    requestSequence.current++; actionLock.current = false;
    const currentToken = token;
    window.localStorage.removeItem("tgqf_access_token");
    setToken(""); setUser(null); setSignedIn(false); setTasks([]); setSummary(null);
    if (currentToken) void fetch(`${API_BASE}/api/v1/auth/logout`, { method: "POST", headers: { Authorization: `Bearer ${currentToken}` } }).catch(() => undefined);
  }

  async function act(task: Task, action: "pause" | "resume" | "retry" | "cancel") {
    if (action === "cancel" && !window.confirm("确定取消这条任务吗？")) return;
    if (actionLock.current) return; actionLock.current = true;
    setBusy(true); setNotice(""); setError("");
    try { await request(`/api/v1/tasks/${task.type}/${encodeURIComponent(task.id)}/${action}`, token, { method: "POST" }); setNotice("任务状态已更新"); await load(token, true); }
    catch (e) { setError(e instanceof Error ? e.message : "操作失败"); }
    finally { actionLock.current = false; setBusy(false); }
  }
  const selectedTasks = useMemo(() => tasks.filter(t => selected.includes(t.id)), [tasks, selected]);
  const selectedTypes = [...new Set(selectedTasks.map(t => t.type))];
  async function bulk(action: "pause" | "resume" | "retry" | "cancel") {
    if (selectedTasks.length === 0 || actionLock.current) return;
    if (action === "cancel" && !window.confirm("确定取消选中的 " + selectedTasks.length + " 条任务吗？")) return;
    actionLock.current = true; setBusy(true); setNotice(""); setError("");
    try {
      let succeeded = 0, failed = 0;
      const failures: string[] = [];
      for (const taskType of selectedTypes) {
        const ids = selectedTasks.filter(t => t.type === taskType).map(t => t.id);
        const result = await request<{ succeeded: number; failed: number; results: { id: string; error?: string }[] }>("/api/v1/tasks/bulk/" + taskType + "/" + action, token, { method: "POST", body: JSON.stringify({ ids }) });
        succeeded += result.succeeded; failed += result.failed;
        for (const item of result.results || []) if (item.error) failures.push(taskType + "/" + item.id + ": " + item.error);
      }
      setNotice("批量操作完成：成功 " + succeeded + " 条，失败 " + failed + " 条" + (failures.length ? "；" + failures.slice(0,3).join("；") : ""));
      setSelected([]); await load(token, true);
    } catch (e) { setError(e instanceof Error ? e.message : "批量操作失败"); }
    finally { actionLock.current = false; setBusy(false); }
  }
  const toggle = (id: string) => setSelected(current => current.includes(id) ? current.filter(v => v !== id) : [...current, id]);

  if (!signedIn) return <main className="login-shell"><form className="login-card" onSubmit={login}><div className="brand-mark">TG</div><h1>TG 账号管理工具</h1><p>Telegram 账号、群组、消息与任务统一管理</p><div className="login-mode-switch" role="group" aria-label="选择登录身份"><button type="button" className={loginMode==="tenant"?"active":""} aria-pressed={loginMode==="tenant"} onClick={()=>setLoginMode("tenant")}><strong>租户号登录</strong><small>进入租户业务工作台</small></button><button type="button" className={loginMode==="admin"?"active":""} aria-pressed={loginMode==="admin"} onClick={()=>setLoginMode("admin")}><strong>管理号登录</strong><small>进入系统管理后台</small></button></div><label>用户名<input value={username} onChange={e => setUsername(e.target.value)} autoComplete="username" required /></label><label>密码<input type="password" value={password} onChange={e => setPassword(e.target.value)} autoComplete="current-password" required /></label>{error && <div className="alert error">{error}</div>}<button className="primary full" disabled={busy}>{busy ? "登录中…" : "登录"}</button><small>安全登录 · 租户数据隔离</small></form></main>;
  if (!user) return <main className="login-shell"><div className="login-card">正在验证登录会话…</div></main>;

  return <AppShell user={user} onLogout={logout} workbench={<Workbench token={token} user={user} />} accounts={<AccountsPage token={token} />} messages={<MessagesPage token={token} />} groups={<GroupsPage token={token} canImport={user.role==="tenant_admin"} />} join={<JoinTasksPage token={token} />} monitor={<KeywordMonitor token={token} />} runtime={<RuntimePage token={token} user={user} />} settings={<SettingsPage token={token} user={user} />} adminContent={<AdminConsole token={token} user={user}/>}><main className="shell">
    <header className="topbar"><div><div className="eyebrow">TG 账号管理工具</div><h1>任务中心</h1><p>统一查看和控制账号资料、加群、关键词处理及消息回复任务</p></div><div className="top-actions"><span className="live-dot" role="status" aria-live="polite">{busy?"正在同步任务":"自动刷新 · 10 秒"}</span><button className="secondary" onClick={() => void load(token)} disabled={busy}>{busy?"同步中…":"↻ 刷新"}</button></div></header>
    {error && <div className="alert error">{error}</div>}{notice && <div className="alert success">{notice}</div>}
    <section className="summary-grid">
      <article className="summary-card"><span>任务总数</span><strong>{summary?.total ?? "—"}</strong></article>
      <article className="summary-card"><span>正在执行</span><strong>{summary?.processing ?? "—"}</strong></article>
      <article className="summary-card"><span>租约超时</span><strong className={(summary?.expired_leases || 0) > 0 ? "warning-text" : ""}>{summary?.expired_leases ?? "—"}</strong></article>
      <article className="summary-card"><span>当前列表</span><strong>{tasks.length} <em>/ {total}</em></strong></article>
    </section>
    <section className="panel">
      <div className="panel-heading"><div><h2>执行任务</h2><p>任务状态直接读取各业务队列，不复制或伪造任务状态</p></div><span className="muted">{summary?.generated_at ? `更新于 ${new Date(summary.generated_at).toLocaleTimeString()}` : "等待数据"}</span></div>
      <div className="filters">
        <select value={type} onChange={e => { setType(e.target.value); setOffset(0); }}><option value="">全部任务类型</option>{Object.entries(typeNames).map(([key, value]) => <option key={key} value={key}>{value}</option>)}</select>
        <select value={status} onChange={e => { setStatus(e.target.value); setOffset(0); }}><option value="">全部状态</option>{Object.entries(statusNames).map(([key, value]) => <option key={key} value={key}>{value}</option>)}</select>
        <input value={accountID} onChange={e => { setAccountID(e.target.value); setOffset(0); }} placeholder="按账号 ID 筛选" />
        <input value={query} maxLength={120} onChange={e => { setQuery(e.target.value); setOffset(0); }} placeholder="搜索任务、账号、错误信息或 ID" />
        <button className="secondary" onClick={() => void load(token)} disabled={busy}>筛选</button>
      </div>
      {selected.length > 0 && <div className="bulkbar"><span>已选 {selected.length} 条{selectedTypes.length > 1 ? "（跨 " + selectedTypes.length + " 种任务类型）" : ""}</span><button disabled={busy} onClick={() => void bulk("pause")}>暂停</button><button disabled={busy} onClick={() => void bulk("resume")}>恢复</button><button disabled={!selectedType || busy} onClick={() => void bulk("retry")}>重试</button><button className="danger-text" disabled={!selectedType || busy} onClick={() => void bulk("cancel")}>取消</button><button className="ghost" onClick={() => setSelected([])}>清除选择</button></div>}
      <div className="table-wrap task-table-wrap"><table className="task-table"><thead><tr><th><input type="checkbox" aria-label="选择当前页全部任务" checked={tasks.length > 0 && selected.length === tasks.length} onChange={e => setSelected(e.target.checked ? tasks.map(t => t.id) : [])} /></th><th>任务</th><th>账号</th><th>状态</th><th>尝试</th><th>计划时间</th><th>最近错误</th><th>操作</th></tr></thead><tbody>
        {tasks.map(task => <tr key={task.type + task.id}><td data-label="选择"><input type="checkbox" aria-label={`选择任务 ${task.id}`} checked={selected.includes(task.id)} onChange={() => toggle(task.id)} /></td><td data-label="任务"><strong>{task.title || typeNames[task.type]}</strong><small>{typeNames[task.type] || task.type} · {task.id}</small></td><td data-label="账号">{task.account_name || "—"}<small>{task.account_id}</small></td><td data-label="状态"><span className={`status status-${task.status}`}>{statusNames[task.status] || task.status}</span>{task.lease_until && <small>租约至 {new Date(task.lease_until).toLocaleString()}</small>}</td><td data-label="尝试">{task.attempts}</td><td data-label="计划时间">{task.scheduled_at ? new Date(task.scheduled_at).toLocaleString() : "—"}</td><td data-label="最近错误" className="error-cell">{task.last_error || "—"}</td><td data-label="操作"><div className="row-actions">{task.can_pause && <button onClick={() => void act(task,"pause")} disabled={busy}>暂停</button>}{task.can_resume && <button onClick={() => void act(task,"resume")} disabled={busy}>恢复</button>}{task.can_retry && <button onClick={() => void act(task,"retry")} disabled={busy}>重试</button>}{task.can_cancel && <button className="danger-text" onClick={() => void act(task,"cancel")} disabled={busy}>取消</button>}</div></td></tr>)}
        {tasks.length === 0 && <tr><td colSpan={8} className="empty">{busy ? "正在加载…" : error ? "加载失败，请根据上方提示重试" : "暂无符合条件的任务"}</td></tr>}
      </tbody></table></div>
      <footer className="pagination"><span>共 {total} 条</span><div><button className="secondary" disabled={offset === 0 || busy} onClick={() => setOffset(Math.max(0, offset-limit))}>上一页</button><span>第 {Math.floor(offset/limit)+1} / {Math.max(1,Math.ceil(total/limit))} 页</span><button className="secondary" disabled={offset+limit >= total || busy} onClick={() => setOffset(offset+limit)}>下一页</button></div></footer>
    </section>
    <section className="panel counts-panel"><div className="panel-heading"><div><h2>任务状态分布</h2><p>按真实队列类型和状态汇总</p></div></div><div className="count-list">{(summary?.counts || []).map(item => <div className="count-item" key={item.type+item.status}><span>{typeNames[item.type] || item.type}</span><span className="muted">{statusNames[item.status] || item.status}</span><strong>{item.count}</strong></div>)}{!summary?.counts?.length && <p className="empty">暂无任务统计</p>}</div></section>
    <div className="footer-note">任务控制遵循各队列当前状态和租户授权；Telegram FloodWait 等平台冷却不会因任务重试而绕过。</div>
  </main></AppShell>;
}
