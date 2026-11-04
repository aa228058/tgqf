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
    return body.error || fallback;
  } catch (error) {
    return error instanceof Error ? error.message : fallback;
  }
}
