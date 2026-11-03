# TG Manager

独立部署的 Telegram 用户账号管理平台。面向经过授权的 Telegram 普通用户账号，使用 MTProto，不使用 Bot API。

## 技术栈

- Frontend: Next.js, React, TypeScript, Tailwind CSS, shadcn/ui
- Backend API: Python, FastAPI
- Telegram client: Telethon (async)
- Data: PostgreSQL, Redis
- Realtime: WebSocket
- Deployment: Docker Compose, Nginx, HTTPS

## 核心模块（规划）

1. 管理员登录与系统安全
2. Telegram 账号登录、Session 加密存储与账号分组
3. Worker 进程池、连接健康监控与断线恢复
4. 群组资料库、导入、分类与账号群组关联
5. 关键词监听、历史消息检索与命中记录
6. 任务中心、进度、暂停/恢复/取消与任务日志
7. 统计、审计日志、备份与系统设置

## 自动化边界

自动私聊、群内回复、联系人卡片和语音呼叫仅面向有明确授权且允许联系的对象启用。必须提供退订/停止联系、去重、频率限制、每日上限、失败冷却与审计记录。严格遵守 Telegram 的隐私设置、FloodWait 和平台限制，不实现绕过限制或无差别骚扰。

语音呼叫先进行独立技术验证；Telethon 本身不等于完整的一对一通话 SDK。只有验证 MTProto 信令、密钥协商和音频传输链路后，才将其列为可用功能。

## 开发原则

- 全新实现，不依赖旧版本代码。
- API 与 Telegram Worker 分离；Worker 独立进程和事件循环。
- Telegram Session 加密保存，不提交到 Git，不通过普通 API 返回。
- 默认不批量下载媒体，只保存必要消息元数据和媒体引用。
- 先完成模块开发，再统一进行集成测试和部署验证；避免频繁构建半成品。

## 目录结构（目标）

```text
tgqf/
  frontend/       # Next.js 管理界面
  backend/
    app/           # FastAPI
    workers/       # Telethon workers
    migrations/    # 数据库迁移
  deploy/          # Docker Compose / Nginx
  docs/            # 架构、部署与运维文档
```

## 初始容量目标

首阶段以约 50 个在线账号为设计目标，后续通过真实负载测试确定 Worker 数量、CPU、内存和数据库配置。容量数字不是未经测试的性能保证。

## 状态

项目初始化阶段。功能清单为开发规划，不代表已经实现。
