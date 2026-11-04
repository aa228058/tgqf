"use client";

import { FormEvent, useCallback, useEffect, useRef, useState } from "react";
import QRCode from "qrcode";

const API_BASE=(process.env.NEXT_PUBLIC_API_BASE_URL||"").replace(/\/$/,"");
type Account={id:string;phone:string;display_name:string;group_name:string;tags:string[];status:string;has_session:boolean;has_proxy:boolean;last_error:string;created_at:string;updated_at:string};
type AccountPage={accounts:Account[];total:number;limit:number;offset:number};
type Proxy={id:string;name:string;protocol:string;host:string;port:number;username:string;has_password:boolean;is_default:boolean;last_check_status?:string;last_check_latency_ms?:number;last_check_at?:string};
type ProxyInput={name:string;protocol:string;host:string;port:number;username:string;password:string;clear_password?:boolean};
type QRState={accountId:string;url:string;expiresAt:string};
type LoginMode="qr"|"phone"|"session";
type ProfileJob={id:string;batch_id:string;account_id:string;first_name:string;last_name:string;about:string;generate_username:boolean;has_photo:boolean;applied_username?:string;status:string;attempts:number;next_run_at:string;last_error?:string};
type UsernameSettings={prefix:string;random_length:number;charset:"letters"|"alphanumeric"};
type DeviceProfile={platform:string;device_model:string;system_version:string;app_version:string;lang_code:string;system_lang_code:string;lang_pack:string;package_id:string};

async function api<T>(path:string,token:string,init?:RequestInit):Promise<T>{
 const headers:Record<string,string>={Authorization:`Bearer ${token}`};
 if(!(init?.body instanceof FormData))headers["Content-Type"]="application/json";
 Object.assign(headers,init?.headers||{});
 const response=await fetch(`${API_BASE}${path}`,{...init,headers,cache:"no-store"});
 if(!response.ok){let message=`请求失败（${response.status}）`;try{const body=await response.json();message=body.error||message}catch{}throw new Error(message)}
 if(response.status===204)return undefined as T;
 return response.json() as Promise<T>;
}
const statusLabel:Record<string,string>={authorized:"已授权",pending:"待登录",error:"异常",disconnected:"已断开"};
const statusClass:Record<string,string>={authorized:"status-joined",pending:"status-pending",error:"status-failed",disconnected:"status-paused"};
const fmt=(value?:string)=>value?new Date(value).toLocaleString():"—";

export function AccountsPage({token}:{token:string}){
 const [tab,setTab]=useState<"accounts"|"proxies"|"profile">("accounts");
 const [accounts,setAccounts]=useState<Account[]>([]),[total,setTotal]=useState(0),[proxies,setProxies]=useState<Proxy[]>([]);
 const [query,setQuery]=useState(""),[debouncedQuery,setDebouncedQuery]=useState(""),[status,setStatus]=useState(""),[groupFilter,setGroupFilter]=useState("");
 const accountLoadSequence=useRef(0),proxyLoadSequence=useRef(0),profileAccountSequence=useRef(0),profileJobSequence=useRef(0),qrPollLock=useRef(false),authActionLock=useRef(false),bulkActionLock=useRef(false);
 useEffect(()=>{const timer=window.setTimeout(()=>setDebouncedQuery(query),280);return()=>window.clearTimeout(timer)},[query]);
 const [offset,setOffset]=useState(0),[selected,setSelected]=useState<string[]>([]);
 const [busy,setBusy]=useState(false),[error,setError]=useState(""),[notice,setNotice]=useState("");
 const [loginOpen,setLoginOpen]=useState(false),[loginMode,setLoginMode]=useState<LoginMode>("qr");
 const [groupName,setGroupName]=useState(""),[phone,setPhone]=useState(""),[loginGroup,setLoginGroup]=useState("");
 const [loginAccountID,setLoginAccountID]=useState(""),[phoneCode,setPhoneCode]=useState(""),[twoFactor,setTwoFactor]=useState(false),[phonePassword,setPhonePassword]=useState("");
 const [delivery,setDelivery]=useState(""),[resendAfter,setResendAfter]=useState(0);
 const [qr,setQR]=useState<QRState|null>(null),[qrImage,setQRImage]=useState("");
 const [importFile,setImportFile]=useState<File|null>(null),[importPhone,setImportPhone]=useState(""),[importName,setImportName]=useState(""),[importGroup,setImportGroup]=useState("");
 const [profile,setProfile]=useState<{account:Account;data:unknown}|null>(null);
 const [deviceProfile,setDeviceProfile]=useState<{account:Account;data:DeviceProfile}|null>(null);
 const [proxyEditor,setProxyEditor]=useState<{id?:string;input:ProxyInput}|null>(null);
 const [profileAccounts,setProfileAccounts]=useState<Account[]>([]),[profileSelected,setProfileSelected]=useState<string[]>([]);
 const [profileForm,setProfileForm]=useState({first_name:"",last_name:"",about:"",generate_username:false});
 const [usernameSettings,setUsernameSettings]=useState<UsernameSettings>({prefix:"tg",random_length:10,charset:"alphanumeric"}),[usernameSettingsBusy,setUsernameSettingsBusy]=useState(false);
 const [avatarFile,setAvatarFile]=useState<File|null>(null),[profileJobs,setProfileJobs]=useState<ProfileJob[]>([]),[profileJobTotal,setProfileJobTotal]=useState(0),[profileJobStatus,setProfileJobStatus]=useState("");

 const loadAccounts=useCallback(async()=>{
  const sequence=++accountLoadSequence.current;
  setBusy(true);setError("");
  try{
   const params=new URLSearchParams({limit:"30",offset:String(offset)});
   if(debouncedQuery.trim())params.set("q",debouncedQuery.trim());
   if(status)params.set("status",status);
   if(groupFilter.trim())params.set("group",groupFilter.trim());
   const result=await api<AccountPage>(`/api/v1/accounts?${params}`,token);
   if(sequence===accountLoadSequence.current){setAccounts(result.accounts||[]);setTotal(result.total||0);setSelected([])}
  }catch(e){if(sequence===accountLoadSequence.current)setError(e instanceof Error?e.message:"账号列表加载失败")}
  finally{if(sequence===accountLoadSequence.current)setBusy(false)}
 },[token,offset,debouncedQuery,status,groupFilter]);
 const loadProxies=useCallback(async()=>{
  const sequence=++proxyLoadSequence.current;
  try{const result=await api<{proxies:Proxy[]}>("/api/v1/proxies",token);if(sequence===proxyLoadSequence.current)setProxies(result.proxies||[])}
  catch(e){if(sequence===proxyLoadSequence.current)setError(e instanceof Error?e.message:"代理列表加载失败")}
 },[token]);
 const loadProfileAccounts=useCallback(async()=>{const sequence=++profileAccountSequence.current;try{const r=await api<AccountPage>("/api/v1/accounts?limit=100&offset=0&status=authorized",token);if(sequence===profileAccountSequence.current)setProfileAccounts(r.accounts||[])}catch(e){if(sequence===profileAccountSequence.current)setError(e instanceof Error?e.message:"授权账号加载失败")}},[token]);
 const loadProfileJobs=useCallback(async()=>{const sequence=++profileJobSequence.current;try{const p=new URLSearchParams({limit:"50",offset:"0"});if(profileJobStatus)p.set("status",profileJobStatus);const r=await api<{jobs:ProfileJob[];total:number}>(`/api/v1/accounts/profile-updates?${p}`,token);if(sequence===profileJobSequence.current){setProfileJobs(r.jobs||[]);setProfileJobTotal(r.total||0)}}catch(e){if(sequence===profileJobSequence.current)setError(e instanceof Error?e.message:"资料更新任务加载失败")}},[token,profileJobStatus]);
 const loadUsernameSettings=useCallback(async()=>{try{const r=await api<{settings:UsernameSettings}>("/api/v1/accounts/username-generation-settings",token);setUsernameSettings(r.settings)}catch(e){setError(e instanceof Error?e.message:"用户名生成规则加载失败")}},[token]);
 useEffect(()=>{if(tab==="accounts")void Promise.all([loadAccounts(),loadProxies()]);else if(tab==="proxies")void loadProxies();else void Promise.all([loadProfileAccounts(),loadProfileJobs(),loadUsernameSettings()])},[tab,loadAccounts,loadProxies,loadProfileAccounts,loadProfileJobs,loadUsernameSettings]);
 useEffect(()=>{if(tab!=="profile")return;const refresh=()=>{if(document.visibilityState==="visible")void loadProfileJobs()};const timer=window.setInterval(refresh,10000);document.addEventListener("visibilitychange",refresh);return()=>{profileJobSequence.current++;window.clearInterval(timer);document.removeEventListener("visibilitychange",refresh)}},[tab,loadProfileJobs]);
 useEffect(()=>{
  if(!qr)return;
  let active=true;
  void QRCode.toDataURL(qr.url,{width:220,margin:2,errorCorrectionLevel:"M"}).then(value=>{if(active)setQRImage(value)}).catch(()=>{if(active)setQRImage("")});
  return()=>{active=false};
 },[qr?.url]);
 useEffect(()=>{
  if(!qr)return;
  let active=true;
  const timer=window.setInterval(async()=>{
   if(!active||document.visibilityState!=="visible"||qrPollLock.current)return;
   qrPollLock.current=true;
   try{
    const result=await api<{status:string;qr?:{url:string;expires_at:string};telegram_user?:unknown}>(`/api/v1/accounts/${qr.accountId}/qr/status`,token);
    if(!active)return;
    if(result.status==="authorized"){setQR(null);setQRImage("");setNotice("Telegram 账号授权成功");await loadAccounts()}
    else if(result.qr)setQR({accountId:qr.accountId,url:result.qr.url,expiresAt:result.qr.expires_at});
   }catch(e){if(active)setError(e instanceof Error?e.message:"二维码状态检查失败")}
   finally{qrPollLock.current=false}
  },3000);
  return()=>{active=false;window.clearInterval(timer)};
 },[qr?.accountId,token,loadAccounts]);

 const refresh=async()=>{if(tab==="accounts")await Promise.all([loadAccounts(),loadProxies()]);else if(tab==="proxies")await loadProxies();else await Promise.all([loadProfileAccounts(),loadProfileJobs()])};
 const toggle=(id:string)=>setSelected(current=>current.includes(id)?current.filter(x=>x!==id):[...current,id]);
 const allSelected=accounts.length>0&&accounts.every(a=>selected.includes(a.id));

 async function saveUsernameSettings(event:FormEvent){
  event.preventDefault();setUsernameSettingsBusy(true);setError("");setNotice("");
  try{const r=await api<{settings:UsernameSettings}>("/api/v1/accounts/username-generation-settings",token,{method:"PUT",body:JSON.stringify(usernameSettings)});setUsernameSettings(r.settings);setNotice("用户名生成规则已保存，新建任务时生效")}
  catch(e){setError(e instanceof Error?e.message:"用户名生成规则保存失败")}
  finally{setUsernameSettingsBusy(false)}
 }
 async function createProfileBatch(event:FormEvent){
  event.preventDefault();setError("");setNotice("");
  if(profileSelected.length<1||profileSelected.length>20){setError("请选择 1–20 个已授权账号");return}
  if(!profileForm.first_name.trim()&&!profileForm.last_name.trim()&&!profileForm.about.trim()&&!profileForm.generate_username){setError("请至少填写一个资料字段或启用自动生成用户名");return}
  setBusy(true);
  try{const r=await api<{batch_id:string;jobs:ProfileJob[]}>("/api/v1/accounts/profile-updates",token,{method:"POST",body:JSON.stringify({accounts:profileSelected.map(account_id=>({account_id,...profileForm}))})});setNotice(`资料更新任务已创建：${r.jobs.length} 个账号`);await loadProfileJobs()}
  catch(e){setError(e instanceof Error?e.message:"资料更新任务创建失败")}
  finally{setBusy(false)}
 }
 async function createAvatarBatch(event:FormEvent){
  event.preventDefault();setError("");setNotice("");
  if(profileSelected.length<1||profileSelected.length>20){setError("请选择 1–20 个已授权账号");return}
  if(!avatarFile){setError("请选择 JPEG 或 PNG 头像文件");return}
  if(avatarFile.size>5*1024*1024){setError("头像文件不能超过 5 MiB");return}
  setBusy(true);
  try{const body=new FormData();body.append("account_ids",JSON.stringify(profileSelected));body.append("photo",avatarFile);const r=await api<{batch_id:string;jobs:ProfileJob[];photo_format:string}>("/api/v1/accounts/profile-updates/photo",token,{method:"POST",body});setNotice(`头像更新任务已创建：${r.jobs.length} 个账号（${r.photo_format.toUpperCase()}）`);setAvatarFile(null);await loadProfileJobs()}
  catch(e){setError(e instanceof Error?e.message:"头像上传任务创建失败")}
  finally{setBusy(false)}
 }
 async function controlProfileJob(job:ProfileJob,action:"retry"|"resume"|"cancel"|"pause"){
  if(action==="cancel"&&!window.confirm("确定取消这条资料更新任务吗？"))return;
  setBusy(true);setError("");
  try{await api(`/api/v1/accounts/profile-updates/${job.id}/${action}`,token,{method:"POST"});setNotice("资料更新任务操作已提交");await loadProfileJobs()}
  catch(e){setError(e instanceof Error?e.message:"资料更新任务操作失败")}
  finally{setBusy(false)}
 }
 async function startQR(event:FormEvent){
  event.preventDefault();if(authActionLock.current)return;authActionLock.current=true;setBusy(true);setError("");setNotice("");
  try{
   const result=await api<{account:Account;status?:string;qr?:{url:string;expires_at:string}}>("/api/v1/accounts/qr/start",token,{method:"POST",body:JSON.stringify({group_name:loginGroup})});
   if(result.qr){setQR({accountId:result.account.id,url:result.qr.url,expiresAt:result.qr.expires_at});setLoginAccountID(result.account.id)}
   else{setNotice("Telegram 账号授权成功");setLoginOpen(false);await loadAccounts()}
  }catch(e){setError(e instanceof Error?e.message:"二维码登录启动失败")}
  finally{authActionLock.current=false;setBusy(false)}
 }
 async function startPhone(event:FormEvent){
  event.preventDefault();if(authActionLock.current)return;authActionLock.current=true;setBusy(true);setError("");
  try{
   const result=await api<{account:Account;status:string;delivery_type:string;resend_after_seconds:number}>("/api/v1/accounts/phone/start",token,{method:"POST",body:JSON.stringify({phone,group_name:loginGroup})});
   setLoginAccountID(result.account.id);setDelivery(result.delivery_type);setResendAfter(result.resend_after_seconds);setPhoneCode("");setTwoFactor(false);await loadAccounts();
  }catch(e){setError(e instanceof Error?e.message:"手机号登录启动失败")}
  finally{authActionLock.current=false;setBusy(false)}
 }
 async function verifyPhone(event:FormEvent){
  event.preventDefault();if(authActionLock.current)return;authActionLock.current=true;setBusy(true);setError("");
  try{
   const result=await api<{status:string}>(`/api/v1/accounts/${loginAccountID}/phone/verify`,token,{method:"POST",body:JSON.stringify({code:phoneCode})});
   if(result.status==="two_factor_required"){setTwoFactor(true);return}
   setNotice("Telegram 账号授权成功");setLoginOpen(false);setLoginAccountID("");await loadAccounts();
  }catch(e){setError(e instanceof Error?e.message:"验证码验证失败")}
  finally{authActionLock.current=false;setBusy(false)}
 }
 async function verifyPassword(event:FormEvent){
  event.preventDefault();if(authActionLock.current)return;authActionLock.current=true;setBusy(true);setError("");
  try{await api(`/api/v1/accounts/${loginAccountID}/phone/2fa`,token,{method:"POST",body:JSON.stringify({password:phonePassword})});setNotice("Telegram 账号授权成功");setLoginOpen(false);setLoginAccountID("");setPhonePassword("");await loadAccounts()}
  catch(e){setError(e instanceof Error?e.message:"两步验证失败")}
  finally{authActionLock.current=false;setBusy(false)}
 }
 async function cancelPending(){
  if(!loginAccountID)return;
  try{await api(`/api/v1/accounts/${loginAccountID}/qr`,token,{method:"DELETE"});setQR(null);setQRImage("");setLoginAccountID("");setTwoFactor(false);await loadAccounts()}
  catch(e){setError(e instanceof Error?e.message:"取消登录失败")}
 }
 async function importSession(event:FormEvent){
  event.preventDefault();if(!importFile)return;
  setBusy(true);setError("");setNotice("");
  try{
   const data=new FormData();data.append("file",importFile);
   if(importPhone)data.append("phone",importPhone);
   if(importName)data.append("display_name",importName);
   if(importGroup)data.append("group_name",importGroup);
   const result=await api<{total:number;succeeded:number;failed:number}>("/api/v1/accounts/import-file",token,{method:"POST",body:data});
   setNotice(`导入完成：成功 ${result.succeeded} 个，失败 ${result.failed} 个`);setImportFile(null);await loadAccounts();
  }catch(e){setError(e instanceof Error?e.message:"Session 导入失败")}
  finally{setBusy(false)}
 }
 async function bulkAction(action:"disconnect"|"reconnect"|"delete"){
  if(!selected.length||bulkActionLock.current)return;
  if(!window.confirm(action==="delete"?`确定删除选中的 ${selected.length} 个账号吗？`:action==="disconnect"?"确定断开选中账号吗？":"确定重连选中账号吗？"))return;
  bulkActionLock.current=true;setBusy(true);setError("");setNotice("");
  try{
   if(action==="delete")await api("/api/v1/accounts/bulk",token,{method:"DELETE",body:JSON.stringify({account_ids:selected})});
   else await api("/api/v1/accounts/bulk-action",token,{method:"POST",body:JSON.stringify({account_ids:selected,action})});
   setNotice(action==="delete"?"所选账号已删除":"账号运行操作已提交");await loadAccounts();
  }catch(e){setError(e instanceof Error?e.message:"批量操作失败")}
  finally{bulkActionLock.current=false;setBusy(false)}
 }
 async function assignGroup(){
  if(!selected.length)return;
  setBusy(true);setError("");
  try{await api("/api/v1/accounts/group",token,{method:"PATCH",body:JSON.stringify({account_ids:selected,group_name:groupName})});setNotice("账号分组已更新");await loadAccounts()}
  catch(e){setError(e instanceof Error?e.message:"分组更新失败")}
  finally{setBusy(false)}
 }
 async function assignTags(){
  if(!selected.length)return;
  const raw=window.prompt("输入标签，多个标签用英文逗号分隔");if(raw===null)return;
  setBusy(true);setError("");
  try{await api("/api/v1/accounts/tags",token,{method:"PATCH",body:JSON.stringify({account_ids:selected,tags:raw.split(",").map(x=>x.trim()).filter(Boolean)})});setNotice("账号标签已更新");await loadAccounts()}
  catch(e){setError(e instanceof Error?e.message:"标签更新失败")}
  finally{setBusy(false)}
 }
 async function editAccount(account:Account,field:"name"|"group"|"tags"){
  const current=field==="name"?account.display_name:field==="group"?account.group_name:account.tags.join(", ");
  const value=window.prompt(field==="name"?"修改显示名称":field==="group"?"修改分组名称":"修改标签（英文逗号分隔）",current);
  if(value===null)return;
  setBusy(true);setError("");
  try{
   if(field==="name")await api(`/api/v1/accounts/${account.id}`,token,{method:"PATCH",body:JSON.stringify({display_name:value})});
   if(field==="group")await api(`/api/v1/accounts/${account.id}/group`,token,{method:"PATCH",body:JSON.stringify({group_name:value})});
   if(field==="tags")await api(`/api/v1/accounts/${account.id}/tags`,token,{method:"PATCH",body:JSON.stringify({tags:value.split(",").map(x=>x.trim()).filter(Boolean)})});
   await loadAccounts();
  }catch(e){setError(e instanceof Error?e.message:"账号资料更新失败")}
  finally{setBusy(false)}
 }
 async function showProfile(account:Account){
  setBusy(true);setError("");
  try{const result=await api<{profile:unknown}>(`/api/v1/accounts/${account.id}/profile`,token);setProfile({account,data:result.profile})}
  catch(e){setError(e instanceof Error?e.message:"读取 Telegram 资料失败")}
  finally{setBusy(false)}
 }
 async function showDeviceProfile(account:Account){
  setBusy(true);setError("");
  try{const result=await api<{device_profile:DeviceProfile}>(`/api/v1/accounts/${account.id}/device-profile`,token);setDeviceProfile({account,data:result.device_profile})}
  catch(e){setError(e instanceof Error?e.message:"读取设备资料失败")}
  finally{setBusy(false)}
 }
 async function bindProxy(account:Account,proxyID:string){
  setBusy(true);setError("");
  try{await api(`/api/v1/accounts/${account.id}/proxy`,token,{method:"PATCH",body:JSON.stringify({proxy_id:proxyID})});setNotice("账号代理绑定已更新");await loadAccounts()}
  catch(e){setError(e instanceof Error?e.message:"代理绑定失败")}
  finally{setBusy(false)}
 }
 async function saveProxy(event:FormEvent){
  event.preventDefault();if(!proxyEditor)return;
  setBusy(true);setError("");
  try{
   const input=proxyEditor.input;
   await api(proxyEditor.id?`/api/v1/proxies/${proxyEditor.id}`:"/api/v1/proxies",token,{method:proxyEditor.id?"PUT":"POST",body:JSON.stringify(input)});
   setProxyEditor(null);setNotice("代理配置已保存");await loadProxies();
  }catch(e){setError(e instanceof Error?e.message:"代理保存失败")}
  finally{setBusy(false)}
 }
 async function checkProxy(proxy:Proxy){
  setBusy(true);setError("");
  try{const result=await api<{reachable:boolean;message?:string}>(`/api/v1/proxies/${proxy.id}/check`,token,{method:"POST"});setNotice(result.reachable?"代理检测成功":"代理检测失败");await loadProxies()}
  catch(e){setError(e instanceof Error?e.message:"代理检测失败");await loadProxies()}
  finally{setBusy(false)}
 }
 async function deleteProxy(proxy:Proxy){
  if(!window.confirm(`确定删除代理「${proxy.name}」吗？`))return;
  setBusy(true);setError("");
  try{await api(`/api/v1/proxies/${proxy.id}`,token,{method:"DELETE"});setNotice("代理已删除");await loadProxies()}
  catch(e){setError(e instanceof Error?e.message:"代理删除失败")}
  finally{setBusy(false)}
 }
 async function setDefaultProxy(proxy:Proxy){
  setBusy(true);
  try{await api("/api/v1/proxies/default",token,{method:"PUT",body:JSON.stringify({proxy_id:proxy.id})});await loadProxies()}
  catch(e){setError(e instanceof Error?e.message:"设置默认代理失败")}
  finally{setBusy(false)}
 }

 return <div className="accounts-page">
  <div className="accounts-heading"><div><div className="eyebrow">ACCOUNT MANAGEMENT</div><h1>账号管理</h1><p>账号登录、分组、代理和资料管理</p></div><div className="accounts-actions"><button className="secondary" onClick={()=>void refresh()} disabled={busy}>{busy?"处理中…":"↻ 刷新"}</button><button className="primary" onClick={()=>{setLoginOpen(true);setLoginMode("qr");setError("")}}>添加账号 / 登录</button></div></div>
  {error&&<div className="alert error">{error}</div>}{notice&&<div className="alert success">{notice}</div>}
  <div className="account-tabs"><button className={tab==="accounts"?"active":""} onClick={()=>setTab("accounts")}>账号列表</button><button className={tab==="proxies"?"active":""} onClick={()=>setTab("proxies")}>代理管理</button><button className={tab==="profile"?"active":""} onClick={()=>setTab("profile")}>资料修改 / 头像</button></div>
  {tab==="profile"?<section className="profile-workspace"><div className="panel profile-targets"><div className="panel-heading"><div><h2>选择授权账号</h2><p>资料修改和头像上传均通过队列执行，每批最多 20 个账号。</p></div><span className="muted">已选 {profileSelected.length} / 20</span></div><div className="profile-account-list">{profileAccounts.map(a=><label key={a.id}><input type="checkbox" checked={profileSelected.includes(a.id)} disabled={!profileSelected.includes(a.id)&&profileSelected.length>=20} onChange={e=>setProfileSelected(v=>e.target.checked?[...v,a.id]:v.filter(x=>x!==a.id))}/><span><strong>{a.display_name||a.phone}</strong><small>{a.phone} · {a.group_name||"未分组"}</small></span></label>)}{!profileAccounts.length&&<div className="empty">暂无已授权账号</div>}</div></div><div className="profile-forms"><section className="panel"><h2>用户名生成规则</h2><p>规则按当前租户保存，只影响之后创建的资料更新任务。</p><form className="account-form" onSubmit={saveUsernameSettings}><label>用户名固定前缀<input value={usernameSettings.prefix} maxLength={20} onChange={e=>setUsernameSettings(v=>({...v,prefix:e.target.value}))}/></label><label>随机字符长度<input type="number" min={1} max={30} value={usernameSettings.random_length} onChange={e=>setUsernameSettings(v=>({...v,random_length:Number(e.target.value)}))}/></label><label>字符集<select value={usernameSettings.charset} onChange={e=>setUsernameSettings(v=>({...v,charset:e.target.value as UsernameSettings["charset"]}))}><option value="alphanumeric">字母 + 数字</option><option value="letters">仅字母</option></select></label><small>最终用户名长度必须为 5–32 位，前缀以字母开头，只能包含字母、数字和下划线。</small><button className="primary" disabled={usernameSettingsBusy}>保存用户名规则</button></form></section><section className="panel"><h2>修改账号资料</h2><p>可批量更新名字、姓氏、简介，或使用已保存的用户名生成规则。</p><form className="account-form" onSubmit={createProfileBatch}><label>名字<input value={profileForm.first_name} onChange={e=>setProfileForm(v=>({...v,first_name:e.target.value}))} maxLength={64}/></label><label>姓氏<input value={profileForm.last_name} onChange={e=>setProfileForm(v=>({...v,last_name:e.target.value}))} maxLength={64}/></label><label>个人简介<textarea value={profileForm.about} onChange={e=>setProfileForm(v=>({...v,about:e.target.value}))} maxLength={70} rows={3}/></label><label className="profile-checkbox"><input type="checkbox" checked={profileForm.generate_username} onChange={e=>setProfileForm(v=>({...v,generate_username:e.target.checked}))}/>自动生成用户名</label><small>名字/姓氏最多 64 字符，简介最多 70 字符。任务由 Worker 异步执行。</small><button className="primary" disabled={busy||profileSelected.length===0}>创建资料更新任务</button></form></section><section className="panel"><h2>上传账号头像</h2><p>支持 JPEG / PNG，最大 5 MiB、4096×4096、16MP。一个文件可应用到所选账号。</p><form className="account-form" onSubmit={createAvatarBatch}><label>头像图片<input key={avatarFile?.name||"avatar-empty"} type="file" accept="image/jpeg,image/png,.jpg,.jpeg,.png" onChange={e=>setAvatarFile(e.target.files?.[0]||null)} required/></label>{avatarFile&&<small>{avatarFile.name} · {(avatarFile.size/1024/1024).toFixed(2)} MiB</small>}<button className="primary" disabled={busy||!avatarFile||profileSelected.length===0}>创建头像上传任务</button></form></section></div><section className="panel profile-jobs"><div className="panel-heading"><div><h2>资料更新任务</h2><p>显示当前租户的异步执行结果与错误信息</p></div><div className="profile-job-filters"><select value={profileJobStatus} onChange={e=>setProfileJobStatus(e.target.value)}><option value="">全部状态</option>{["pending","processing","paused","completed","failed","cancelled"].map(s=><option key={s} value={s}>{s}</option>)}</select><button className="secondary" onClick={()=>void loadProfileJobs()} disabled={busy}>刷新</button></div></div><div className="profile-job-list">{profileJobs.map(job=><article key={job.id}><div className="profile-job-title"><div><strong>{profileAccounts.find(a=>a.id===job.account_id)?.display_name||profileAccounts.find(a=>a.id===job.account_id)?.phone||job.account_id}</strong><small>{job.first_name} {job.last_name} {job.about?`· ${job.about}`:""} {job.generate_username?"· 自动生成用户名":""} {job.has_photo?"· 含头像":" "}</small></div><span className={`status ${job.status==="completed"?"status-joined":job.status==="failed"?"status-failed":job.status==="cancelled"?"status-paused":"status-pending"}`}>{job.status}</span></div><small>任务 {job.id} · 批次 {job.batch_id} · 尝试 {job.attempts} 次 · 下次执行 {fmt(job.next_run_at)}</small>{job.applied_username&&<small>已应用用户名：@{job.applied_username}</small>}{job.last_error&&<small className="account-last-error">{job.last_error}</small>}<div className="profile-job-actions">{job.status==="pending"&&<button onClick={()=>void controlProfileJob(job,"pause")} disabled={busy}>暂停</button>}{job.status==="failed"&&<button onClick={()=>void controlProfileJob(job,"retry")} disabled={busy}>重试</button>}{job.status==="paused"&&<button onClick={()=>void controlProfileJob(job,"resume")} disabled={busy}>恢复</button>}{["pending","paused","failed"].includes(job.status)&&<button className="danger-text" onClick={()=>void controlProfileJob(job,"cancel")} disabled={busy}>取消</button>}</div></article>)}{!profileJobs.length&&<div className="empty">暂无资料更新任务</div>}</div><div className="profile-job-total">共 {profileJobTotal} 条任务</div></section></section>:tab==="accounts"?<>
   <section className="panel">
    <div className="account-filters"><input value={query} onChange={e=>{setQuery(e.target.value);setOffset(0)}} placeholder="搜索手机号或显示名称"/><input value={groupFilter} onChange={e=>{setGroupFilter(e.target.value);setOffset(0)}} placeholder="按分组筛选"/><select value={status} onChange={e=>{setStatus(e.target.value);setOffset(0)}}><option value="">全部状态</option><option value="authorized">已授权</option><option value="pending">待登录</option><option value="error">异常</option><option value="disconnected">已断开</option></select><button className="secondary" onClick={()=>void loadAccounts()} disabled={busy}>查询</button></div>
    <div className="account-table-meta"><span>共 {total} 个账号</span><span>已选 {selected.length} 个</span></div>
    {selected.length>0&&<div className="account-bulkbar"><input value={groupName} onChange={e=>setGroupName(e.target.value)} placeholder="批量设置分组"/><button onClick={()=>void assignGroup()} disabled={busy}>设置分组</button><button onClick={()=>void assignTags()} disabled={busy}>设置标签</button><button onClick={()=>void bulkAction("disconnect")} disabled={busy}>断开</button><button onClick={()=>void bulkAction("reconnect")} disabled={busy}>重连</button><button className="danger-text" onClick={()=>void bulkAction("delete")} disabled={busy}>删除</button></div>}
    <div className="account-table-wrap"><table className="account-table"><thead><tr><th><input type="checkbox" checked={allSelected} onChange={e=>setSelected(e.target.checked?accounts.map(a=>a.id):[])}/></th><th>账号</th><th>分组 / 标签</th><th>状态</th><th>代理</th><th>最近更新</th><th>操作</th></tr></thead><tbody>
     {accounts.map(a=><tr key={a.id}><td data-label="选择"><input type="checkbox" checked={selected.includes(a.id)} onChange={()=>toggle(a.id)}/></td><td data-label="账号"><strong>{a.display_name||a.phone||"未命名账号"}</strong><small>{a.phone}</small><small className="account-id">{a.id}</small></td><td data-label="分组 / 标签">{a.group_name||"未分组"}<small>{(a.tags||[]).join(" · ")||"无标签"}</small></td><td data-label="状态"><span className={`status ${statusClass[a.status]||""}`}>{statusLabel[a.status]||a.status}</span>{a.last_error&&<small className="account-last-error">{a.last_error}</small>}</td><td data-label="代理"><span>{a.has_proxy?"已绑定代理":"直连"}</span><select aria-label="绑定代理" value="" onChange={e=>void bindProxy(a,e.target.value)}><option value="" disabled>更换代理…</option>{proxies.map(p=><option key={p.id} value={p.id}>{p.name}</option>)}</select></td><td data-label="最近更新">{fmt(a.updated_at)}</td><td data-label="操作"><div className="account-row-actions"><button onClick={()=>void showProfile(a)} disabled={busy}>资料</button><button onClick={()=>void showDeviceProfile(a)} disabled={busy}>设备资料</button>{a.has_proxy&&<button onClick={()=>void bindProxy(a,"")} disabled={busy}>解除代理</button>}<button onClick={()=>void editAccount(a,"name")} disabled={busy}>改名</button><button onClick={()=>void editAccount(a,"group")} disabled={busy}>分组</button><button onClick={()=>void editAccount(a,"tags")} disabled={busy}>标签</button><button className="danger-text" onClick={()=>void bulkActionForOne(a)} disabled={busy}>删除</button></div></td></tr>)}
     {!accounts.length&&<tr><td colSpan={7} className="empty">{busy?"正在加载账号…":error?"加载失败，请检查提示后重试":"暂无账号，先添加或导入账号"}</td></tr>}
    </tbody></table></div>
    <div className="account-pagination"><span>第 {Math.floor(offset/30)+1} / {Math.max(1,Math.ceil(total/30))} 页</span><div><button className="secondary" disabled={offset===0||busy} onClick={()=>setOffset(Math.max(0,offset-30))}>上一页</button><button className="secondary" disabled={offset+30>=total||busy} onClick={()=>setOffset(offset+30)}>下一页</button></div></div>
   </section>
  </>:<section className="panel"><div className="proxy-heading"><div><h2>代理列表</h2><p>代理密码只写入加密存储，不会回显</p></div><button className="primary" onClick={()=>setProxyEditor({input:{name:"",protocol:"socks5",host:"",port:1080,username:"",password:""}})}>添加代理</button></div>
   {proxies.length?<div className="proxy-grid">{proxies.map(p=><article className="proxy-card" key={p.id}><div className="proxy-card-title"><strong>{p.name}</strong>{p.is_default&&<span className="status status-joined">默认</span>}</div><p>{p.protocol.toUpperCase()} · {p.host}:{p.port}</p><small>{p.username||"无用户名"} · {p.has_password?"已配置密码":"无密码"}</small><small>最近检测：{p.last_check_status||"尚未检测"} {p.last_check_latency_ms?(`· ${p.last_check_latency_ms} ms`):""}</small><div className="proxy-actions"><button onClick={()=>void checkProxy(p)} disabled={busy}>检测</button><button onClick={()=>setDefaultProxy(p)} disabled={busy||p.is_default}>设为默认</button><button onClick={()=>setProxyEditor({id:p.id,input:{name:p.name,protocol:p.protocol,host:p.host,port:p.port,username:p.username,password:""}})}>编辑</button><button className="danger-text" onClick={()=>void deleteProxy(p)} disabled={busy}>删除</button></div></article>)}</div>:<div className="empty">暂无代理配置</div>}
  </section>}
  {loginOpen&&<div className="modal-backdrop" onMouseDown={e=>{if(e.target===e.currentTarget&&!qr)setLoginOpen(false)}}><section className="account-modal"><div className="modal-heading"><div><h2>添加 Telegram 账号</h2><p>使用二维码、手机号验证码或 Session 文件</p></div><button className="ghost" onClick={()=>setLoginOpen(false)} disabled={!!qr}>关闭</button></div>
   <div className="login-tabs"><button className={loginMode==="qr"?"active":""} onClick={()=>setLoginMode("qr")}>扫码登录</button><button className={loginMode==="phone"?"active":""} onClick={()=>setLoginMode("phone")}>手机号登录</button><button className={loginMode==="session"?"active":""} onClick={()=>setLoginMode("session")}>Session 导入</button></div>
   {!qr&&loginMode==="qr"&&<form onSubmit={startQR} className="account-form"><label>账号分组（可选）<input value={loginGroup} onChange={e=>setLoginGroup(e.target.value)} placeholder="例如：客服账号"/></label><button className="primary" disabled={busy}>生成 Telegram 登录二维码</button></form>}
   {qr&&<div className="qr-login"><p>请使用 Telegram 官方客户端扫描二维码完成登录。</p>{qrImage?<img src={qrImage} alt="Telegram 登录二维码"/>:<p>二维码生成中…</p>}<small>有效期至 {fmt(qr.expiresAt)}</small><code>{qr.url}</code><button className="secondary" onClick={()=>{void navigator.clipboard?.writeText(qr.url).then(()=>setNotice("登录链接已复制")).catch(()=>setError("复制失败，请检查浏览器剪贴板权限"))}}>复制登录链接</button><button className="danger-text" onClick={()=>void cancelPending()}>取消本次登录</button></div>}
   {loginMode==="phone"&&!qr&&<form onSubmit={loginAccountID?(twoFactor?verifyPassword:verifyPhone):startPhone} className="account-form">
    {!loginAccountID?<><label>手机号<input value={phone} onChange={e=>setPhone(e.target.value)} placeholder="+8613800000000" required/></label><label>账号分组（可选）<input value={loginGroup} onChange={e=>setLoginGroup(e.target.value)}/></label><button className="primary" disabled={busy}>发送验证码</button></>:twoFactor?<><p>Telegram 要求输入两步验证密码。</p><label>两步验证密码<input type="password" value={phonePassword} onChange={e=>setPhonePassword(e.target.value)} required/></label><button className="primary" disabled={busy}>验证并完成登录</button></>:<><p>验证码发送方式：{delivery||"Telegram"}。请填写 Telegram 收到的验证码。</p><label>验证码<input value={phoneCode} onChange={e=>setPhoneCode(e.target.value)} required/></label><small>请在 {resendAfter} 秒后再尝试重新发送；当前流程可继续验证。</small><button className="primary" disabled={busy}>验证验证码</button><button type="button" className="danger-text" onClick={()=>void cancelPending()}>取消登录</button></>}
   </form>}
   {loginMode==="session"&&!qr&&<form onSubmit={importSession} className="account-form"><label>Session 文件<input type="file" accept=".session,.json,.csv,.txt" onChange={e=>setImportFile(e.target.files?.[0]||null)} required/></label>{importFile?.name.toLowerCase().endsWith(".session")&&<><label>账号手机号<input value={importPhone} onChange={e=>setImportPhone(e.target.value)} placeholder="+8613800000000" required/></label><label>显示名称<input value={importName} onChange={e=>setImportName(e.target.value)}/></label><label>分组<input value={importGroup} onChange={e=>setImportGroup(e.target.value)}/></label></>}<small>支持 Telethon .session、JSON、CSV 和 TXT；单文件最大 1 MiB，最多导入 20 个账号。</small><button className="primary" disabled={busy||!importFile}>导入账号</button></form>}
   {error&&<div className="alert error">{error}</div>}
  </section></div>}
  {profile&&<div className="modal-backdrop" onMouseDown={e=>{if(e.target===e.currentTarget)setProfile(null)}}><section className="account-modal"><div className="modal-heading"><div><h2>{profile.account.display_name||profile.account.phone} · Telegram 资料</h2><p>读取 Telegram 当前账号资料</p></div><button className="ghost" onClick={()=>setProfile(null)}>关闭</button></div><pre className="profile-json">{JSON.stringify(profile.data,null,2)}</pre></section></div>}
  {deviceProfile&&<div className="modal-backdrop" onMouseDown={e=>{if(e.target===e.currentTarget)setDeviceProfile(null)}}><section className="account-modal"><div className="modal-heading"><div><h2>{deviceProfile.account.display_name||deviceProfile.account.phone} · 设备资料</h2><p>device-manager 持久化的 iOS 设备身份；此处只读，不会重新生成或修改</p></div><button className="ghost" onClick={()=>setDeviceProfile(null)}>关闭</button></div><div className="profile-json"><p>平台：{deviceProfile.data.platform}</p><p>设备型号：{deviceProfile.data.device_model}</p><p>系统版本：{deviceProfile.data.system_version}</p><p>Telegram App 版本：{deviceProfile.data.app_version}</p><p>语言：{deviceProfile.data.lang_code}</p><p>系统语言：{deviceProfile.data.system_lang_code}</p><p>语言包：{deviceProfile.data.lang_pack||"—"}</p><p>Package ID：{deviceProfile.data.package_id||"—"}</p></div></section></div>}
  {proxyEditor&&<div className="modal-backdrop"><form className="account-modal account-form" onSubmit={saveProxy}><div className="modal-heading"><h2>{proxyEditor.id?"编辑代理":"添加代理"}</h2><button type="button" className="ghost" onClick={()=>setProxyEditor(null)}>关闭</button></div><label>名称<input value={proxyEditor.input.name} onChange={e=>setProxyEditor(v=>v?{...v,input:{...v.input,name:e.target.value}}:v)} required/></label><label>协议<select value={proxyEditor.input.protocol} onChange={e=>setProxyEditor(v=>v?{...v,input:{...v.input,protocol:e.target.value}}:v)}><option value="socks5">SOCKS5</option><option value="socks4">SOCKS4</option><option value="http">HTTP</option><option value="https">HTTPS</option></select></label><label>主机<input value={proxyEditor.input.host} onChange={e=>setProxyEditor(v=>v?{...v,input:{...v.input,host:e.target.value}}:v)} required/></label><label>端口<input type="number" min="1" max="65535" value={proxyEditor.input.port} onChange={e=>setProxyEditor(v=>v?{...v,input:{...v.input,port:Number(e.target.value)}}:v)} required/></label><label>用户名<input value={proxyEditor.input.username} onChange={e=>setProxyEditor(v=>v?{...v,input:{...v.input,username:e.target.value}}:v)}/></label><label>密码<input type="password" value={proxyEditor.input.password} placeholder={proxyEditor.id?"留空表示保留原密码":"可选"} onChange={e=>setProxyEditor(v=>v?{...v,input:{...v.input,password:e.target.value}}:v)}/></label><button className="primary" disabled={busy}>保存代理</button></form></div>}
 </div>;
 
 async function bulkActionForOne(account:Account){if(!window.confirm(`确定删除账号 ${account.phone} 吗？`))return;setBusy(true);try{await api(`/api/v1/accounts/${account.id}`,token,{method:"DELETE"});setNotice("账号已删除");await loadAccounts()}catch(e){setError(e instanceof Error?e.message:"删除账号失败")}finally{setBusy(false)}}
}
