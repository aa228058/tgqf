"use client";

import { apiErrorMessage, readAPIJSON } from "./api-response";
import {useEffect,useState} from "react";
const API=(process.env.NEXT_PUBLIC_API_BASE_URL||"").replace(/\/$/,"");
type Row={id:number;action:string;tenant_name?:string;actor_username?:string;created_at:string};
export function AdminAuditPage({token}:{token:string}){
 const [rows,setRows]=useState<Row[]>([]);
 const [error,setError]=useState("");
 useEffect(()=>{
  let alive=true;
  fetch(API+"/api/v1/admin/audit-logs?limit=50&offset=0",{headers:{Authorization:"Bearer "+token},cache:"no-store"})
   .then(async r=>{if(!r.ok)throw new Error(await apiErrorMessage(r,"读取失败"));return readAPIJSON<{logs:Row[]}>(r)})
   .then(v=>{if(alive)setRows(v.logs||[])})
   .catch(e=>{if(alive)setError(e.message||"读取失败")});
  return()=>{alive=false};
 },[token]);
 return <section className="panel"><h2>审计日志</h2>{error&&<p>{error}</p>}<ul>{rows.map(r=><li key={r.id}>{new Date(r.created_at).toLocaleString()} · {r.action} · {r.tenant_name||"全局"} · {r.actor_username||"系统"}</li>)}</ul>{!rows.length&&!error&&<p>暂无记录</p>}</section>;
}
