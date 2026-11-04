# TG Manager 后端现状与需求差距审计 V1

> 审计基准：产品需求 `docs/PRODUCT_SPEC_V1.md`、前端方案 `docs/FRONTEND_PLAN_V1.md`。  
> 审计范围：当前 `main` 分支可读取的源码、路由、数据库迁移和最近 CI 工作流记录。  
> 重要声明：这是静态代码审计，不等于运行中的生产环境验收；“已存在”仅表示在代码中找到实现线索，仍需测试验证。

## 1. 总体结论

项目已有可复用的基础后端，不是空仓库；但当前实现明显处于基础账号管理与消息收件箱阶段，尚未达到产品 V1 的完整业务闭环。
- [x] 账号批量分组：新增租户隔离、事务原子性的批量设置分组接口 `PATCH /api/v1/accounts/group`，限制 1-100 个账号，校验 UUID、重复 ID 和分组长度；跨租户或不存在账号会整体回滚。
- [x] 账号批量代理绑定：新增 `PATCH /api/v1/accounts/proxy`，支持一次为 1-100 个本租户账号绑定或清除代理，使用事务原子提交并校验代理归属；任何账号越权/不存在时整批回滚。

已经找到的基础能力：
- Go API 与独立 Worker 进程。
- MTGo 普通用户账号客户端、二维码登录、Session 加密保存、重连调度。
- device-manager iOS 设备资料生成，并在账号创建/二维码登录预建账号时写入 `telegram_device_profiles`。
- 注册/登录/激活卡、租户/超级管理员基础能力和租户 RLS。
- 账号基础 CRUD、分组字段、代理 CRUD/绑定和 TCP 连通检测。
- 新消息过滤入库、消息列表、SSE 事件流、标记已处理、文字回复队列。
- 租户级聊天/群组观察数据。

关键阻塞和规格冲突：
1. **历史消息回填已移除**：Worker 不再定时调用 GetChatHistory；保留实时新消息事件。P0-04 的实时采集边界测试仍需补齐。
2. **设备资料持久化接入已完成核心实现**：Worker 启动/重连和 QR 登录均读取数据库 profile 并应用到 MTGo；旧账号缺失资料会幂等补齐，仍需真实账号重启/重连验收。
3. **全局公共群组总库第一阶段已实现**：public_groups 全局实体、来源与租户贡献统计、实时入库和共享目录查询已接入；资料纠错审核及人工合并审计也已实现。
4. **消息实时接口仍是 SSE，不是 WebSocket**；本轮改为 Redis Streams 持久事件流，支持 Last-Event-ID 断点续传及每租户有界保留，仍需 Redis 集成测试和 WebSocket 接口。
5. 未发现完整关键词规则引擎、命中后的私聊/群组自动动作编排、媒体动作、原生联系人卡片、真实语音通话链路。
6. 自动加群公开/私密邀请执行及审批状态同步已有代码闭环，仍需真实 Telegram 账号验收；账号模块现有二维码、手机验证码/2FA 单账号及批量登录，支持 JSON/CSV/TXT 和 Telethon/Pyrogram SQLite Session 导入、头像/姓名/简介/用户名批量资料任务；本轮补齐账号标签单条/批量管理及 CSV 导出。账号登录挑战仍是进程内短期状态，服务重启后需重新发起。
7. 前端目前仍需独立开发完整管理页面；本审计不修改前端。

## 2. 按模块核对

| 产品模块 | 当前代码证据 | 审计状态 | 主要差距 |
|---|---|---|---|
| 多租户/认证 | tenants、users、activation_cards、auth_sessions、RLS；auth handlers | 已有基础实现，待完整验收 | 多设备会话、密码变更撤销策略、到期停任务/恢复暂停语义需集成测试 |
| 超管/卡密 | admin handlers、activation_cards | 部分实现 | 租户生命周期完整操作、审计、续期暂停任务闭环需核验 |
| MTProto/MTGo | telegrammtgo Manager、独立 Worker | 已有基础实现 | MTGo 手机验证码/2FA 单账号及批量登录 API 已实现；OTP/2FA challenge 暂存于 Manager 进程内，10 分钟到期定时清理并断开未完成的临时客户端（QR token 可刷新但整个挑战最多 10 分钟），进程重启后需重新发起登录；Session 文件导入支持 JSON/CSV/TXT 及 Telethon/Pyrogram SQLite .session，仍需真实账号验收 |
| 二维码登录 | accounts QR start/poll/cancel 路由 | 已实现基础流程 | 需端到端测试过期、取消、重复登录、异常恢复 |
| Session 安全 | session_ciphertext 加密、Worker 内解密 | 已有基础实现 | 文件导入/导出策略和密钥轮换/备份恢复需补齐 |
| device-manager | telegram_device_profiles、RuntimeAccount.DeviceProfile、Manager.Start/StartQR | 核心实现已接入 | 持久化 profile 已用于 Worker 启动/重连和 QR 登录；仍需在线账号端到端验证 |
| 账号列表/分组/标签 | accounts GET/POST/DELETE、搜索/筛选/分页、CSV/Session 文件导入导出、分组/标签、批量分组/代理绑定/断开重连 | 后端功能已实现，待 CI 与真实账号验收 | 标签本轮新增 PostgreSQL TEXT[]、单账号/批量替换 API 和 CSV 导出；登录挑战进程重启恢复仍未实现 |
| 代理管理 | proxies CRUD、账号单/批量绑定、默认代理、协议认证检测、Telegram DC TCP 隧道检测、检测历史 | 后端功能已实现，待 CI/集成验收 | SOCKS4/5、HTTP/HTTPS 均执行协议协商/认证并尝试建立 Telegram DC TCP 隧道；检测结果及耗时持久化，Worker 每 10 分钟自动巡检并通过 PostgreSQL advisory lock 避免多 Worker 重复巡检；每代理保留最近 100 条结果，列表返回最近状态，支持历史查询；这不等于某个 MTGo 账号已完成 Telegram 授权连接 |
| 批量资料修改 | account_profile_update_jobs、MTGo UpdateProfile/UpdateProfilePhoto/UpdateUsername、资料任务 API 与 Worker | 后端功能已实现，待 CI/真实账号验收 | 支持姓名/简介/头像/随机用户名，逐账号结果、冲突重试、暂停/恢复/重试/取消；新增 `POST /api/v1/accounts/profile-updates/{id}/pause`，只允许暂停尚未领取的 pending 任务，避免把已发起的 Telegram RPC 误报为可中断；Worker 租约过期的 processing 任务转 paused 并标记结果不确定，需人工核验后恢复；用户名规则前端仍待后续前端阶段 |
| 消息收件箱 | unified_messages、筛选设置、列表、processed、reply jobs | 已有基础实现 | 媒体呈现/聊天上下文能力待核；需严格新消息语义 |
| 实时消息 | GET /api/v1/messages/events，text/event-stream | 部分实现 | 当前为可断点续传的 SSE + Redis Streams；仍未实现 WebSocket，且需 Redis 集成测试覆盖断线重连、游标续传和保留窗口 |
| 群组目录 | public_groups、public_group_sources、tenant_group_contributions、GET /api/v1/groups、CSV 导入导出、纠错审核/合并 | 核心后端闭环已实现，待最新 CI 与并发/多租户验收 | 全系统共享单一公共目录；实时采集跨租户汇总，来源账号/租户独立记录；CSV 导入现对已存在的全局群组也记录导入租户贡献，不覆盖现有 Telegram 元数据；超级管理员纠错审核与群合并会迁移来源/贡献/链接并留审计；仍需补齐 TXT/Excel 格式和真实多租户并发采集验收 |
| 链接采集器 | ExtractTelegramLinks、public_group_links、public_group_link_sources、GET /api/v1/group-links | 采集与私密邀请只读验证已接入，尚待运行验收 | 只处理实时新消息文本/说明；支持公开 username、`+invite`、`joinchat`；私密邀请码 AES-GCM 加密并以哈希去重；worker 每 15 分钟对有来源账号执行到期检查（同一链接至少间隔 6 小时），仅调用 `messages.checkChatInvite`，不加入群；结果区分 valid/approval_required/invalid/unknown，保存最近检查时间和错误；公开 username 已接入 `contacts.resolveUsername` 只读解析（不加入群）；公开/私密链接均由 worker 按 6 小时冷却刷新并记录结果；链接与群资料自动关联完善仍待补齐 |
| 自动加群 | tenant_join_tasks、tenant_join_task_accounts、Worker 租约执行器 | 公开用户名/公开链接/私密邀请链接执行及审批状态 Worker 轮询同步已实现代码闭环，仍需真实账号验收 | 支持明确授权、租户内账号分配、来源校验、任务状态机、过期租约回收；对 INVITE_REQUEST_SENT 记录待审批；FloodWait 向上取整并加 1 秒缓冲，同一任务及触发限流的账号进入冷却，不通过切换账号规避；Worker 领取子任务时同步返回递增后的 attempts，瞬时 CHANNELS_TOO_MUCH/USER_RESTRICTED 等错误进入有界退避，永久失效邀请/私有群等错误才终止；私密邀请审批在缺少可靠 chat_id 时保持 request_pending，不伪造审批结果 |
| 关键词监听 | tenant_keyword_rules、tenant_keyword_matches、规则 CRUD 与实时匹配 | 第一阶段已实现 | 支持 any/all/exact/regex、OR/AND 关键词分组、排除词、账号/聊天/媒体/发送者过滤、聊天类型和大小写；规则启停和命中分页 API 已提供 |
| 自动处理 | keyword_action_jobs、关键词命中自动入队及 Worker 执行 | 文本与原生联系人卡片动作已接入；租户媒体素材库基础 API 已实现 | 文本支持私聊/群组独立文案；联系人卡片支持私聊/群组，规则配置、租户持久化、命中入队、Worker 发送及任务结果查询已贯通；媒体素材支持租户隔离上传、列表、下载和删除，单文件上限 20 MiB、内容类型嗅探和 SHA-256 去重；MTGo 已增加基于受控字节内容的单条图片/视频/文件/音频/语音/圆形视频消息发送封装；素材尚未接入关键词动作队列，媒体动作配置、相册混合顺序发送和动作失败停止策略仍待实现 |
| 真实语音通话 | 未发现调用链路 | 待专项技术验证/尚未实现 | 呼叫信令、密钥交换、音频传输、接听/拒绝均需端到端验证 |
| 通用任务中心 | `GET /api/v1/tasks` 已聚合回复、关键词动作、资料修改、加群账号队列 | 第一阶段只读聚合已实现 | 统一任务控制 API 已支持消息回复暂停/恢复/重试/取消、关键词动作暂停/恢复/重试/取消、资料修改暂停/恢复/重试/取消、加群任务暂停/恢复/重试/取消（重试仅重新排队仍授权账号的失败子任务）；幂等键、断点恢复、统一任务生命周期和 WebSocket 推送仍待实现 |
| 日志/监控/备份 | 基础健康检查、结构化日志、audit_logs 表 | 部分实现 | 资源监控、Worker 心跳、代理异常看板、备份恢复流程需补齐 |
| 前端 | Next.js 工程基础 | 尚未形成完整产品 UI | 按 FRONTEND_PLAN_V1 逐页设计实现；当前阶段仍按约定不改前端 |

## 3. 必须优先处理的 P0/P1

### P0 — 规格冲突和数据正确性
- [x] P0-01 已完成：移除 Worker 定时历史消息回填调用，并删除 Manager.GetChatHistory 历史读取封装；保留实时消息事件处理。对应 CI 已通过。提交：`fc028fdf`、`eb31f9ec`。
- [x] P0-02 已完成核心闭环：账号运行和 QR 登录读取数据库持久化 profile 并传给 MTGo；Manager 不再自行生成。Worker 启动时使用 device-manager 按稳定 account ID 幂等补齐旧账号缺失/关键字段为空的 profile，完整记录不覆盖。增加 profile 选择校验测试。对应 backfill 提交 CI 已通过。
- [x] P0-03 已实现：Worker 每轮 reconcile 停止不再获授权租户的账号；租户禁用/到期时，待执行/处理中回复任务改为 `paused`，释放租约并保留数据；续费/重新启用不会自动恢复。增加显式 `POST /api/v1/message-replies/{id}/resume`，仅在租户有效、账号仍授权且任务处于 paused 时允许人工恢复。管理员暂停事务和续费事务均设置租户 RLS 上下文。相关 Go CI 已通过。
- P0-04 补充消息采集测试，证明仅实时事件进入收件箱；不回填历史。

### P1 — 基础能力补齐
- P1-01 代码已实现：MTGo 手机验证码/2FA 单账号及批量登录 API；challenge 在 Manager 内存保存并设置 10 分钟过期，超时自动拒绝并断开待登录客户端。批量 Session 文件导入支持 JSON/CSV/TXT 与 Telethon/Pyrogram SQLite `.session` 转换。进程重启后 challenge 不恢复，需重新发起登录；真实账号验收仍待完成。
- P1-02 完成设备 profile repository：读取、创建、校验、迁移补齐、测试重启/重连一致性。
- [x] P1-03：新增租户默认代理设置/清除 API；新建账号与 QR 登录账号自动继承默认代理；账号单独绑定及批量绑定可覆盖。SOCKS4/5、HTTP/HTTPS 检测已执行协议协商/认证并尝试建立 Telegram DC TCP 隧道；Worker 每 10 分钟自动巡检，PostgreSQL advisory lock 避免多 Worker 重复执行；最近状态/耗时及每代理最近 100 条检测历史已持久化。此检测不等同于 MTGo 账号完成 Telegram 授权连接；真实账号连接状态仍由账号运行状态体现。
- [x] P1-04 第一阶段：账号搜索/分页/CSV 导出、批量 Session 文件导入、分组、标签、批量代理绑定、批量断开/重连状态控制、批量手机号登录均已有代码实现；实际客户端停止/重连由 Worker reconcile 执行。待 Go CI、PostgreSQL 集成及真实账号验收。
- [x] P1-05 第一阶段：姓名/简介/头像/随机用户名资料修改已进入逐账号任务队列，Worker 调用 MTGo UpdateProfile、UploadFile/SetProfilePhoto、UpdateUsername，记录逐账号结果并遵守 FloodWait；租户停用/到期时暂停，恢复后需人工显式恢复。待 CI 与真实账号上传/资料修改验收。

### P1 — 全局公共群组总库
- [x] P1-06 第一阶段：新增 `public_groups`，按 Telegram chat type + chat ID 去重，维护全局资料。
- [x] P1-07 第一阶段：新增 `public_group_sources`、`tenant_group_contributions`，记录来源账号、发现租户、发现次数和时间。
- [x] P1-10 第二阶段：租户可提交群组标题、公开用户名、成员数和认证标记的纠错建议；超级管理员审核通过后才更新全局资料，拒绝/通过均记录审核人、时间和备注。
- [x] P1-11 第一阶段：超级管理员可人工合并同类型重复群组；迁移来源记录、租户贡献、公开链接关联和纠错历史，并保存合并快照与审计日志。合并不自动根据相似名称执行。
- P1-08 第一阶段已接入：Worker 从 MTGo `message.Chat.Permissions` 采集群组默认发言权限，并将当前账号的管理员/受限权限快照单独写入该租户的来源记录；两者分列保存；当前账号快照记录是否管理员/创建者及可用的管理员/受限权限字段，未获得的群组默认权限保持 SQL NULL/未知，不推断为允许。权限快照除实时消息更新外，已增加 Worker 离线定期刷新：按租户/账号来源独立领取 5 分钟租约，使用 MTGo GetChat 只读读取群组元数据，分别更新全局默认权限和该来源账号权限；失败按 30 分钟起步指数退避、最长 24 小时重试。已新增 PostgreSQL 集成测试覆盖多租户独立领取、租约过期重领、旧 Worker fencing、失败退避；仍需真实 Telegram 账号验收。
- P1-09 已将实时新群组/频道元数据写入全局公共库，`GET /api/v1/groups` 读取共享目录并提供来源数、贡献租户数；本轮增加稳定内部 ID、成员/来源/贡献筛选、认证状态筛选、多字段排序、群组默认权限快照、最多 10,000 条 CSV 导出；新增 insert-only CSV 总库导入，重复群组跳过且不覆盖已有资料。
- P1-10/P1-11 已增加纠错建议审核、人工合并和合并审计；审核只由超级管理员执行，租户建议不会直接写入全局资料。本轮增加 CSV 导入纠错建议（最多 1,000 行），所有修改仍进入待审核队列；导入不直接创建/篡改全局群组。

### P2 — 自动化业务闭环
- [x] P2-01 第一阶段：实时新消息链接识别与公共链接目录入库；私密邀请链接加密存储，不回溯历史。链接来源和待验证状态可查询。公开 username 已在群组元数据入库时自动关联，并通过迁移回填历史未关联链接；私密邀请不凭空推断群组实体，待获得可靠 chat ID 后再关联。公开/私密链接的 MTGo 只读验证已接入；PostgreSQL 集成测试覆盖公开/私密链接租约独占、过期重领及旧 Worker fencing，仍需真实账号验收。
- [x] P2-02：公开用户名/公开链接/加密私密邀请链接自动加入执行、租户内账号分配、明确授权、暂停/恢复/取消、租约回收、待审批状态、Worker 审批状态同步和 FloodWait 冷却已实现；仍需真实 Telegram 账号验收。
- [x] P2-03 第一阶段：关键词规则模型、any/all/regex、排除词、聊天类型筛选、实时新消息评估与命中记录；不自动发送、不读取历史。
- P2-04 私聊/群组动作链完全隔离；先实现安全、可审计的文本/媒体动作。
- P2-05 Telegram 原生联系人卡片能力验证。
- P2-06 真实语音通话专项 spike；未完成全链路前保持不可用状态。
- [x] P2-07 第一阶段：新增租户隔离的 `GET /api/v1/tasks`，统一分页查询消息回复、关键词动作、账号资料修改和逐账号加群任务，保留各业务队列作为唯一状态源；第一阶段已实现统一查询和按类型控制入口（各业务队列继续作为唯一状态源），并新增回复/关键词任务取消状态迁移及失败加群账号安全重试；幂等键、断点恢复、统一任务生命周期和 WebSocket 推送仍待实现。

### P3 — 运维和前端
- P3-01 Worker 心跳、资源/依赖监控、告警和运维 API。
- P3-02 加密 Session 备份恢复演练、审计日志完善。
- P3-03 按 FRONTEND_PLAN_V1 开发桌面/手机管理界面；前端不能使用假数据冒充 API。
- P3-04 集成、并发、安全、备份恢复和真实 Telegram 测试。

## 4. 当前 API 快照

已从路由确认：
- `GET/POST/DELETE /api/v1/accounts`（GET 支持 `q`、`group`、`status`、`limit`、`offset`）
- `GET /api/v1/accounts/export.csv`（复用账号筛选条件；CSV 字段公式注入防护；不导出 Session/代理密钥）
- `POST /api/v1/accounts/import`（单次最多 20 个 JSON Session 记录，逐条结果；Session 仍按账号独立加密保存）
- `POST /api/v1/accounts/import-file`（multipart 文件上传，支持 JSON/CSV/TXT；单文件最大 1 MiB、最多 20 条；Session 按账号独立加密，响应不包含 Session）
- `POST /api/v1/accounts/profile-updates`（每批 1–20 个已授权账号，可分别设置姓名/简介，并通过 `generate_username: true` 启用随机用户名；用户名冲突自动重新生成最多 8 次，返回统一 batch_id 和逐账号任务）
- `GET /api/v1/accounts/username-generation-settings`、`PUT /api/v1/accounts/username-generation-settings`（租户配置随机用户名前缀、随机长度和字符集；任务创建时保存规则快照）
- `PATCH /api/v1/accounts/{id}/tags`、`PATCH /api/v1/accounts/tags`（单账号/批量替换标签；每账号最多 20 个标签、单标签最多 32 字符；空数组清空；列表及 CSV 导出返回标签）
- `GET /api/v1/proxies/{id}/checks`（读取最近代理检测历史，最多 100 条；代理列表同时返回最近检测状态、耗时和时间）
- `PUT /api/v1/proxies/{id}` 支持 `clear_password: true` 显式清除已保存的代理密码；不传密码仍保持原密码。
- 数据库迁移 `0040_account_tags.sql`：`telegram_accounts.tags TEXT[]`，并建立 GIN 索引。
- 数据库迁移 `0041_proxy_check_history.sql`：新增租户隔离的代理检测历史表；手动及 Worker 每 10 分钟自动检测均记录协议认证/Telegram DC TCP 隧道结果、耗时和脱敏错误信息；每代理最多保留最近 100 条。
- 数据库迁移 `0042_public_group_permissions.sql`：分别保存全局群组默认权限与租户来源账号权限快照。`0043_private_invite_link_checks.sql`：保存私密邀请最近检查时间与错误信息。
- `GET /api/v1/accounts/profile-updates`（按 batch_id/status 分页查询逐账号执行结果）
- `POST /api/v1/accounts/profile-updates/{id}/retry`、`POST /api/v1/accounts/profile-updates/{id}/resume`、`POST /api/v1/accounts/profile-updates/{id}/cancel`（失败重试、暂停恢复；仅 pending/paused 可取消，正在执行的任务必须等待当前 Telegram RPC 结束）
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
- `GET /api/v1/groups/export.csv`（沿用群组筛选条件导出 CSV，最多 10,000 条）
- `POST /api/v1/groups/import.csv`（tenant_admin 导入 CSV 纠错建议，最多 1,000 行；只进入待审核队列）
- `POST /api/v1/groups/catalogue/import.csv`（tenant_admin 导入共享群组总库；最多 10,000 行；仅插入新 Telegram 群组标识，已存在记录只跳过、不覆盖；成功新增记录写入租户贡献统计）
- `GET /api/v1/groups` 支持 `chat_type`、`q`、`verified`、`min_members`、`max_members`、`min_sources`、`min_contributors`、`sort=recent|title|members|sources|contributors`；结果包含内部 `id` 与 Telegram `chat_id`、群组默认发言权限 JSON 和采集时间。
- `GET /api/v1/groups/{id}/sources`（仅返回当前租户贡献的发现来源、账号信息及该账号在该群的权限快照，不泄露其他租户的账号数据）
- `POST /api/v1/groups/{id}/corrections`、`GET /api/v1/groups/corrections`（租户提交/查询自身的群组资料纠错建议）
- `GET /api/v1/admin/group-corrections`、`POST /api/v1/admin/group-corrections/{id}/review`（超级管理员审核全局群组资料建议）
- `POST /api/v1/admin/groups/{id}/merge`（超级管理员将来源群组人工合并至目标群组；保留来源、贡献、链接关系和合并审计）
- `GET /api/v1/group-links`（共享链接目录；支持 `type`、`status` 筛选；私密邀请码不向 API 明文返回）
- `GET/POST /api/v1/keyword-rules`、`PUT/DELETE /api/v1/keyword-rules/{id}`（租户关键词规则管理）
- `GET /api/v1/keyword-rules/matches`（分页查询实时命中记录，可按 `rule_id` 筛选）
- `GET/POST /api/v1/join-tasks`、`GET /api/v1/join-tasks/{id}`（自动加群任务及账号分配状态）
- `POST /api/v1/join-tasks/{id}/{action}`（`start`、`pause`、`resume`、`cancel`；公开目标启动后由 Worker 调用 MTProto 加入）
- `GET/PUT /api/v1/group-links/settings`（分别启停私聊、群组、频道来源及总开关）
- `GET /api/v1/group-links` 对外返回 `last_checked_at`、`last_check_error`；公开 username 使用 `contacts.resolveUsername` 只读解析，私密邀请使用 `messages.checkChatInvite`；两者均由 worker 定期检查，临时网络错误/FloodWait 保留为 `unknown`，不会误标失效，也不会触发加群；链接再次被消息发现时会重置为 pending 并重新排队检查。

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


## 8. 2026-11-04 七项专项补齐进展（增量审计）

本轮已继续推进以下代码，注意区分“代码已接入”和“真实 Telegram 端到端验收”：

| 用户要求项 | 当前代码进展 | 当前验收结论 |
|---|---|---|
| 账号头像上传 | `POST /api/v1/accounts/profile-updates/photo`；multipart 1–5 MiB、JPEG/PNG 解码/尺寸校验；图片字节随逐账号资料任务持久化；MTGo UploadFile + SetProfilePhoto；Worker 按账号执行 | 后端实现已接入，需最新 CI 和真实账号上传验证 |
| 账号资料修改前端页面 | 按用户要求暂不开始前端 | 未开发，等七项专项结束后再做 |
| 用户名规则前端设置页面 | 按用户要求暂不开始前端；后端租户 GET/PUT 配置接口已存在 | 未开发，等七项专项结束后再做 |
| 私密邀请链接加入 | migration 0026 增加密文列；AES-GCM 加密；仅允许 t.me/telegram.me 邀请 hash；任务 Worker 解密后通过 MTGo JoinChat 执行；不向普通 API 回显明文 | 代码已接入，需 CI、数据库迁移和真实私密群测试 |
| 加群审批结果自动同步 | Worker 定期轮询待审批项；公开群通过 GetChatMember 判断；私密邀请通过 messages.checkChatInvite 判断已加入 | 已实现批准状态自动同步路径；私密邀请拒绝状态无法仅凭 checkChatInvite 明确区分，需进一步验证/补充可观测事件 |
| 真实 Telegram 语音通话 | MTGo v0.22.0 提供 PhoneRequestCall/AcceptCall/ConfirmCall 等原始 TL RPC，但当前库未提供一对一 VoIP 密钥协商和音频媒体传输引擎 | **未完成**。不能将呼叫信令或拨号按钮冒充真实通话；需要接入并验证兼容的 VoIP media engine 后才能启用 |
| 联系人原生卡片发送 | 增加 contact reply payload、migration 0027、`POST /api/v1/messages/{id}/contact`、队列 Worker 调用 MTGo SendContact | 原生联系人卡片代码已接入；需最新 CI 与真实 Telegram 收件端验收 |

新增迁移：`0025_account_profile_photo_jobs.sql`、`0026_private_invite_join_tasks.sql`、`0027_native_contact_reply_jobs.sql`。

新增/调整 API：
- `POST /api/v1/accounts/profile-updates/photo`（multipart：`account_ids` 为 JSON UUID 数组，`photo` 为 JPEG/PNG）
- `POST /api/v1/messages/{id}/contact`（JSON：`phone`、`first_name`、可选 `last_name`）
- `POST /api/v1/join-tasks` 增加 `private_invite_link` 字段；私密链接只接受明确授权的私密邀请 hash。

CI 修复记录：联系人发送选项参数与 MTGo v0.22.0 实际签名不一致，已按编译错误修正；之后对应工作流已出现成功结果。其余后续提交工作流仍需以各自 head SHA 的最终状态为准。

本轮仍不修改前端。语音通话继续保持不可用，不允许使用伪造音频或文字邀请替代真实通话。


## 9. 2026-11-04 代理检测能力补齐（增量）

- [x] 代理检测不再只测试代理主机 TCP 端口是否可达。
- [x] SOCKS5：协商无认证/用户名密码认证，验证认证结果，并请求建立到 Telegram 数据中心 TCP 端点 `149.154.167.51:443` 的隧道。
- [x] SOCKS4：验证 CONNECT 响应，并支持 SOCKS4a 域名目标。
- [x] HTTP/HTTPS：发送 CONNECT；配置用户名时发送 Basic Proxy-Authorization；HTTPS 代理先完成 TLS 握手并验证证书。
- [x] 从租户代理记录读取协议、用户名及加密密码；密码只在 Worker/API 服务端解密使用，不回显。
- [x] 检测接口成功响应明确标注 `proxy_auth_and_telegram_dc_tcp`，失败时不再误报为单纯 TCP 不可达。
- [x] 增加 SOCKS5 认证成功/失败、HTTP CONNECT 认证头/代理拒绝、SOCKS4 CONNECT 测试。
- CI：提交 `f0115a6392434492382711912cf185d55f18695d` 对应 Go backend checks 成功（test、vet、API build）。
- 验收边界：该检测证明代理协议协商/认证和 Telegram DC TCP 隧道可建立；**不等于 Telegram 账号 MTProto 登录成功**，账号最终连接状态仍以 MTGo Worker 的真实连接结果为准。


## 10. 2026-11-04 关键词精确匹配增量

- [x] 关键词匹配模式新增 `exact`，按整条消息文本进行精确相等匹配；不会把包含额外字符的文本误判为精确命中。
- [x] 保留规则的大小写设置：默认不区分大小写，开启后区分大小写。
- [x] 数据库约束通过迁移 `0028_keyword_exact_match_mode.sql` 同步支持 `exact`，避免应用层接受而数据库拒绝。
- [x] 增加精确匹配、附加字符拒绝、前后缀拒绝和大小写测试。
- CI：应用层与测试变更已触发 GitHub Actions，最终结果待完成；数据库迁移在后续 workflow 中验证。
- 当前范围：这只是关键词匹配能力的一项补齐；监听账号/群组过滤、消息类型与资料字段匹配、组合分组规则，以及命中后的规则化动作编排仍需继续开发，不能将关键词模块标记为完成。


## 11. 2026-11-04 关键词监听筛选增量

- [x] 规则支持限定 Telegram 账号（account_ids），创建/更新时校验账号确实属于当前租户。
- [x] 规则支持限定 chat_id（chat_ids），并验证 ID 非零、无重复。
- [x] 规则支持媒体类型过滤：photo、video、document、audio、voice、video_note、animation、sticker。
- [x] 过滤字段持久化到租户关键词规则表，并在实时新消息匹配时读取、应用；空数组表示不额外限制。
- [x] 增加账号、群组、媒体类型和非法过滤值的单元测试。
- [ ] GitHub Actions 测试、vet、API build 最终结果待最新提交对应 workflow 完成。
- 剩余：发送者用户名/昵称、群名等资料字段筛选；复杂 OR/AND 分组表达式；关键词命中后的私聊/群组动作执行链路。媒体类型目前是额外过滤条件，纯媒体消息（没有文本）不会单独触发关键词规则。


## 12. 2026-11-04 关键词媒体独立触发与创建参数修复

- [x] 修复关键词规则创建 SQL 的参数绑定：新增的 account_ids、chat_ids、media_types 三列此前已进入 INSERT 列表，但参数值未全部传入；现已补齐，创建与更新字段顺序一致。
- [x] 支持不依赖文本关键词的纯媒体规则：只配置 media_types 即可匹配指定媒体类型。
- [x] 纯媒体命中记录 matched_terms 为 `media:<type>`，便于审计区分媒体命中与文本命中。
- [x] 纯媒体规则仍受账号、chat_id、会话类型、出站消息过滤约束。
- [x] 增加纯媒体匹配、错误媒体类型、出站消息及空规则拒绝测试。
- [ ] 最新 GitHub Actions 仍在运行，最终测试/vet/build 结果待确认。
- 剩余：发送者资料字段匹配、群名/用户名过滤、复杂组合表达式和命中后自动动作执行。

 
## 13. 2026-11-04 关键词发送者与群组资料筛选

- [x] 规则增加 chat title、chat username、sender numeric ID、sender username、sender display name 筛选。
- [x] MTGo 0.22.0 的 `Message.Sender` 提供 username/first name/last name；worker 从真实更新事件映射到 Incoming。
- [x] 新增 JSONB migration 0030，并覆盖匹配/不匹配和非法 sender ID 测试。
- [ ] CI 与完整数据库 migration 验收待完成。
- 剩余：复杂 OR/AND 规则组和命中后的自动动作编排。

 
## 14. 2026-11-04 关键词命中自动回复队列

- [x] 关键词规则增加可选 `reply_text` 自动回复动作，限制 4096 字符。
- [x] 新增 `keyword_action_jobs` 持久化队列；仅新产生的唯一命中记录会创建一次动作任务，重复更新不会重复入队。
- [x] Worker 领取动作任务，通过既有 MTGo `SendReply` 发送，并保存 Telegram 消息 ID。
- [x] 支持租户停用时暂停动作、账号离线重试、普通错误退避重试、FloodWait 冷却和最多五次普通失败后终止。
- [x] 新增 migration 0031 与回复文本验证测试。
- [ ] CI、数据库迁移执行及真实 Telegram 测试待验收；目前仅完成静态实现与单元测试提交。
- 当前动作类型仅支持固定文本回复；OR/AND 条件组、媒体动作、联系人卡片动作及可视化动作编排仍未完成。

 
## 15. 2026-11-04 关键词 OR/AND 分组表达式

- [x] 规则支持 `keyword_groups`：每组 1-20 个关键词，最多 10 组。
- [x] `group_operator=any` 表示组间 OR；`group_operator=all` 表示组间 AND；每组内部沿用当前 match_mode。
- [x] 分组配置进入创建、更新、列表及实时消息评估 SQL。
- [x] 新增 migration 0032 与 OR/AND 组合、空分组、非法运算符测试。
- [ ] CI 和 migration 运行结果待确认；当前无真实租户数据库环境可执行迁移验收。
- 注：分组是简单两层结构（组间 AND/OR + 组内现有 match_mode），尚不是任意深度的布尔表达式树。

 
## 16. 2026-11-04 关键词自动动作任务管理 API

- [x] 新增 `GET /api/v1/keyword-actions`，支持状态筛选、分页，返回规则名、命中记录、账号/群组、回复内容、发送状态、尝试次数和错误信息。
- [x] 新增 `POST /api/v1/keyword-actions/{id}/pause`：暂停待处理或失败任务。
- [x] 新增 `POST /api/v1/keyword-actions/{id}/resume`：恢复暂停任务；仅有效租户可恢复。
- [x] 新增 `POST /api/v1/keyword-actions/{id}/retry`：重置失败次数并重新排队；仅有效租户可重试。
- [x] 列表及状态变更均限定当前租户；状态变更检查合法来源状态。
- [ ] CI 与数据库迁移执行验收待完成；目前未连接生产 Telegram 账号进行端到端验证。

 
## 17. 2026-11-04 私聊与群组关键词动作隔离

- [x] 新增 `reply_text_by_chat_type`，分别配置 `private` 与 `group` 自动回复内容；supergroup 归入 group。
- [x] 规则通用 `reply_text` 可作为默认回复，指定会话类型的配置优先覆盖。
- [x] channel 不自动创建回复动作，避免把私聊/群组回复逻辑错误套用到频道。
- [x] 新增 migration 0033、字段持久化及配置校验/动作选择测试。
- [ ] CI、数据库迁移及 Telegram 实际发送验证待完成。


## 18. 2026-11-04 账号级任务调度与资料任务取消（增量）

- [x] 普通消息回复、关键词自动回复、加群任务、账号资料更新领取任务时统一检查账号级发送时间窗口和冷却状态。
- [x] 领取条件互相排斥：同一 Telegram 账号存在有效 processing lease 时，其他任务队列不再同时领取，避免不同 Worker/队列并发操作同一账号。
- [x] 领取任务时持久化更新 `telegram_send_next_at`；FloodWait 同步写入账号级 `telegram_send_cooldown_until`，加群任务同时保留专用 join cooldown。
- [x] 加群任务和资料更新也纳入跨队列互斥，避免与消息回复、关键词动作同时调用 Telegram API。
- [x] 资料更新任务新增 `POST /api/v1/accounts/profile-updates/{id}/cancel`；仅允许取消 pending/paused，正在执行中的 Telegram RPC 不会被伪装成可立即中断。
- [x] 回复、关键词动作和资料更新任务的完成/失败/暂停/重排操作增加领取租约值校验，过期 Worker 无法覆盖新 Worker 的任务状态。
- [x] migration `0035_profile_update_cancellation.sql` 放开 cancelled 状态；列表状态筛选可查询 cancelled。
- [ ] 当前提交的 Go test、vet、API build 正在运行；需以最新 head SHA 的最终 CI 结果为准。
- [ ] 尚未在真实数据库验证迁移、跨队列并发锁竞争和多 Worker 场景；需后续集成测试。


## 20. 2026-11-04 Worker 心跳与异常退出监控（增量）

- [x] 心跳改为独立 goroutine 定时写入，不再依赖主任务循环结束；耗时 Telegram RPC 不会单纯因阻塞主循环而造成健康实例被误判 stale。
- [ ] 若数据库连续不可用超过 stale 阈值，实例仍会被标记 stale；这属于真实依赖故障信号，需结合日志/告警判断。

- [x] 新增 migration `0038_worker_heartbeat.sql`，持久化 worker instance、主机名、PID、启动时间、最近心跳和停止时间。
- [x] Worker 启动时登记实例，每 10 秒更新心跳；收到正常退出信号后标记 stopped。
- [x] 新增超级管理员接口 `GET /api/v1/admin/workers`，展示 Worker 实例、最近心跳及运行状态。
- [x] 心跳超过 45 秒未更新的 running 实例标记为 stale；正常 stopped 实例不误报 stale。
- [x] 增加 stale 判定单元测试。
- [ ] 需在最新 HEAD 统一执行 Go test、vet、API/Worker build，并验证 migration、异常杀进程和多实例部署；当前未声称运行验收通过。

## 21. 2026-11-04 回复任务幂等请求（增量）

- [x] 新增 migration `0039_reply_idempotency.sql`，为 `message_reply_jobs` 增加 `idempotency_key`、`request_hash`，并建立租户范围的部分唯一索引。
- [x] `POST /api/v1/messages/{id}/reply` 接受可选 `Idempotency-Key` 请求头；同租户同 key 的重复请求返回原任务，不再重复创建待发送任务。
- [x] 原生联系人卡片接口 `POST /api/v1/messages/{id}/contact` 使用同一机制，避免客户端超时重试时重复排入联系人卡片发送任务。
- [x] 同一个 key 如果用于不同消息或不同回复文本，返回冲突错误，不会静默复用旧任务。
- [x] 增加 key 格式与请求指纹单元测试。
- [ ] 仍需最新 HEAD 的 Go test/vet/build 和真实 PostgreSQL migration/并发重复请求集成测试。
- 边界：这是 API 入队幂等，不是 Telegram 外部发送 exactly-once。若 Telegram 已成功发送但 Worker 在持久化成功结果前崩溃，外部副作用仍可能重复；需要通过结果核对/人工审计等进一步降低风险，不能宣称绝对一次发送。



## 22. 2026-11-04 过期 Worker 的账号级副作用隔离（增量）

- [x] 修复加群任务 FloodWait 重排顺序：先以当前任务的精确 lease 条件更新 processing 任务；只有租约仍有效并成功消费后，才写入账号级 join/send cooldown。旧 Worker 即使在租约过期后返回 FloodWait，也不能再延长账号冷却时间。
- [x] 修复普通消息回复 FloodWait：先通过精确 lease 条件完成任务失败/重排状态迁移，再更新账号级 cooldown；失去租约的旧 Worker 不会改变账号调度状态。
- [x] 修复关键词自动回复 FloodWait：同样将账号级 cooldown 更新放在成功消费当前 lease 之后。
- [x] 修复账号资料更新 FloodWait：先校验并消费当前 profile job lease，再写入账号级 cooldown。
- [ ] 尚未执行 Go test/vet/build；按项目约定待后端模块整体完成后统一执行。
- [ ] 尚需 PostgreSQL 并发集成测试：模拟 Worker A 租约过期、Worker B 重新领取、Worker A 延迟返回 FloodWait，验证 A 不能改变任务或账号冷却状态。
- 说明：此修复保护数据库任务状态与账号调度状态，不可能撤销已经发出的 Telegram 外部请求；对外部发送仍需保留结果核对和重复风险说明。


## 23. 2026-11-04 Worker 批量领取与租约时长匹配

- [x] 修复 Worker 串行处理与批量领取不匹配：普通回复、关键词动作、加群子任务、资料更新均改为每轮每个领取入口只 claim 1 条，避免先领取多条并串行执行 45 秒 Telegram RPC，导致后续任务尚未执行就超过 2 分钟租约。
- [x] 领取后仍保留 DB 侧 lease 条件写回，失去租约的旧 Worker 不能覆盖新 Worker 的任务状态。
- [ ] 仍需压力测试：大量租户同时有任务时，按租户轮询的总耗时与任务公平性；若确认吞吐不足，应采用受控并发处理并为每个执行中任务增加独立 lease renewal，而不是重新扩大串行 claim 批量。
- [ ] 仍需验证 worker 停机、进程崩溃、网络分区及 PostgreSQL 慢查询时的恢复延迟。
- 未运行全量 Go test/vet/build；按项目约定留待后端整体完成后统一执行。


## 24. 2026-11-04 Worker 执行超时与租约窗口对齐（增量）

- [x] 普通回复队列已有 5 分钟 lease；单次 Telegram send 的 Worker 超时为 45 秒。
- [x] 关键词动作队列 lease 从 2 分钟延长至 3 分钟，覆盖 45 秒发送超时及数据库结果持久化余量。
- [x] 加群子任务 lease 从 90 秒延长至 3 分钟，覆盖 60 秒 JoinChat 超时及结果持久化余量。
- [x] 账号资料更新 lease 从 90 秒延长至 5 分钟。资料任务可能依次执行资料修改、头像上传、用户名更新（用户名冲突时最多 8 次尝试）；Worker 为整组资料/头像/用户名操作设置 45 秒共享 context deadline；5 分钟租约为该执行窗口及数据库写回留出余量。
- [ ] 当前仍采用领取时固定 lease，而非执行中续租；若将来延长 RPC 超时或允许多步骤任务耗时超过 5 分钟，必须引入带 fencing token 的 lease renewal，并确保旧 Worker 无法续租或写回。
- [ ] 仍需多 Worker/PostgreSQL 集成测试，覆盖 lease 到期、重领、慢 RPC 和任务完成写回；按项目约定暂不运行全量 Go test/vet/build。
- 本轮仅调整数据库 claim SQL 的 lease 时长，不改变 Telegram 发送频率、FloodWait 规则或每轮 claim=1 的串行安全策略。


## 25. 2026-11-04 批量手机号登录编排（增量）

- [x] 新增 POST /api/v1/accounts/phone/bulk/start：一次接收 1-20 个手机号，先整体校验数量、手机号格式、重复号码、分组和代理 JSON，再逐账号创建 pending 记录、读取持久化 device profile/代理配置并调用 MTGo StartPhoneLogin。
- [x] 批次采用逐账号部分成功语义：一个号码被 Telegram 拒绝或账号额度不足，不会抹掉其他已成功发出的 OTP challenge；响应逐项返回账号记录、发送状态、投递类型及重发等待秒数，不返回 Session/代理凭据。
- [x] 新增 POST /api/v1/accounts/phone/bulk/verify：逐项提交 account_id + OTP 或 2FA password；只允许二者恰好提供一个，调用 MTGo VerifyPhoneCode / VerifyPhonePassword，支持逐账号返回 two_factor_required，成功后复用加密 Session 持久化。
- [x] 新增 POST /api/v1/accounts/phone/bulk/cancel：只允许取消当前租户仍处于 pending 的账号；先取消 MTGo challenge，再删除对应 pending account，不会按传入 ID 跨租户操作。
- [x] 所有批量入口限制最多 20 项，拒绝重复手机号/账号 ID；OTP 与 2FA 密码不会回显，批量接口不接受或返回 Session。
- [x] 增加批量手机号、代理 JSON、重复账号、OTP/2FA 二选一等输入校验单测。
- [ ] 仍需最新 HEAD 统一 Go test/vet/API build；按项目约定暂不提前运行全量 CI。
- [ ] OTP challenge 与 MTGo pending client 仍只存在 API 进程内存，进程重启后必须重新发起登录；没有持久化 OTP、2FA 密码或 Telegram challenge。需真实号码、真实 Telegram、服务重启及账号额度场景验收。
- [ ] 当前每个账号仍使用现有单账号 verify/2FA API 兼容路径；批量接口是逐账号结果，不是 Telegram 端原子批处理。


## 26. 2026-11-04 账号删除与批量清理闭环（增量）

- [x] 新增 DELETE /api/v1/accounts/bulk：支持 1-100 个账号 ID，先校验 UUID/去重，再在单个租户 RLS 事务内锁定全部目标记录。
- [x] 批量删除采用全有或全无语义：只要有任意账号不存在或不属于当前租户，整个事务回滚，不会部分删除；删除行数与锁定行数不一致时回滚。
- [x] 删除提交后逐个停止 API 进程中由 MTGo Manager 管理的客户端，并返回数据库删除数量、客户端停止数量及清理错误数量。
- [x] 修复单账号 DELETE 原先依赖无分页 List 做归属判断的问题：现在由租户范围的数据库 DELETE 作为最终授权判断，避免列表实现变化造成误判；UUID 在执行前校验。
- [x] 增加批量删除 ID 数量、UUID、重复 ID 输入测试。
- [ ] API 进程内存客户端停止属于删除提交后的 best-effort 清理；若 Stop 返回错误，账号数据已删除，接口会明确返回清理错误/待清理状态。仍需验证 Worker 侧账号消失后的自动断开/重连行为及 DB cascade 覆盖的子表。
- [ ] 尚需 PostgreSQL 集成测试：跨租户 ID 混入批次时零删除、同一账号并发删除、外键级联、事务中断回滚。
- 按项目约定未运行全量 Go test/vet/build；仍待后端全部模块完成后统一 CI。

## 27. 2026-11-04 Telethon / Pyrogram SQLite Session 导入

- [x] `POST /api/v1/accounts/import-file` 接受 `.session` 上传；单文件大小仍限制为 1 MiB，临时文件由系统以私有权限创建并在处理后清理。
- [x] SQLite 通过与 MTGo storage 同源的 `modernc.org/sqlite` 纯 Go 驱动只读打开，并设置 `PRAGMA query_only=ON`；不会对用户上传数据库执行 schema 初始化或写入。
- [x] 校验 `sessions` 表结构，识别 Telethon 标准列 `dc_id/server_address/port/auth_key` 或 Pyrogram 列 `dc_id/api_id/test_mode/auth_key/user_id/is_bot`，且恰好一条 session 记录；校验 DC、API ID、用户 ID、IP、端口和 256 字节 auth key。
- [x] 使用 `session-converter v0.6.0` 的 `EncodeTelethon` / `EncodePyrogram` 分别生成标准 StringSession；MTGo `v0.22.0` 支持自动识别这两种格式，继续沿用现有账号创建、设备资料创建及 Session 加密存储流程。
- [x] Telethon / Pyrogram SQLite session 均不保存可用于账号管理的手机号，上传表单必须额外提交 `phone`，可选提交 `display_name`、`group_name`；不从 auth key 猜测手机号或伪造 MTGO1 元数据。
- [x] 增加 SQLite fixture 测试，覆盖 Telethon / Pyrogram schema 转换、非法数据库、多 session 行拒绝及 `.session` 文件识别。
- [x] JSON/CSV/TXT/SQLite 文件导入统一使用 `session-converter v0.6.0` 识别和校验 Session 字符串格式；无效或不支持的字符串逐条失败，不写入账号表。
- [ ] 当前实现 Telethon 与 Pyrogram SQLite `.session`；加密/损坏数据库、WAL sidecar 组合文件及其他库的 SQLite schema 尚未支持。
- [ ] 本流程只验证 SQLite 结构与 session 编码，不会在导入时自动连接 Telegram；账号授权有效性需由后续 MTGo 连接流程确认。
- [ ] 需要运行 Go test/build 并在真实 Telethon session 文件上验收；按项目约定暂不执行全量 CI，等待后端模块整体完成。

## 27. 2026-11-04 SQLite Session 导入安全加固（增量）

- [x] Telethon / Pyrogram SQLite .session 解析在读取 sessions 表前执行 PRAGMA quick_check(1)；数据库损坏、页结构异常或完整性检查失败时拒绝导入。
- [x] SQLite 仍以只读 URI mode=ro 打开并启用 PRAGMA query_only=ON，上传内容只写入权限受限的临时文件，完成后删除临时文件。
- [x] 当 schema 同时满足 Telethon 与 Pyrogram 特征时明确拒绝，不再按优先级猜测来源格式，避免错误解释 auth key / user metadata。
- [x] 新增 schema 歧义测试和输入字节不可变测试；已有测试覆盖 Telethon / Pyrogram 单行转换、重复行拒绝、损坏文件和超限文件。
- [ ] 仍需统一 Go test/vet/build 及真实 MTGo 连接验证；本轮按约定不提前运行全量 CI。
- [ ] 当前 SQLite 转换输出保持来源格式（Telethon 或 Pyrogram）供 MTGo Config.SessionString 自动识别；账号连接仍使用项目统一的 MTGo 客户端，不引入 gotd/td 运行时。


## 28. 2026-11-04 统一任务中心动作能力提示（增量）

- [x] 统一任务列表在每条任务上返回 `can_pause`、`can_resume`、`can_retry`、`can_cancel`，前端可按队列类型与当前状态显示真实可执行的动作，避免对 processing/sent/completed 等不可操作状态展示无效按钮。
- [x] 按各队列现有状态迁移规则分别计算能力：普通回复/关键词动作、账号资料更新、加群子任务使用不同的暂停/取消状态集合，不把各队列状态错误地合并成一套通用规则。
- [x] 新增纯状态矩阵测试，覆盖 pending、processing、failed、paused、sent、queued、request_pending 及未知任务类型。
- [ ] 能力字段只是 UI 提示，不替代服务端事务中的租户隔离和状态条件更新；状态可能在列表读取后发生变化，动作接口仍以数据库更新结果为准。
- [ ] 尚未执行 Go test/vet/build；按项目约定待后端功能整体完成后统一执行。


## 29. 2026-11-04 统一任务中心统计接口（增量）

- [x] 新增 GET /api/v1/tasks/summary，在租户 RLS 事务中聚合普通回复、关键词动作、账号资料更新和加群子任务四类现有队列。
- [x] 返回总任务数、processing 数量、租约已过期的 processing 数量，以及按 task type/status 分组的计数；带有统计生成时间。
- [x] 统计直接读取各队列表，不创建第二份任务状态，也不改变 Worker 的 claim/lease/重试行为。
- [ ] 尚未运行 Go test/vet/build 或 PostgreSQL 集成测试；需在最终统一验收时验证迁移后 schema、RLS 和大队列统计查询性能。


## 30. 2026-11-04 统一任务中心批量控制接口

- [x] 新增 POST /api/v1/tasks/bulk/{type}/{action}，支持 message_reply、keyword_action、profile_update、group_join 四类任务的 pause/resume/retry/cancel 批量动作。
- [x] 请求在执行前整体校验任务类型、动作、ID格式、重复ID及数量（1-100）；普通队列使用 UUID，加群队列使用正整数子任务行 ID。
- [x] 每条任务复用既有 ChangeUnifiedTaskStatus 服务端状态条件与租户事务，不绕过队列控制逻辑；响应逐条返回成功/失败及原因，部分成功使用 HTTP 207。
- [ ] 批量控制采用逐项独立事务，明确为部分成功语义，不承诺全有或全无；需要 UI 对失败项展示原因，并允许用户重新提交失败项。
- [ ] 尚未运行 Go test/vet/build 或 PostgreSQL 集成测试；最终统一验收需覆盖混合有效/无效状态、跨租户 ID、并发 Worker claim 与批量取消竞争。


## 31. 2026-11-04 任务中心批量控制与加群子任务状态一致性（增量）

- [x] 统一任务中心的 group_join 行按所选账号子任务执行 pause/resume/retry/cancel，不再把单行操作错误升级为整个父任务操作；同一父任务的多个子任务可以独立批量操作。
- [x] 加群子任务控制使用租户范围的行更新；暂停/取消 processing 子任务时清空 lease，旧 Worker 的完成/失败写回因精确 lease 不匹配而失效。
- [x] 恢复/重试加群子任务时同时验证租户有效、父任务保留明确授权记录、Telegram 账号仍授权且 Session 存在；重试仅重置被选中的失败子任务。
- [x] 父任务状态从子任务集合重新推导：queued/processing/request_pending 优先保持 queued；无可运行项但存在 paused 时为 paused；否则按 failed、全 cancelled、completed 收敛。未知未来状态按活动状态保守处理。
- [x] 加群父任务行锁统一先于子任务行锁：Worker claim、完成/失败/FloodWait 重排、审批同步、单项控制及父任务重试遵循相同锁顺序，降低并发控制与 Worker 写回之间的死锁风险。
- [x] 增加父子状态聚合单元测试，覆盖混合状态、待审批、失败、全取消和未知状态。
- [x] 保留现有批量接口的逐项独立事务与部分成功语义；不承诺整批原子提交。
- [ ] 本次变更尚未运行 Go test、vet、build 或 GitHub Actions；必须在最终后端验收时执行。
- [ ] 尚需 PostgreSQL 多连接集成测试覆盖：两个 Worker 同时 claim、租约到期后重领、旧 Worker 延迟写回、跨租户 ID 混入批次、并发父子状态变化，以及批量请求部分失败。
- [ ] Worker 当前通过各队列 claim SQL 直接重新领取过期 processing 租约，无独立定时 reaper；该设计避免额外状态迁移，但恢复时延取决于 Worker 轮询周期。执行中固定租约仍需继续遵守当前 RPC deadline 上限。


## 32. 2026-11-04 群链接采集器与自动加群模块（整合完善批次）

- [x] 群链接采集继续按租户、发现账号、来源会话和消息记录归集；私密邀请码只保存 SHA-256 去重值及 AES-GCM 加密后的原始 hash。
- [x] 公开链接和私密邀请链接均通过 MTGo 只读 messages.checkChatInvite / contacts.resolveUsername 检查；状态区分 valid、approval_required、invalid、unknown。检查路径不调用加入 API。
- [x] Worker 每 15 分钟调度检查；同一链接正常检查后至少 6 小时才进入下一次常规检查。unknown 采用 15 分钟起步的指数退避，最高 6 小时；Telegram FloodWait 优先延后到服务端等待窗口（至少 15 分钟）。检查领取使用 6 分钟数据库租约和 SKIP LOCKED，多 Worker 不会同时领取同一链接；写回使用租约时间 fencing，过期 Worker 无法覆盖新检查结果。
- [x] 新消息再次发现链接时：若旧检查租约仍有效，只更新来源最近发现时间，不打断正在执行的检查、不清空重试状态；若租约不存在或已过期，则清空旧检查状态并重新排队。公开链接和私密邀请链接均遵守该规则。
- [x] 新增 migration 0044/0045/0046：链接检查次数、下次检查时间、到期索引、带 fencing 的多 Worker 链接检查租约，以及加群审批状态轮询租约；并将历史上已采集但未关联群组的公开链接按 username 回填到公共群组库。
- [x] 新增 GET /api/v1/group-links/stats，返回当前租户可见链接的总数、公开/私密数量及状态分布；列表同时返回检查次数和下次检查时间，支持按 type/status 筛选与分页。
- [x] 修复 GET /api/v1/group-links 的租户隔离：列表、总数及来源计数均限定当前租户的来源记录，避免暴露其他租户发现的链接和来源数量。
- [x] 自动加群仅由明确创建并确认授权的 tenant_join_tasks 执行；支持公共 username/link 与私密邀请链接。MTGo Client.JoinChat 对私密邀请内部调用 messages.importChatInvite，该调用只在实际加群任务执行路径发生。
- [x] 加群临时错误按 30 秒起步指数退避，最高 30 分钟；最多 5 次执行尝试后转失败。明确失效链接、禁入、权限拒绝等永久错误直接失败；FloodWait 使用 Telegram 指定等待时间。`INVITE_REQUEST_SENT` 进入 request_pending，`USER_ALREADY_PARTICIPANT` 记为 already_member，不误记失败；可能临时的 USER_RESTRICTED 保留重试。
- [x] 保留账号级任务互斥、每次 claim 单条、3 分钟 lease、租户有效期检查、账号 Session 授权检查、租户来源校验和审批状态轮询。
- [x] 暂停/取消不再撤销正在执行的 Telegram RPC 租约：停止后续 claim，但允许当前 RPC 返回后写回真实加入结果；若返回可重试错误，则根据父任务 paused/cancelled 状态转为暂停/取消，不会重新排队。
- [x] FloodWait 只延迟尚未领取的队列项，不再清空其他正在执行任务的 processing lease；账号级 FloodWait 冷却仍全局生效。新增 PostgreSQL 集成测试覆盖暂停成功写回、取消后不重试、同账号其他 in-flight lease 保留。
- [x] 对于已有 Telegram chat ID 的公开群审批，通过 MTGo membership 查询同步为 joined；明确 left/banned 状态同步为 declined。审批状态轮询使用 2 分钟数据库租约、SKIP LOCKED 和 fencing，FloodWait 按 Telegram 指定时长延后。私密邀请审批不伪装成可自动查询：checkChatInvite 不提供足够信息判定申请已被管理员批准或拒绝，因此保留 request_pending，待后续接入可靠的成员状态事件/映射后再同步。邀请链接本身只返回待审批或有效状态时，不伪造拒绝结论。
- [x] 增加加群重试退避和永久/临时错误分类单元测试。
- [x] 新增 PostgreSQL 集成覆盖：公开/私密链接在有效检查租约期间被重复发现时保留租约与重试状态；租约过期后重新发现才清空旧状态并重新排队。
- [x] GitHub Actions 已通过：Go test ./...、vet、API/Worker build；PostgreSQL 服务集成测试覆盖共享群组 insert-only 导入、租户来源权限快照刷新、公开/私密链接租约与旧 Worker fencing。
- [ ] 仍需 PostgreSQL 多 Worker 集成测试覆盖加群审批状态轮询租约的过期重领和旧 Worker fencing；群链接检查租约及跨租户来源公平性已覆盖。
- [ ] 仍需真实 Telegram 测试账号验证公开加入、私密邀请加入、审批通过/拒绝、FloodWait、链接过期和账号断线恢复。真实 Telegram 端的外部加入动作无法由单元测试替代。
- [ ] 私密邀请 hash 本身无法在所有场景中可靠区分“管理员拒绝申请”与“申请仍在等待”；只有能够查询目标 chat membership 且 Telegram 返回明确 left/banned 时才标记 declined，其余情况保持 pending/unknown，避免误报。


## Keyword media actions

关键词规则已支持绑定租户媒体素材，并由 Worker 通过 MTGo 发送 photo/video/document/audio/voice/video_note 单素材回复。素材删除受规则/队列外键保护；混合相册和动作失败停止策略仍待实现。


## MTGo media album sender foundation

已新增 `Manager.SendMediaAlbumReply`：通过 MTGo 上传每个素材后调用原生 `SendMediaGroup` 一次提交相册；支持 2–10 个图片/视频混合项或纯文件项，保留素材顺序和逐项说明。上传任一项失败时不会提交相册；图片/视频与文件不混组。此方法尚未接入关键词规则和 Worker 队列，队列级相册动作及失败/重试状态记录仍待完成。


## Keyword media albums

关键词规则现支持 `action_type=media_album`，可按顺序绑定 2–10 个租户素材，支持图片/视频混合或纯文件相册。规则创建/修改会验证素材归属、重复素材、类型组合和单项说明长度；命中后把有序 JSONB 素材清单写入任务队列，Worker 逐项读取租户素材并调用 MTGo `SendMediaAlbumReply`。上传阶段任一素材失败不会提交相册；数据库触发器阻止删除仍被规则或任务引用的相册素材。任务记录当前保存 Telegram 返回的首条消息 ID，Telegram 相册整体结果按一个动作任务管理。尚未执行最终 CI/编译检查。


## 2026-11-04 关键词监听与动作队列闭环增量

- [x] 修复纯媒体关键词规则：即使 Telegram 更新同时带有 caption，也按媒体类型独立触发，不再要求消息文本为空。
- [x] Worker 领取关键词动作时校验账号仍为 authorized 且 Session 密文存在；失效账号不再反复领取、重试同一动作。
- [x] 租户订阅失效时只暂停 pending 和租约已过期的 processing 动作，不撤销仍在执行 Telegram RPC 的有效租约。
- [x] 处理动作执行期间租户到期：成功 RPC 仍记录 sent；失败 RPC 转 paused，不继续自动重试；FloodWait 仍保存账号级冷却。
- [x] 新增 PostgreSQL 集成测试覆盖授权账号领取、失效账号不领取、租户到期后的 in-flight 成功/失败结果，以及有效租约不被停用扫描清除。
- [x] 新增租户行锁串行化关键词规则创建/启用时的 100 条启用规则上限，避免并发请求超限。


## 2026-11-04 消息处理与人工回复队列闭环增量

- [x] 回复任务领取仅接受仍处于 authorized 且 Session 密文存在的账号；失效账号任务留在队列中等待账号恢复，不会反复领取后空转。
- [x] 租户停用/到期扫描只暂停 pending 和租约已过期的 processing 任务，不撤销仍在执行 Telegram RPC 的有效租约。
- [x] Telegram 发送成功后，即使租户恰在 RPC 期间到期，仍持久化真实 sent 结果；发送失败则暂停任务，不在失效租户下继续重试。
- [x] FloodWait 即使发生在租户到期边界，也保留账号级 Telegram 冷却，不能因暂停任务而丢失服务端等待要求。
- [x] 新增 PostgreSQL 集成测试覆盖账号授权筛选、失效租户 live lease 保护、RPC 成功/失败写回和 FloodWait 冷却。


## 2026-11-04 统一任务中心与加群子任务控制增量

- [x] 修复任务中心对单个 group_join 子任务执行 pause/cancel 时直接清空 processing 租约的问题。
- [x] 新增 migration 0054：`pause_requested` / `cancel_requested`。对正在执行的 Telegram RPC 只登记控制请求，不撤销租约；RPC 返回后根据实际结果和请求状态落库。
- [x] 父级加群任务 pause/cancel 同样保留 in-flight lease；暂停后恢复会清除未完成的 pause request。
- [x] Worker 处理重试/FloodWait/永久失败时消费 pause/cancel request；任务取消后不会因瞬时异常重新入队，暂停后不会继续自动重试。
- [x] Worker 崩溃后，控制请求对应的 processing 租约到期时自动转为 paused/cancelled，不会被当作普通任务重新领取。
- [x] 统一任务父级状态改为根据子任务状态派生，已取消父任务不会被子任务状态刷新重新激活。
- [x] 增加 PostgreSQL 集成测试覆盖单子任务暂停/取消、父任务暂停及 in-flight RPC 结果落库。
