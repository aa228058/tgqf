# Go Backend

TG账号管理系统 API 服务，基于 Go 标准库启动 HTTP 服务。Telegram 长连接和后台任务将放在独立 Worker 进程，不运行在 HTTP handler 中。

## 本地运行

```bash
cd backend
go run ./cmd/api
```

默认监听 `:8080`。可通过环境变量调整：

- `HTTP_ADDR`：HTTP 监听地址，默认 `:8080`
- `APP_ENV`：运行环境，默认 `development`

健康检查：`GET /health/live`。当前仅检查 API 进程存活；数据库、Redis 和 Worker 就绪检查将在对应组件接入后实现。

## 下一步

1. 接入 PostgreSQL、迁移和 Redis。
2. 建立多租户身份认证、授权期限和卡密模型。
3. 集成 gotd 普通用户账号客户端与加密 Session 存储。
4. 验证 iOS 设备资料生成逻辑并适配 gotd `DeviceConfig`。
5. 将 Telegram 账号连接放入独立 Worker 池。

不要将 Session、API Hash、密码或代理凭据提交到 Git。生产环境必须使用密钥管理和 HTTPS。
