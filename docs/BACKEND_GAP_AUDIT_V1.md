# TG Manager 后端现状与需求差距审计 V1

> 审计基准：产品需求 `docs/PRODUCT_SPEC_V1.md`、前端方案 `docs/FRONTEND_PLAN_V1.md`。  
> 审计范围：当前 `main` 分支可读取的源码、路由、数据库迁移和最近 CI 工作流记录。  
> 重要声明：这是静态代码审计，不等于运行中的生产环境验收；“已存在”仅表示在代码中找到实现线索，仍需测试验证。

## 1. 总体结论

项目已有可复用的基础后端，不是空仓库；但当前实现明显处于基础账号管理与消息收件箱阶段，尚未达到产品 V1 的完整业务闭环。

已经找到的基础能力：
- Go API 与独立 Worker 进程。
- MTGo 普通用户账号客户端、二维码登录、Session 加密保存、重连调度。
- device-manager iOS 设备资料生成，并在账号创建/二维码登录预建账号时写入 `telegram_device_profiles`。
- 注册/登录/激活卡、租户/超级管理员基础能力和租户 RLS。
- 账号基础 CRUD、分组字段、代理 CRUD/绑定和 TCP 连通检测。
- 新消息过滤入库、消息列表、SSE 事件流、标记已处理、文字回复队列。
- 租户级聊天/群组观察数据。

关键阻塞和规格冲突：
1. **历史消息回填正在运行**：Worker 每 5 分钟对已知聊天调用 GetChatHistory 并写入消息表，直接违反 V1 “只监听新消息、不扫描或回溯历史”。
2. **设备资料虽然落库，但 Worker 启动 MTGo client 时重新按 account ID 生成 profile，没有读取已持久化 profile**；这会忽略数据库中实际保存的资料/配置，不能满足“使用持久化 profile”的严格要求。
3. **群组目录仍是 tenant_message_chats 租户级表**，不是全系统共享公共群组总库；也没有全局群组实体、来源归并和租户贡献关系。
4. **消息实时接口是 SSE，不是 WebSocket**；当前有收件箱/文字回复，不等于通用任务中心。
5. 未发现完整关键词规则引擎、命中后的私聊/群组自动动作编排、媒体动作、原生联系人卡片、真实语音通话链路。
6. 自动加群已支持公开用户名/公开链接的 MTProto 执行，私密邀请链接、审批回调仍未完成；批量手机号登录、Session 文件批量导入、批量资料修改仍缺失。
7. 前端目前仍需独立开发完整管理页面；本审计不修改前端。

## 2. 按模块核对

| 产品模块 | 当前代码证据 | 审计状态 | 主要差距 |
|---|---|---|---|
| 多租户/认证 | tenants、users、activation_cards、auth_sessions、RLS；auth handlers | 已有基础实现，待完整验收 | 多设备会话、密码变更撤销策略、到期停任务/恢复暂停语义需集成测试 |
| 超管/卡密 | admin handlers、activation_cards | 部分实现 | 租户生命周期完整操作、审计、续期暂停任务闭环需核验 |
| MTProto/MTGo | telegrammtgo Manager、独立 Worker | 已有基础实现 | 手机验证码/2FA、Session 文件导入、批量登录等缺失 |
| 二维码登录 | accounts QR start/poll/cancel 路由 | 已实现基础流程 | 需端到端测试过期、取消、重复登录、异常恢复 |
| Session 安全 | session_ciphertext 加密、Worker 内解密 | 已有基础实现 | 文件导入/导出策略和密钥轮换/备份恢复需补齐 |
| device-manager | 创建账号时写入 telegram_device_profiles | 部分实现 | Manager.Start/StartQR 仍直接 DeviceProfile(accountID) 重新生成，未读取数据库 profile |
| 账号列表/分组 | accounts GET/POST/DELETE、group patch、服务端搜索/筛选/分页、CSV 导出、批量 Session 导入 | 第一阶段已实现 | 标签、批量选择、断开/重连 API 未形成完整集合；批量导入当前接收最多 20 条 JSON 记录，非文件上传 |
| 代理管理 | proxies CRUD、bind、check、tenant default proxy | 第一阶段已实现 | 默认代理可设置/清除，新建手机号账号和 QR 登录账号继承默认代理，单账号可覆盖；检测仍仅 TCP 可达，不代表代理认证或 Telegram 实际可连 |
| 批量资料修改 | 未发现相关路由/服务 | 未实现 | 头像、姓名、简介的单/批量修改和逐账号任务结果 |
| 消息收件箱 | unified_messages、筛选设置、列表、processed、reply jobs | 已有基础实现 | 媒体呈现/聊天上下文能力待核；需严格新消息语义 |
| 实时消息 | GET /api/v1/messages/events，text/event-stream | 部分实现 | 当前为 SSE；产品任务中心要求 WebSocket，不能把 SSE 误称 WebSocket |
| 群组目录 | public_groups、public_group_sources、tenant_group_contributions、GET /api/v1/groups | 第一阶段已实现 | 已全局去重、记录来源与贡献租户；多来源合并审核、权限字段仍待补齐 |
| 链接采集器 | ExtractTelegramLinks、public_group_links、public_group_link_sources、GET /api/v1/group-links | 第一阶段已实现 | 只处理实时新消息文本/说明；支持公开 username、`+invite`、`joinchat`；私密邀请码 AES-GCM 加密并以哈希去重；状态默认 pending；租户可分别开关私聊、群组、频道来源；在线验证与自动关联群组资料仍待补齐 |
| 自动加群 | tenant_join_tasks、tenant_join_task_accounts、Worker 租约执行器 | 公开用户名/公开链接执行已启用；私密邀请链接及审批回调未实现 | 支持明确授权、租户内账号分配、来源校验、任务状态机、过期租约回收；对 INVITE_REQUEST_SENT 记录待审批；FloodWait 向上取整并加 1 秒缓冲，同一任务及触发限流的账号进入冷却，不通过切换账号规避 |
| 关键词监听 | tenant_keyword_rules、tenant_keyword_matches、规则 CRUD 与实时匹配 | 第一阶段已实现 | 支持 any/all/regex、排除词、聊天类型、大小写；只记录命中，不触发自动动作；规则启停和命中分页 API 已提供 |
| 自动处理 | 当前只有人工触发的文字 reply job | 未实现完整功能 | 私聊/群组独立动作链、媒体、联系人卡片、失败停止规则 |
| 真实语音通话 | 未发现调用链路 | 待专项技术验证/尚未实现 | 呼叫信令、密钥交换、音频传输、接听/拒绝均需端到端验证 |
| 通用任务中心 | 未发现通用 task/job 模型和控制路由 | 未实现 | 状态机、暂停/恢复/取消、幂等、断点恢复、逐账号进度 |
| 日志/监控/备份 | 基础健康检查、结构化日志、audit_logs 表 | 部分实现 | 资源监控、Worker 心跳、代理异常看板、备份恢复流程需补齐 |
| 前端 | Next.js 工程基础 | 尚未形成完整产品 UI | 按 FRONTEND_PLAN_V1 逐页设计实现；当前阶段仍按约定不改前端 |

## 3. 必须优先处理的 P0/P1

### P0 — 规格冲突和数据正确性
- [x] P0-01 已完成：移除 Worker 定时历史消息回填调用，并删除 Manager.GetChatHistory 历史读取封装；保留实时消息事件处理。对应 CI 已通过。提交：`fc028fdf`、`eb31f9ec`。
- [x] P0-02 已完成核心闭环：账号运行和 QR 登录读取数据库持久化 profile 并传给 MTGo；Manager 不再自行生成。Worker 启动时使用 device-manager 按稳定 account ID 幂等补齐旧账号缺失/关键字段为空的 profile，完整记录不覆盖。增加 profile 选择校验测试。对应 backfill 提交 CI 已通过。
- [x] P0-03 已实现：Worker 每轮 reconcile 停止不再获授权租户的账号；租户禁用/到期时，待执行/处理中回复任务改为 `paused`，释放租约并保留数据；续费/重新启用不会自动恢复。增加显式 `POST /api/v1/message-replies/{id}/resume`，仅在租户有效、账号仍授权且任务处于 paused 时允许人工恢复。管理员暂停事务和续费事务均设置租户 RLS 上下文。相关 Go CI 已通过。
- P0-04 补充消息采集测试，证明仅实时事件进入收件箱；不回填历史。

### P1 — 基础能力补齐
- P1-01 完成账号登录方式：手机号验证码、2FA、Session 导入、批量导入/登录；每账号独立登录状态机。
- P1-02 完成设备 profile repository：读取、创建、校验、迁移补齐、测试重启/重连一致性。
- [x] P1-03 第一阶段：新增租户默认代理设置/清除 API；新建账号与 QR 登录账号自动继承默认代理；账号单独绑定代理仍可覆盖。检测目前只证明 TCP 可达，代理协议握手/认证和 Telegram 实际连通检测仍待补齐。
- P1-04 账号标签/搜索/分页/批量操作/导出/断开重连。
- P1-05 批量头像/姓名/简介修改接入统一任务执行底座。

### P1 — 全局公共群组总库
- [x] P1-06 第一阶段：新增 `public_groups`，按 Telegram chat type + chat ID 去重，维护全局资料。
- [x] P1-07 第一阶段：新增 `public_group_sources`、`tenant_group_contributions`，记录来源账号、发现租户、发现次数和时间。
- [x] P1-09 第二阶段：租户可提交群组标题、公开用户名、成员数和认证标记的纠错建议；超级管理员审核通过后才更新全局资料，拒绝/通过均记录审核人、时间和备注。
- [x] P1-10 第一阶段：超级管理员可人工合并同类型重复群组；迁移来源记录、租户贡献、公开链接关联和纠错历史，并保存合并快照与审计日志。合并不自动根据相似名称执行。
- P1-08 记录群组默认发言权限与账号个人权限为不同字段/事实来源（后续权限采集阶段）。
- P1-09 已将实时新群组/频道元数据写入全局公共库，`GET /api/v1/groups` 改为读取共享目录，并提供来源数、贡献租户数；多链接、人工纠错、合并审计仍待补齐。

### P2 — 自动化业务闭环
- [x] P2-01 第一阶段：实时新消息链接识别与公共链接目录入库；私密邀请链接加密存储，不回溯历史。链接来源和待验证状态可查询。实际 Telegram 验证及自动关联群组实体仍待补齐。
- [x] P2-02：公开用户名/公开链接自动加入执行、租户内账号分配、明确授权、暂停/恢复/取消、租约回收、待审批状态和 FloodWait 冷却已实现；私密邀请链接及审批回调仍待专项实现。
- [x] P2-03 第一阶段：关键词规则模型、any/all/regex、排除词、聊天类型筛选、实时新消息评估与命中记录；不自动发送、不读取历史。
- P2-04 私聊/群组动作链完全隔离；先实现安全、可审计的文本/媒体动作。
- P2-05 Telegram 原生联系人卡片能力验证。
- P2-06 真实语音通话专项 spike；未完成全链路前保持不可用状态。
- P2-07 通用任务中心：幂等键、租户/账号调度、租约、暂停/恢复/取消、重试、WebSocket 推送。

### P3 — 运维和前端
- P3-01 Worker 心跳、资源/依赖监控、告警和运维 API。
- P3-02 加密 Session 备份恢复演练、审计日志完善。
- P3-03 按 FRONTEND_PLAN_V1 开发桌面/手机管理界面；前端不能使用假数据冒充 API。
- P3-04 集成、并发、安全、备份恢复和真实 Telegram 测试。

## 4. 当前 API 快照

已从路由确认：
- `GET/POST/DELETE /api/v1/accounts`（GET 支持 `q`、`group`、`status`、`limit`、`offset`）
- `GET /api/v1/accounts/export.csv`（复用账号筛选条件；CSV 字段公式注入防护；不导出 Session/代理密钥）
- `POST /api/v1/accounts/import`（单次最多 20 个 Session 记录，逐条结果；Session 仍按账号独立加密保存）
- `PATCH /api/v1/accounts/{id}/group`
- `PATCH /api/v1/accounts/{id}/proxy`
- `GET/POST/PUT/DELETE /api/v1/proxies`
- `POST /api/v1/proxies/{id}/check`
- `PUT /api/v1/proxies/default`（设置默认代理；传空 `proxy_id` 清除）
- `POST /api/v1/accounts/qr/start`
- `GET /api/v1/accounts/{id}/qr/status`
- `DELETE /api/v1/accounts/{id}/qr`
- `GET /api/v1/messages`
- `GET /api/v1/messages/events`（SSE）
- `GET /api/v1/messages/chats`
- `GET/PUT /api/v1/messages/settings`
- `PATCH /api/v1/messages/{id}/processed`
- `POST /api/v1/messages/{id}/reply`
- `GET /api/v1/message-replies/{id}`
- `POST /api/v1/message-replies/{id}/resume`（人工审核后显式恢复 paused 回复任务）
- `GET /api/v1/groups`（全局共享公共群组/频道目录；返回来源数和贡献租户数）
- `GET /api/v1/groups/{id}/sources`（仅返回当前租户贡献的发现来源和账号信息，不泄露其他租户的账号数据）
- `POST /api/v1/groups/{id}/corrections`、`GET /api/v1/groups/corrections`（租户提交/查询自身的群组资料纠错建议）
- `GET /api/v1/admin/group-corrections`、`POST /api/v1/admin/group-corrections/{id}/review`（超级管理员审核全局群组资料建议）
- `POST /api/v1/admin/groups/{id}/merge`（超级管理员将来源群组人工合并至目标群组；保留来源、贡献、链接关系和合并审计）
- `GET /api/v1/group-links`（共享链接目录；支持 `type`、`status` 筛选；私密邀请码不向 API 明文返回）
- `GET/POST /api/v1/keyword-rules`、`PUT/DELETE /api/v1/keyword-rules/{id}`（租户关键词规则管理）
- `GET /api/v1/keyword-rules/matches`（分页查询实时命中记录，可按 `rule_id` 筛选）
- `GET/POST /api/v1/join-tasks`、`GET /api/v1/join-tasks/{id}`（自动加群任务及账号分配状态）
- `POST /api/v1/join-tasks/{id}/{action}`（`start`、`pause`、`resume`、`cancel`；公开目标启动后由 Worker 调用 MTProto 加入）
- `GET/PUT /api/v1/group-links/settings`（分别启停私聊、群组、频道来源及总开关）

## 5. CI 与代码状态

最近 Go 后端工作流：
- P0-01、P0-02、P0-03 相关实现的 Go 检查已通过。
- 全局群组目录和链接采集器第一阶段的 Go test、vet、API build 已通过（`f90a2966`）。
- 默认代理继承、QR 登录代理加载、链接来源开关和公开链接关联的 Go test、vet、API build 均已通过（`4c0169be` 及其前序工作流）。
- 曾出现链接目录查询代码拼接错误，已恢复干净服务文件并修复；`f90a2966` 对应完整检查通过。

这只能证明对应提交的 CI 检查通过，不证明未实现模块已完成，也不代表生产环境端到端验证通过。

## 6. 推荐实际执行顺序

1. 先消除 P0 历史回填冲突，并补测试。
2. 修复设备资料运行时读取持久化 profile。
3. 做租户授权状态与 Worker/任务生命周期闭环。
4. 建全局公共群组数据模型。
5. 补齐账号登录、Session 导入和代理默认配置。
6. 做链接采集、关键词监听、自动加群、自动处理。
7. 建统一任务中心和 WebSocket。
8. 完成监控、备份、前端和端到端验收。

## 7. 本审计的限制

- 未连接真实 Telegram 账号进行在线操作测试。
- 未对数据库实例执行迁移或查询。
- 未对部署环境、真实并发量、资源使用作推断。
- 语音通话和联系人卡片未作真实协议链路验证。
- 后续代码变更后需重新运行测试和更新本报告的状态。
