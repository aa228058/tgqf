# 一对一 Telegram 语音通话实现记录

状态：进行中（技术路径已确认，尚未实现真实通话）

## 目标与不可降低的验收条件

本模块只实现用户明确授权、且 Telegram 隐私设置允许联系对象的一对一 Telegram 原生语音通话。语音消息（voice note）、文字邀请、群组语音聊天室均不满足本模块验收。

完成必须同时具备：
1. MTProto 呼叫生命周期：发起、来电、接听、拒绝、挂断、忙线、超时、断线恢复。
2. Telegram 要求的 DH 密钥协商、指纹校验及端到端媒体加密。
3. phone.sendSignalingData / updatePhoneCallSignalingData 双向信令转发。
4. ICE/STUN/TURN 或 Telegram 提供的呼叫连接建立，以及真实双向音频 RTP 收发。
5. 接听、拒绝、无应答、网络中断、主动挂断的状态和持久化结果。
6. 自动化测试覆盖协议状态机；真实双账号端到端测试通过后才能标记“已完成”。

## 当前依赖核查

项目当前固定依赖 github.com/mtgo-labs/mtgo v0.22.0（见 backend/go.mod）。

在该版本源码中确认：
- telegram/group_call.go 实现的是群组通话创建及群组直播流读取，不是一对一电话。
- telegram/handler_raw_update.go 提供 OnRawUpdate，可以接收 *tg.UpdatePhoneCallSignalingData。
- 生成的 tg TL schema/types 包含 PhoneCallProtocol、PhoneCallRequested、PhoneCallAccepted、PhoneCall、PhoneConnectionWebrtc、PhoneRequestCallRequest、PhoneAcceptCallRequest、PhoneConfirmCallRequest、PhoneDiscardCallRequest、PhoneSendSignalingDataRequest 等协议对象。
- MTGo 的高层 telegram 包没有发现一对一呼叫控制器或音频媒体引擎；其 go.mod 也没有 Pion WebRTC、Opus 或 tgcalls 媒体依赖。
- 仓库现有 SendVoice 仅用于发送语音消息，不是实时通话。

结论：MTGo 已具备足够的底层 MTProto 类型/RPC 和原始更新入口作为信令适配基础，但仅靠现有高层 API 无法完成真实通话。不得只调用原始呼叫 RPC 后就宣称通话可用；没有 WebRTC/音频媒体层仍然不完整。

## 实施路径

1. 保持 MTGo 为唯一 Telegram MTProto 客户端，不引入 gotd 或 Gogram 作为第二套账号连接栈。
2. 在 telegrammtgo 内新增一对一呼叫适配器：使用 MTGo 的 Raw() RPC、tg 类型和 OnRawUpdate，实现呼叫信令状态机。
3. 为媒体面单独接入经过审查的纯 Go WebRTC/Opus 实现；信令数据与媒体密钥不得写入普通日志或租户可读字段。
4. 先以可注入接口隔离 MTGo RPC、随机数/DH、媒体引擎，覆盖状态机和并发取消测试；再接入真实网络。
5. HTTP/任务层只接受 tenant/account 上下文，不允许客户端传入任意 account ID 越权控制；账号必须由当前租户授权。
6. 将当前呼叫状态和最终结果写入租户隔离的数据表/任务记录；进程重启后对未决呼叫采取安全结束/同步策略，不自动重拨。

## 明确禁止

- 不用 SendVoice 代替通话。
- 不向 Telegram 隐私设置禁止联系或未明确授权的对象发起呼叫。
- 不通过账号轮换、代理切换或并发呼叫绕过 Telegram 限制。
- 在真实双账号音频链路验收前，不将该模块标记为完成。

## 参考实现与协议资料

- MTGo v0.22.0：https://github.com/mtgo-labs/mtgo/tree/v0.22.0
- Telegram 一对一呼叫协议：https://core.telegram.org/api/calls
- Telegram 端到端语音/视频加密：https://core.telegram.org/api/end-to-end/video-calls
- gotd calls package（仅作为协议实现参考，不作为项目账号客户端）：https://pkg.go.dev/github.com/gotd/td/telegram/calls
- gortc（需单独核实 1:1 子包与信令适配，不可把群组通话能力误当作 1:1）：https://github.com/AmarnathCJD/gortc