"use client";

import { ReactNode, useEffect, useRef, useState } from "react";

export type AppUser = { id:string; tenant_id?:string; username:string; role:string };
type Key = "workbench"|"accounts"|"messages"|"groups"|"join"|"monitor"|"runtime"|"tasks"|"settings"|"admin";
const items:{key:Key;label:string;icon:string;description:string}[] = [
  {key:"workbench",label:"工作台",icon:"⌂",description:"工作台只展示真实账号、消息和 Worker 数据。"},
  {key:"accounts",label:"账号管理",icon:"◎",description:"账号列表、登录、Session、代理、分组和设备资料。"},
  {key:"messages",label:"消息管理",icon:"▤",description:"实时消息收件箱、聊天上下文和人工回复。"},
  {key:"groups",label:"群组管理",icon:"◫",description:"全系统共享群组库、来源链接和采集设置。"},
  {key:"join",label:"自动加群",icon:"⇥",description:"共享群组目标、账号分配、加群任务及审批状态。"},
  {key:"monitor",label:"监控回复",icon:"⌕",description:"关键词规则、命中记录、自动回复和动作任务。"},
  {key:"runtime",label:"运行状态与日志",icon:"◷",description:"API、Worker、账号连接状态和代理检测记录。"},
  {key:"tasks",label:"任务中心",icon:"☷",description:"统一查看和控制各业务队列任务。"},
  {key:"settings",label:"系统设置",icon:"⚙",description:"个人资料、密码和租户设置。"},
  {key:"admin",label:"管理后台",icon:"▦",description:"仅超级管理员可访问租户、卡密和系统配置。"}
];

export function AppShell({
  user,onLogout,children,workbench,accounts,messages,groups,join,monitor,runtime,settings,adminContent
}:{
  user:AppUser;onLogout:()=>void;children:ReactNode;
  workbench?:ReactNode;accounts?:ReactNode;messages?:ReactNode;groups?:ReactNode;
  join?:ReactNode;monitor?:ReactNode;runtime?:ReactNode;settings?:ReactNode;adminContent?:ReactNode;
}) {
  const [active,setActive] = useState<Key>("tasks");
  const [moreOpen,setMoreOpen] = useState(false);
  const [theme,setTheme] = useState<"light"|"dark">("light");
  const moreMenuRef = useRef<HTMLDivElement>(null);
  const isAdmin = user.role==="super_admin";
  const visible = items.filter(item=>item.key!=="admin"||isAdmin);
  const section = visible.find(item=>item.key===active)||items.find(item=>item.key==="tasks")!;
  useEffect(()=>{
    const saved=localStorage.getItem("tgqf_theme");
    if(saved==="dark"||saved==="light")setTheme(saved);
  },[]);
  useEffect(()=>{
    document.documentElement.dataset.theme=theme;
    localStorage.setItem("tgqf_theme",theme);
  },[theme]);
  const go=(key:Key)=>{
    if(key==="admin"&&!isAdmin)return;
    setActive(key);setMoreOpen(false);
  };
  const primary:Key[]=["workbench","accounts","messages"];
  const more=visible.filter(item=>!primary.includes(item.key));
  const content =
    active==="tasks" ? children :
    active==="workbench" ? workbench :
    active==="accounts" ? accounts :
    active==="messages" ? messages :
    active==="groups" ? groups :
    active==="join" ? join :
    active==="monitor" ? monitor :
    active==="runtime" ? runtime :
    active==="settings" ? settings :
    active==="admin"&&isAdmin ? adminContent : null;
  return <div className="app-frame">
    <aside className="app-sidebar">
      <div className="app-brand"><div className="app-brand-mark">TG</div><div><strong>TG Manager</strong><small>管理控制台</small></div></div>
      <div className="sidebar-caption">工作空间</div>
      <nav className="sidebar-nav" aria-label="主导航">
        {visible.map(item=><button key={item.key} aria-current={active===item.key?"page":undefined} className={active===item.key?"nav-item active":"nav-item"} onClick={()=>go(item.key)}>
          <span className="nav-icon">{item.icon}</span><span>{item.label}</span>{item.key==="tasks"&&<span className="nav-pill">任务</span>}
        </button>)}
      </nav>
      <div className="sidebar-bottom">
        <div className="account-chip"><span className="avatar">{user.username.slice(0,1).toUpperCase()}</span><span><strong>{user.username}</strong><small>{isAdmin?"超级管理员":"租户管理员"}</small></span></div>
        <button className="sidebar-logout" onClick={onLogout}>退出登录</button>
      </div>
    </aside>
    <div className="app-main">
      <header className="app-topbar"><div className="breadcrumb"><span>TG Manager</span><span className="breadcrumb-separator">/</span><strong>{section.label}</strong></div>
        <div className="topbar-user"><span className="role-label">{isAdmin?"超级管理员":"租户空间"}</span><button className="theme-toggle" onClick={()=>setTheme(value=>value==="light"?"dark":"light")}>{theme==="light"?"◐ 深色":"☼ 浅色"}</button><span className="topbar-username">{user.username}</span><button className="topbar-logout" onClick={onLogout}>退出</button></div>
      </header>
      <div className="app-content">{content||<section className="module-placeholder"><div className="placeholder-icon">{section.icon}</div><div className="eyebrow">TG MANAGER · {section.label.toUpperCase()}</div><h1>{section.label}</h1><p>{section.description}</p><div className="placeholder-note">页面正在接入真实接口；当前不展示演示数据或无效操作。</div></section>}</div>
      <nav className="mobile-bottom-nav" aria-label="手机主导航">
        {primary.map(key=>{const item=items.find(x=>x.key===key)!;return <button key={key} aria-current={active===key?"page":undefined} className={active===key?"mobile-nav-item active":"mobile-nav-item"} onClick={()=>go(key)}><span>{item.icon}</span><small>{item.label}</small></button>})}
        <div className="mobile-more-wrap"><button className={moreOpen||more.some(item=>item.key===active)?"mobile-nav-item active":"mobile-nav-item"} onClick={()=>setMoreOpen(value=>!value)}><span>•••</span><small>更多</small></button>
          {moreOpen&&<div className="mobile-more-menu">{more.map(item=><button key={item.key} role="menuitem" aria-current={active===item.key?"page":undefined} onClick={()=>go(item.key)}>{item.icon}<span>{item.label}</span></button>)}</div>}
        </div>
      </nav>
    </div>
  </div>;
}
