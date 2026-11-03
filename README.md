# TG账号管理系统

面向经过授权的 Telegram 普通用户账号的多租户管理平台，使用 MTProto，不使用 Bot API。

## 技术架构

- Web：Next.js、React、TypeScript、Tailwind CSS、shadcn/ui
- Backend：Go
- Telegram MTProto：gotd (`github.com/gotd/td`)
- Device profile：适配 `device-manager` 的 iOS 设备资料生成逻辑；每个账号生成后固定保存
- Data：PostgreSQL、Redis
- Realtime：WebSocket
- Deployment：Docker Compose、Nginx、HTTPS

## 核心模块

1. 多租户、超级管理员、卡密授权与会话安全
2. Telegram 普通用户账号登录、Session 加密存储、代理与设备资料
3. 独立 Telegram Worker、连接健康监控和断线恢复
4. 公共群组资料库、群链接采集、群组检测与自动加群
5. 关键词监听、规则编排和授权范围内的自动处理
6. 任务中心、实时进度、暂停/恢复/取消和审计日志
7. 系统监控、备份与运维设置

## 自动化边界

自动联系仅面向明确授权且允许联系的对象；必须提供退订/停止联系、去重、频率限制、每日上限、失败冷却与审计记录。严格遵守 Telegram 隐私设置、FloodWait 和平台限制，不实现绕过限制或无差别骚扰。

联系人卡片使用 Telegram 原生联系人消息。真实语音通话需要独立验证 MTProto 信令、密钥协商和音频传输链路；在验证完成前不宣称可用。

## 开发原则

- 全新 Go 后端，不继续扩展早期 Python/Telethon 占位代码。
- HTTP API 与 Telegram Worker 分离；长连接不运行在 HTTP 请求处理器中。
- Telegram Session 加密保存，不提交到 Git，不通过普通 API 返回。
- 账号设备资料自动生成后固定保存，重连和服务重启不随机更换。
- 默认不批量下载媒体，只保存必要消息元数据和媒体引用。
- 先完成模块开发，再统一集成测试和部署验证。

## 目录

```text
tgqf/
  frontend/                 # Next.js 管理界面
  backend/
    cmd/api/                # API 入口
    internal/config/        # 环境配置
    internal/httpapi/       # HTTP 路由与中间件
    internal/telegram/      # gotd 客户端与账号设备资料（后续模块）
    internal/worker/        # Telegram 后台 Worker（后续模块）
  deploy/                   # Docker Compose / Nginx（后续模块）
  docs/                     # 架构、部署与运维文档
```

## 容量目标

首阶段以约 50 个在线账号为设计目标，后续通过真实负载测试确定 Worker 数量、CPU、内存和数据库配置。容量数字不是未经测试的性能保证。

## 项目名称

TG账号管理系统
