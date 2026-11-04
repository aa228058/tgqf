"use client";
import { ReactNode, useEffect, useState } from "react";
export type AppUser={id:string;tenant_id?:string;username:string;role:string};
type Key="workbench"|"accounts"|"messages"|"groups"|"runtime"|"tasks"|"settings"|"admin";
const items:{key:Key;label:string;icon:string;description:string}[]=[
{key:"workbench",label:"工作台",icon:"⌂",description:"工作台只展示真实账号、消息和 Worker 数据；接口接入前不填充演示统计。"},
{key:"accounts",label:"账号管理",icon:"◎",description:"账号列表、扫码/手机号登录、Session 导入、代理、分组和设备资料将在后续页面接入真实 API。"},
{key:"messages",label:"消息管理",icon:"▤",description:"统一收件箱、聊天上下文、回复和处理状态将在消息页面接入现有接口。"},
{key:"groups",label:"群组管理",icon:"◫",description:"这里将展示全系统共享群组总库；租户可以查看和贡献，不提供手工编辑或删除公共记录的入口。"},
{key:"runtime",label:"运行状态与日志",icon:"◷",description:"API、Worker、账号连接状态和运行日志将在监控接口接入后展示。"},
{key:"tasks",label:"任务中心",icon:"☷",description:"统一任务队列"},
{key:"settings",label:"系统设置",icon:"⚙",description:"个人资料、密码和主题设置将在真实设置能力接入后开放。"},
{key:"admin",label:"管理后台",icon:"▦",description:"仅超级管理员可访问租户、卡密、授权和系统配置。"}];
export function AppShell({user,onLogout,children,workbench,accounts,messages,groups,runtime}:{user:AppUser;onLogout:()=>void;children:ReactNode;workbench?:ReactNode;accounts?:ReactNode;messages?:ReactNode;groups?:ReactNode;runtime?:ReactNode}){
 const [active,setActive]=useState<Key>("tasks"),[moreOpen,setMoreOpen]=useState(false),[theme,setTheme]=useState<"light"|"dark">("light");
 const admin=user.role==="super_admin",visible=items.filter(x=>x.key!=="admin"||admin),section=visible.find(x=>x.key===active)||items[5];
 useEffect(()=>{const v=localStorage.getItem("tgqf_theme");if(v==="dark"||v==="light")setTheme(v)},[]);
 useEffect(()=>{document.documentElement.dataset.theme=theme;localStorage.setItem("tgqf_theme",theme)},[theme]);
 const go=(key:Key)=>{if(key==="admin"&&!admin)return;setActive(key);setMoreOpen(false)};
 const primary:Key[]=["workbench","accounts","messages"],more=visible.filter(x=>!primary.includes(x.key));
 return <div className="app-frame">
  <aside className="app-sidebar"><div className="app-brand"><div className="app-brand-mark">TG</div><div><strong>TG Manager</strong><small>管理控制台</small></div></div>
   <div className="sidebar-caption">工作空间</div><nav className="sidebar-nav" aria-label="主导航">{visible.map(x=><button key={x.key} className={active===x.key?"nav-item active":"nav-item"} onClick={()=>go(x.key)}><span className="nav-icon">{x.icon}</span><span>{x.label}</span>{x.key==="tasks"&&<span className="nav-pill">任务</span>}</button>)}</nav>
   <div className="sidebar-bottom"><div className="account-chip"><span className="avatar">{user.username.slice(0,1).toUpperCase()}</span><span><strong>{user.username}</strong><small>{admin?"超级管理员":"租户管理员"}</small></span></div><button className="sidebar-logout" onClick={onLogout}>退出登录</button></div>
  </aside>
  <div className="app-main"><header className="app-topbar"><div className="breadcrumb"><span>TG Manager</span><span className="breadcrumb-separator">/</span><strong>{section.label}</strong></div><div className="topbar-user"><span className="role-label">{admin?"超级管理员":"租户空间"}</span><button className="theme-toggle" onClick={()=>setTheme(v=>v==="light"?"dark":"light")}>{theme==="light"?"◐ 深色":"☼ 浅色"}</button><span className="topbar-username">{user.username}</span><button className="topbar-logout" onClick={onLogout}>退出</button></div></header>
   <div className="app-content">{active==="tasks"?children:active==="workbench"&&workbench?workbench:active==="accounts"&&accounts?accounts:active==="messages"&&messages?messages:active==="groups"&&groups?groups:active==="runtime"&&runtime?runtime:<section className="module-placeholder"><div className="placeholder-icon">{section.icon}</div><div className="eyebrow">TG MANAGER · {section.label.toUpperCase()}</div><h1>{section.label}</h1><p>{section.description}</p><div className="placeholder-note">页面框架已就位。后续按模块接入真实 API、加载状态、空状态、错误反馈和权限控制。</div></section>}</div>
   <nav className="mobile-bottom-nav" aria-label="手机主导航">{primary.map(key=>{const x=items.find(i=>i.key===key)!;return <button key={key} className={active===key?"mobile-nav-item active":"mobile-nav-item"} onClick={()=>go(key)}><span>{x.icon}</span><small>{x.label}</small></button>})}<div className="mobile-more-wrap"><button className={moreOpen||more.some(x=>x.key===active)?"mobile-nav-item active":"mobile-nav-item"} onClick={()=>setMoreOpen(v=>!v)}><span>•••</span><small>更多</small></button>{moreOpen&&<div className="mobile-more-menu">{more.map(x=><button key={x.key} onClick={()=>go(x.key)}>{x.icon}<span>{x.label}</span></button>)}</div>}</div></nav>
  </div>
 </div>
}
