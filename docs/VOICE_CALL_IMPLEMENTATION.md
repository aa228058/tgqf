# Telegram 原生语音来电弹出功能

状态：按“只发起 Telegram 原生呼叫并触发对方来电提示”的范围实现；仍待 CI 与真实 Telegram 双账号来电弹窗验收。

## 用户需求范围

监控回复规则命中后，由指定的普通 Telegram 用户账号向发来私聊消息的用户发起 Telegram 原生语音呼叫，让目标设备出现 Telegram 原生来电界面。

本阶段只负责拨出呼叫并触发对方 Telegram 原生来电提示。不开发独立通话产品、语音聊天室、自动接听、接听后的 DH 确认或音频媒体传输。系统只报告“Telegram 已接受呼叫请求/正在响铃”，不把它伪报为对方已接听或音频通话已建立。

## 已提交的后端实现

- MTGo v0.22.0 提供一对一呼叫 TL 类型、`phone.requestCall`、DH 配置 RPC 和原始更新处理能力。
- `backend/internal/telegrammtgo/voice_call.go` 新增 `Manager.RequestVoiceCall`：检查账号连接、要求目标用户已在该账号 peer cache 中且具有有效 access hash、从 Telegram 获取 DH 参数、生成一次性 DH 公钥，并按 Telegram 要求计算 `SHA-256(g_a)` 后调用 `phone.requestCall`。
- 呼叫请求只走拨出/响铃流程；不会自动接听，也不会协商或传输音频。
- 监控回复规则新增 `action_type=voice_call`。该动作只允许私聊规则；目标用户取自触发消息的发送者 ID，不允许从前端任意指定用户 ID。
- 新增迁移 `0062_keyword_voice_call_actions.sql`，为任务保存目标用户、Telegram call ID/access hash 和 ringing 状态。
- Worker 执行 voice_call 任务，发起 MTGo 原生呼叫，并将 Telegram 返回的 call ID 和 ringing 状态写入租户隔离任务记录。
- 关键词动作列表 API 返回 `target_user_id`、`telegram_call_id` 和 `voice_call_state`，不伪造 outgoing chat message。
- 新增规则校验单元测试，并扩展现有任务集成测试 fixture。

## 尚未完成的验收

1. CI 编译、Go tests 和 vet 必须通过；当前环境未能在本轮直接运行仓库测试。
2. 监听 `UpdatePhoneCall`，把对方拒绝、接听、结束、超时和失败同步到任务状态。
3. 真实 Telegram 双账号验证：目标设备确实出现原生来电界面，且 FloodWait/隐私限制错误被正确记录。
4. 确认对方接听后的产品处理策略。当前实现只完成呼叫请求/响铃，不完成后续 DH 确认和音频媒体链路；不能展示“通话成功”。

## 安全边界

- 只允许由租户已授权账号对当前收到私聊消息的发送者触发。
- 尊重 Telegram 隐私设置和平台限制，不通过账号轮换、代理切换或并发拨号规避限制。
- DH 随机材料不写普通日志。
- 未通过真实双账号来电弹窗测试前，不标记模块完成。
