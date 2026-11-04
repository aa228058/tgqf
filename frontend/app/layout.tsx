import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "TG Manager · 任务中心",
  description: "Telegram 自动化任务统一管理",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="zh-CN"><body>{children}</body></html>;
}
