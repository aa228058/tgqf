import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "TG 账号管理工具",
  description: "Telegram 账号、群组、消息与任务统一管理工具",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="zh-CN"><body>{children}</body></html>;
}
