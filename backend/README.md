# Go Backend

TG账号管理系统 API。HTTP API 与 Telegram 长连接 Worker 分离。

## 本地运行

需要 Go 1.23+、PostgreSQL 和 Redis。设置 `DATABASE_URL`、`REDIS_URL` 后运行：

```bash
cd backend
go mod tidy
go run ./cmd/api
```

配置：`HTTP_ADDR`（默认 `:8080`）、`APP_ENV`、`DATABASE_URL`、`REDIS_URL`。健康检查：`GET /health/live`；依赖就绪检查：`GET /health/ready`。

## 认证 API

- `POST /api/v1/auth/register`：注册租户管理员账号，创建待激活租户。
- `POST /api/v1/auth/login`：登录并签发随机 Bearer Token（服务端仅保存 SHA-256 摘要）。
- `POST /api/v1/auth/activate`：登录后提交一次性卡密激活租户。
- `GET /api/v1/auth/me`：当前用户信息。
- `POST /api/v1/auth/logout`：撤销当前会话。
- `POST /api/v1/auth/password`：修改密码并撤销其他设备会话，当前会话保留。

注册后允许登录和激活卡密，但订阅激活前不能访问业务接口。卡密生成/超级管理员接口尚未实现，因此当前不能通过公开 API 自行创建卡密。

## 安全边界

Telegram Session 和代理凭据必须加密存储；不得提交到 Git 或通过普通 API 返回。生产环境必须使用密钥管理、HTTPS 和受限数据库账户。业务查询必须从认证上下文取得 tenant_id，不得信任客户端传入的 tenant_id。当前认证模块仍需配套限流、验证码/反滥用策略、审计记录和自动化测试后才能用于公网生产。
