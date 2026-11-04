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
};
export function translateAPIError(message: string): string {
  const raw = message.trim();
  if (!raw) return "请求失败，请稍后重试。";
  if (API_ERROR_ZH[raw]) return API_ERROR_ZH[raw];
  const lower = raw.toLowerCase();
  if (lower.includes("failed to load")) return "数据加载失败，请刷新页面后重试。";
  if (lower.includes("invalid") && lower.includes("password")) return "密码格式不正确，请检查后重试。";
  if (lower.includes("already exists") || lower.includes("duplicate key")) return "该记录已存在，请修改后重试。";
  if (lower.includes("not found")) return "请求的数据不存在或已被删除。";
  if (lower.includes("forbidden") || lower.includes("permission denied")) return "没有执行此操作的权限，请联系管理员。";
  if (lower.includes("timeout") || lower.includes("deadline exceeded")) return "请求超时，请检查网络后重试。";
  if (lower.includes("connection refused") || lower.includes("networkerror") || lower.includes("failed to fetch")) return "网络连接失败，请检查网络或联系管理员。";
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
