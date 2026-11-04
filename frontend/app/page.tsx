"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { AppShell, AppUser } from "./AppShell";
import { Workbench } from "./Workbench";

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
  const [signedIn, setSignedIn] = useState(false);
  const [user, setUser] = useState<AppUser | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [summary, setSummary] = useState<Summary | null>(null);
  const [total, setTotal] = useState(0);
  const [status, setStatus] = useState("");
  const [type, setType] = useState("");
  const [accountID, setAccountID] = useState("");
  const [offset, setOffset] = useState(0);
  const [selected, setSelected] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const limit = 30;

  const load = useCallback(async (currentToken: string, quiet = false) => {
    if (!quiet) setBusy(true);
    try {
      const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
      if (status) params.set("status", status);
      if (type) params.set("type", type);
      if (accountID.trim()) params.set("account_id", accountID.trim());
      const [taskData, summaryData] = await Promise.all([
        request<TaskResponse>(`/api/v1/tasks?${params}`, currentToken),
        request<{ summary: Summary }>("/api/v1/tasks/summary", currentToken),
      ]);
      setTasks(taskData.tasks || []); setTotal(taskData.total || 0); setSummary(summaryData.summary);
      setSelected([]); setError("");
    } catch (e) { setError(e instanceof Error ? e.message : "加载任务失败"); }
    finally { if (!quiet) setBusy(false); }
  }, [offset, status, type, accountID]);

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
  useEffect(() => { if (signedIn && token) void load(token); }, [signedIn, token, load]);
  useEffect(() => {
    if (!signedIn || !token) return;
    const timer = window.setInterval(() => { void load(token, true); }, 10000);
    return () => window.clearInterval(timer);
  }, [signedIn, token, load]);

  async function login(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError("");
    try {
      const response = await fetch(`${API_BASE}/api/v1/auth/login`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ username, password, device_label: "web-task-center" }) });
      const body = await response.json();
      if (!response.ok) throw new Error(body.error || "登录失败");
      window.localStorage.setItem("tgqf_access_token", body.access_token);
      setToken(body.access_token); setUser(body.user as AppUser); setSignedIn(true); setPassword("");
    } catch (e) { setError(e instanceof Error ? e.message : "登录失败"); }
    finally { setBusy(false); }
  }
  function logout() {
    const currentToken = token;
    window.localStorage.removeItem("tgqf_access_token");
    setToken(""); setUser(null); setSignedIn(false); setTasks([]); setSummary(null);
    if (currentToken) void fetch(`${API_BASE}/api/v1/auth/logout`, { method: "POST", headers: { Authorization: `Bearer ${currentToken}` } }).catch(() => undefined);
  }

  async function act(task: Task, action: "pause" | "resume" | "retry" | "cancel") {
    if (action === "cancel" && !window.confirm("确定取消这条任务吗？")) return;
    setBusy(true); setNotice(""); setError("");
    try { await request(`/api/v1/tasks/${task.type}/${encodeURIComponent(task.id)}/${action}`, token, { method: "POST" }); setNotice("任务状态已更新"); await load(token, true); }
    catch (e) { setError(e instanceof Error ? e.message : "操作失败"); }
    finally { setBusy(false); }
  }
  const selectedTasks = useMemo(() => tasks.filter(t => selected.includes(t.id)), [tasks, selected]);
  const selectedType = selectedTasks.length && selectedTasks.every(t => t.type === selectedTasks[0].type) ? selectedTasks[0].type : "";
  async function bulk(action: "pause" | "resume" | "retry" | "cancel") {
    if (!selectedType || selectedTasks.length === 0) return;
    if (action === "cancel" && !window.confirm(`确定取消选中的 ${selectedTasks.length} 条任务吗？`)) return;
    setBusy(true); setNotice(""); setError("");
    try {
      const result = await request<{ succeeded: number; failed: number; results: { id: string; error?: string }[] }>(`/api/v1/tasks/bulk/${selectedType}/${action}`, token, { method: "POST", body: JSON.stringify({ ids: selectedTasks.map(t => t.id) }) });
      setNotice(`批量操作完成：成功 ${result.succeeded} 条，失败 ${result.failed} 条`);
      await load(token, true);
    } catch (e) { setError(e instanceof Error ? e.message : "批量操作失败"); }
    finally { setBusy(false); }
  }
  const toggle = (id: string) => setSelected(current => current.includes(id) ? current.filter(v => v !== id) : [...current, id]);

  if (!signedIn) return <main className="login-shell"><form className="login-card" onSubmit={login}><div className="brand-mark">TG</div><h1>TG Manager</h1><p>管理控制台登录</p><label>用户名<input value={username} onChange={e => setUsername(e.target.value)} autoComplete="username" required /></label><label>密码<input type="password" value={password} onChange={e => setPassword(e.target.value)} autoComplete="current-password" required /></label>{error && <div className="alert error">{error}</div>}<button className="primary full" disabled={busy}>{busy ? "登录中…" : "登录"}</button><small>API 地址：{API_BASE || "当前域名"}（可通过 NEXT_PUBLIC_API_BASE_URL 配置）</small></form></main>;
  if (!user) return <main className="login-shell"><div className="login-card">正在验证登录会话…</div></main>;

  return <AppShell user={user} onLogout={logout} workbench={<Workbench token={token} user={user} />}><main className="shell">
    <header className="topbar"><div><div className="eyebrow">TG MANAGER</div><h1>任务中心</h1><p>统一查看和控制账号资料、加群、关键词处理及消息回复任务</p></div><div className="top-actions"><span className="live-dot">实时刷新 · 10 秒</span><button className="secondary" onClick={() => void load(token)} disabled={busy}>刷新</button></div></header>
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
        <button className="secondary" onClick={() => void load(token)} disabled={busy}>筛选</button>
      </div>
      {selected.length > 0 && <div className="bulkbar"><span>已选 {selected.length} 条{!selectedType ? "（批量操作要求同一任务类型）" : ""}</span><button disabled={!selectedType || busy} onClick={() => void bulk("pause")}>暂停</button><button disabled={!selectedType || busy} onClick={() => void bulk("resume")}>恢复</button><button disabled={!selectedType || busy} onClick={() => void bulk("retry")}>重试</button><button className="danger-text" disabled={!selectedType || busy} onClick={() => void bulk("cancel")}>取消</button><button className="ghost" onClick={() => setSelected([])}>清除选择</button></div>}
      <div className="table-wrap"><table><thead><tr><th><input type="checkbox" aria-label="选择当前页全部任务" checked={tasks.length > 0 && selected.length === tasks.length} onChange={e => setSelected(e.target.checked ? tasks.map(t => t.id) : [])} /></th><th>任务</th><th>账号</th><th>状态</th><th>尝试</th><th>计划时间</th><th>最近错误</th><th>操作</th></tr></thead><tbody>
        {tasks.map(task => <tr key={task.type + task.id}><td><input type="checkbox" aria-label={`选择任务 ${task.id}`} checked={selected.includes(task.id)} onChange={() => toggle(task.id)} /></td><td><strong>{task.title || typeNames[task.type]}</strong><small>{typeNames[task.type] || task.type} · {task.id}</small></td><td>{task.account_name || "—"}<small>{task.account_id}</small></td><td><span className={`status status-${task.status}`}>{statusNames[task.status] || task.status}</span>{task.lease_until && <small>租约至 {new Date(task.lease_until).toLocaleString()}</small>}</td><td>{task.attempts}</td><td>{task.scheduled_at ? new Date(task.scheduled_at).toLocaleString() : "—"}</td><td className="error-cell">{task.last_error || "—"}</td><td><div className="row-actions">{task.can_pause && <button onClick={() => void act(task,"pause")} disabled={busy}>暂停</button>}{task.can_resume && <button onClick={() => void act(task,"resume")} disabled={busy}>恢复</button>}{task.can_retry && <button onClick={() => void act(task,"retry")} disabled={busy}>重试</button>}{task.can_cancel && <button className="danger-text" onClick={() => void act(task,"cancel")} disabled={busy}>取消</button>}</div></td></tr>)}
        {tasks.length === 0 && <tr><td colSpan={8} className="empty">{busy ? "正在加载…" : "暂无符合条件的任务"}</td></tr>}
      </tbody></table></div>
      <footer className="pagination"><span>共 {total} 条</span><div><button className="secondary" disabled={offset === 0 || busy} onClick={() => setOffset(Math.max(0, offset-limit))}>上一页</button><span>第 {Math.floor(offset/limit)+1} / {Math.max(1,Math.ceil(total/limit))} 页</span><button className="secondary" disabled={offset+limit >= total || busy} onClick={() => setOffset(offset+limit)}>下一页</button></div></footer>
    </section>
    <section className="panel counts-panel"><div className="panel-heading"><div><h2>任务状态分布</h2><p>按真实队列类型和状态汇总</p></div></div><div className="count-list">{(summary?.counts || []).map(item => <div className="count-item" key={item.type+item.status}><span>{typeNames[item.type] || item.type}</span><span className="muted">{statusNames[item.status] || item.status}</span><strong>{item.count}</strong></div>)}{!summary?.counts?.length && <p className="empty">暂无任务统计</p>}</div></section>
    <div className="footer-note">任务控制遵循各队列当前状态和租户授权；Telegram FloodWait 等平台冷却不会因任务重试而绕过。</div>
  </main></AppShell>;
}
