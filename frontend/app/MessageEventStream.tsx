"use client";

import {useEffect} from "react";

const API_BASE=(process.env.NEXT_PUBLIC_API_BASE_URL||"").replace(/\/$/,"");

export function MessageEventStream({token,onEvent}:{token:string;onEvent:()=>void}){
 useEffect(()=>{
  let active=true;
  let controller:AbortController|undefined;
  let cursor="";
  let reconnectTimer:number|undefined;
  const connect=async()=>{
   while(active){
    controller=new AbortController();
    try{
     const query=cursor?`?last_event_id=${encodeURIComponent(cursor)}`:"";
     const response=await fetch(API_BASE+`/api/v1/messages/events`+query,{
      headers:{Authorization:`Bearer ${token}`},
      cache:"no-store",
      signal:controller.signal
     });
     if(!response.ok||!response.body)throw new Error("实时消息连接不可用");
     const reader=response.body.getReader();
     const decoder=new TextDecoder();
     let buffer="";
     while(active){
      const result=await reader.read();
      if(result.done)break;
      buffer+=decoder.decode(result.value,{stream:true});
      const frames=buffer.split(/\r?\n\r?\n/);
      buffer=frames.pop()||"";
      for(const frame of frames){
       let eventType="";
       let eventID="";
       let hasData=false;
       for(const line of frame.split(/\r?\n/)){
        if(line.startsWith("event:"))eventType=line.slice(6).trim();
        else if(line.startsWith("id:"))eventID=line.slice(3).trim();
        else if(line.startsWith("data:"))hasData=true;
       }
       if(eventID)cursor=eventID;
       if(eventType==="message"&&hasData)onEvent();
      }
     }
    }catch{
     // The inbox keeps its normal refresh fallback while the stream reconnects.
    }
    if(active)await new Promise<void>(resolve=>{
     reconnectTimer=window.setTimeout(resolve,3000);
    });
   }
  };
  void connect();
  return()=>{
   active=false;
   controller?.abort();
   if(reconnectTimer!==undefined)window.clearTimeout(reconnectTimer);
  };
 },[token,onEvent]);
 return null;
}
