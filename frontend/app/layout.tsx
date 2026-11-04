import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "TG Manager · Telegram 管理控制台",
  description: "Telegram 账号、消息、群组和任务管理控制台",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="zh-CN"><body>{children}</body></html>;
}
