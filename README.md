# TG Manager

面向经过授权的 Telegram 普通用户账号的多租户 MTProto 管理平台，不是 Bot 管理后台。

## 项目正式文档

- [产品需求与技术基线 V1.0](docs/PRODUCT_SPEC_V1.md)
- [前端完整方案 V1.0](docs/FRONTEND_PLAN_V1.md)

以上文档是项目需求与设计基线。文档中的需求不代表功能已经实现；实现状态以代码、自动化测试和端到端验收为准。

## 技术架构

- Web：Next.js、React、TypeScript、Tailwind CSS、shadcn/ui
- Backend：Go
- Telegram MTProto：`github.com/mtgo-labs/mtgo`
- iOS device profile：`github.com/mtgo-labs/device-manager`
- Data：PostgreSQL、Redis
- Realtime：WebSocket
- Deployment：Ubuntu VPS、Docker、Nginx、HTTPS

Telegram 接入统一使用 MTGo，不使用 Bot API、Telethon 或旧版 gotd 路线。iOS 设备资料由 device-manager 自动生成，按账号稳定并持久化，再应用到 MTGo client 配置。

## 核心模块

1. 多租户、超级管理员、卡密授权与会话安全
2. Telegram 普通用户账号登录、Session 加密存储、代理和设备资料
3. 独立 Telegram Worker、连接健康监控和断线恢复
4. 全系统共享公共群组总库、群链接新消息采集、群组检测和自动加群
5. 关键词监听、规则编排、授权范围内的私聊/群组自动处理
6. 任务中心、实时进度、暂停/恢复/取消和审计日志
7. 系统监控、备份与运维设置

## 自动化边界

只处理明确授权且 Telegram 隐私设置允许的对象；必须尊重用户意愿和适用法律。自动联系必须提供停止联系/退订处理、去重、频率限制、每日上限、失败冷却和审计记录。严格遵守 Telegram FloodWait 和平台限制，不实现绕过限制或无差别骚扰。

V1 不开发独立群发模块、页面或独立群发任务。关键词命中后的规则化自动处理仍保留消息发送能力。所有采集和监听仅针对功能启用后的新消息，不扫描或回溯历史消息。

联系人卡片使用 Telegram 原生联系人消息能力。真实语音通话需要独立验证 MTProto 呼叫信令、密钥协商和音频传输完整链路；验证完成前不宣称可用。

## 开发原则

- HTTP API 与 Telegram Worker 分离；长连接不运行在 HTTP 请求处理器中。
- 每个 Telegram 账号独立 MTGo client/运行上下文。
- Telegram Session 和代理凭据加密保存，不提交到 Git，不通过普通 API 返回。
- 账号设备资料由 device-manager 生成，按账号持久化；重连和服务重启不随机更换。
- 公共群组总库全系统共享，按 Telegram 群组 ID 去重并归并来源。
- 所有任务具备状态记录、幂等控制和异常处理。
- 需求确认不等于功能实现完成；以测试和端到端验收为准。

## 目录

```text
tgqf/
  frontend/                 # Next.js 管理界面
  backend/
    cmd/api/                # API 入口
    cmd/worker/             # Telegram 后台 Worker
    internal/telegrammtgo/  # MTGo 客户端与 device-manager 设备资料
    internal/               # 业务模块、存储、认证等
  deploy/                   # Docker / Nginx 部署配置
  docs/                     # 产品需求、前端方案、架构与运维文档
```

## 项目状态

产品需求 V1.0 和前端方案 V1.0 已归档到 `docs/`。当前文档用于统一后续开发方向，不表示所有模块已经开发或测试通过。首阶段容量目标和生产配置需通过实际负载测试验证。
