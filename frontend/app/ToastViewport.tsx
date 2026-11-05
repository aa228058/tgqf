"use client";

import { useEffect, useState } from "react";

type Toast = { id: number; message: string; kind: "success" | "error" | "info" };

export function ToastViewport() {
  const [items, setItems] = useState<Toast[]>([]);
  useEffect(() => {
    let sequence = 0;
    const onToast = (event: Event) => {
      const detail = (event as CustomEvent<{message?: string; kind?: Toast["kind"]}>).detail;
      if (!detail?.message) return;
      const id = ++sequence;
      setItems(current => [...current.slice(-3), { id, message: detail.message!, kind: detail.kind || "success" }]);
      window.setTimeout(() => setItems(current => current.filter(item => item.id !== id)), 4200);
    };
    window.addEventListener("tgqf:toast", onToast);
    return () => window.removeEventListener("tgqf:toast", onToast);
  }, []);
  return <div className="toast-viewport" aria-live="polite" aria-atomic="false">
    {items.map(item => <div className={"toast-item toast-" + item.kind} role={item.kind === "error" ? "alert" : "status"} key={item.id}>
      <span className="toast-icon" aria-hidden="true">{item.kind === "success" ? "✓" : item.kind === "error" ? "!" : "i"}</span>
      <span>{item.message}</span>
      <button type="button" aria-label="关闭提示" onClick={() => setItems(current => current.filter(value => value.id !== item.id))}>×</button>
    </div>)}
  </div>;
}
