# Go Backend

TG账号管理系统 API。HTTP API 与 Telegram 长连接 Worker 分离；长连接不会运行在 HTTP handler 中。

## 本地运行

需要 Go 1.23+。仅启动 API（不接数据库）：

```bash
cd backend
go run ./cmd/api
```

连接依赖时同时设置 `DATABASE_URL` 和 `REDIS_URL`，服务启动时会检查连接并执行内置 SQL 迁移：

```bash
export DATABASE_URL='postgres://user:password@localhost:5432/tgmanager?sslmode=disable'
export REDIS_URL='redis://localhost:6379/0'
go run ./cmd/api
```

配置项：`HTTP_ADDR`（默认 `:8080`）、`APP_ENV`（默认 `development`）、`DATABASE_URL`、`REDIS_URL`。健康检查：`GET /health/live`；依赖就绪检查：`GET /health/ready`。

## 数据安全

Telegram Session 和代理凭据只允许以加密密文存储；不得提交到 Git 或通过普通 API 返回。生产环境必须使用密钥管理、HTTPS 和受限数据库账户。租户数据查询必须始终带 tenant_id 约束；后续会在仓储层统一封装租户作用域，不能依赖前端传入 tenant_id。
