"use client";
import {useState} from "react";
import {SettingsPage} from "./SettingsPage";
import {AdminModerationPage} from "./AdminModerationPage";
export function AdminConsole({token,user}:{token:string;user:{role:string}}){
 const [tab,setTab]=useState("tenants");
 return <div>
  <nav className="settings-tabs">
   <button className={tab==="tenants"?"active":""} onClick={()=>setTab("tenants")}>租户与卡密</button>
   <button className={tab==="groups"?"active":""} onClick={()=>setTab("groups")}>公共群组审核</button>
  </nav>
  {tab==="tenants"?<SettingsPage token={token} user={user} initialTab="cards"/>:<AdminModerationPage token={token}/>}
 </div>;
}