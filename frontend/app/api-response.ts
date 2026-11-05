const API_ERROR_ZH: Record<string, string> = {
  "invalid username or password": "用户名或密码错误，请检查后重试。",
  "account is disabled or subscription is inactive": "账号已停用或租户服务未启用，请联系系统管理员。",
  "selected tenant login requires an active tenant administrator account": "当前账号不是已启用的租户管理员。请切换到管理号登录，或先注册一个新租户。",
  "selected management login requires a system administrator account": "当前账号不是系统管理员，请切换到租户号登录。",
  "new tenant registration is disabled by system administrator": "系统暂未开放租户注册，请联系系统管理员。",
  "username already exists": "用户名已被使用，请更换一个用户名。",
  "username must be 3-64 characters, password 10-72 bytes, tenant name 1-120 characters": "填写不符合要求：用户名为 3–64 个字符，密码为 10–72 字节，租户名称为 1–120 个字符。",
  "invalid request body": "提交内容格式不正确，请检查填写内容。",
  "unauthorized": "登录状态已失效，请重新登录。",
  "super_admin_required": "此操作仅限系统管理员使用。",
  "subscription inactive; activate or renew your card": "租户服务尚未激活或已到期，请使用激活卡续期或联系管理员。",
  "activation card is invalid, used, or disabled": "激活卡无效、已使用或已停用，请核对激活码。",
  "rate limiter unavailable": "安全验证服务暂时不可用，请稍后重试。",
  "too many requests; try again later": "操作过于频繁，请稍后再试。",
  "expires_at is required": "请填写租户到期时间。",
  "value is required": "请填写要保存的配置值。",
  "failed to load worker status": "Worker 运行状态加载失败，请稍后刷新。",
  "failed to load audit logs": "审计日志加载失败，请稍后刷新。",
  "failed to load system configuration": "系统配置加载失败，请稍后刷新。",
  "invalid login type": "登录入口选择无效，请重新选择登录身份。",
  "new password must be 10-72 bytes": "新密码长度必须为 10–72 字节。",
};
export function translateAPIError(message: string): string {
  const raw = message.trim();
  if (!raw) return "请求失败，请稍后重试。";
  if (API_ERROR_ZH[raw]) return API_ERROR_ZH[raw];
  const lower = raw.toLowerCase();
  if (lower.includes("failed to load") || lower.includes("load failed")) return "数据加载失败，请刷新页面后重试。";
  if (lower.includes("invalid") && lower.includes("password")) return "密码格式不正确，请检查后重试。";
  if (lower.includes("already exists") || lower.includes("duplicate key") || lower.includes("unique constraint")) return "该记录已存在，请修改后重试。";
  if (lower.includes("not found") || lower.includes("no rows in result set")) return "请求的数据不存在或已被删除，请刷新后重试。";
  if (lower.includes("forbidden") || lower.includes("permission denied") || lower.includes("not authorized")) return "没有执行此操作的权限，请联系管理员。";
  if (lower.includes("timeout") || lower.includes("deadline exceeded") || lower.includes("context canceled")) return "请求超时或已取消，请检查网络后重试。";
  if (lower.includes("connection refused") || lower.includes("networkerror") || lower.includes("failed to fetch") || lower.includes("network request failed")) return "网络连接失败，请检查网络或服务器状态。";
  if (lower.includes("too many") || lower.includes("rate limit")) return "操作过于频繁，请稍后再试。";
  if (lower.includes("flood_wait") || lower.includes("flood wait") || lower.includes("floodwait") || lower.includes("slowmode_wait")) return "Telegram 暂时限制了此操作，请等待冷却结束后自动重试。";
  if (lower.includes("user_privacy_restricted")) return "对方的隐私设置不允许此操作，无法向该用户发起联系或邀请。";
  if (lower.includes("user_not_mutual_contact")) return "该用户仅允许互为联系人的账号联系，请先确认联系人关系。";
  if (lower.includes("phone_number_invalid")) return "手机号格式无效，请检查国家区号和号码。";
  if (lower.includes("phone_code_invalid") || lower.includes("phone_code_expired")) return "Telegram 验证码错误或已过期，请重新获取验证码。";
  if (lower.includes("session_password_needed")) return "该账号已开启二步验证，请输入正确的二步验证密码。";
  if (lower.includes("auth_key_unregistered") || lower.includes("session_revoked")) return "账号登录会话已失效，请重新登录授权。";
  if (lower.includes("user_deactivated_ban")) return "该 Telegram 账号已被停用或封禁，请检查账号状态。";
  if (lower.includes("user_banned_in_channel")) return "该账号已被此群组或频道限制，无法加入。";
  if (lower.includes("invite_request_sent")) return "已提交入群申请，等待群管理员审批。";
  if (lower.includes("user_already_participant")) return "该账号已经在目标群组中，无需重复加入。";
  if (lower.includes("chat_write_forbidden")) return "当前账号没有在此聊天中发送消息的权限。";
  if (lower.includes("channel_private") || lower.includes("invite_hash_invalid")) return "私密群组邀请链接无效或已失效，请更新邀请链接。";
  if (lower.includes("chat_admin_required")) return "此操作需要群管理员权限，当前账号权限不足。";
  if (lower.includes("peer_id_invalid")) return "Telegram 目标账号或群组标识无效，请刷新目标信息后重试。";
  if (lower.includes("phone code")) return "验证码不正确或已过期，请重新获取验证码。";
  if (lower.includes("password hash") || lower.includes("two-factor") || lower.includes("2fa")) return "二步验证密码不正确，请检查后重试。";
  if (/[a-z]{3,}/i.test(raw) && !/[\u3400-\u9fff]/.test(raw)) return "操作未能完成，请稍后重试；如问题持续，请联系管理员并提供操作时间。";
  return raw;
}

export async function readAPIJSON<T>(response: Response): Promise<T> {
  const contentType = response.headers.get("content-type") || "";
  if (!contentType.toLowerCase().includes("json")) {
    const body = (await response.text()).trimStart();
    if (contentType.toLowerCase().includes("text/html") || body.startsWith("<")) {
      throw new Error("API 返回了 HTML 页面而不是 JSON。请检查 NEXT_PUBLIC_API_BASE_URL，以及 Nginx 是否将 /api/ 请求转发到后端 API。");
    }
    throw new Error("API 返回了非 JSON 响应，请检查 API 地址和反向代理配置。");
  }
  return response.json() as Promise<T>;
}

export async function apiErrorMessage(response: Response, fallback: string): Promise<string> {
  try {
    const body = await readAPIJSON<{ error?: string }>(response);
    return body.error ? translateAPIError(body.error) : fallback;
  } catch (error) {
    return error instanceof Error ? error.message : fallback;
  }
}


export type ToastKind = "success" | "error" | "info";
export function notifyUser(message: string, kind: ToastKind = "success") {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new CustomEvent("tgqf:toast", { detail: { message, kind } }));
}
