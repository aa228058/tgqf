# Go Backend

TG账号管理系统 API。需要 Go 1.23+、PostgreSQL 和 Redis。设置 DATABASE_URL、REDIS_URL 后，在 backend 目录执行 go mod tidy，再运行 go run ./cmd/api。

## 超级管理员初始化
首次启动前设置 SUPER_ADMIN_USERNAME 和至少 16 字节的 SUPER_ADMIN_PASSWORD。数据库中没有超级管理员时，服务会创建一个；已有超级管理员时不会覆盖。初始化后建议从部署环境移除这两个环境变量。

## 认证接口
- POST /api/v1/auth/register：注册租户管理员，创建待激活租户。
- POST /api/v1/auth/login：签发随机 Bearer Token，数据库只保存令牌摘要。
- POST /api/v1/auth/activate：提交一次性卡密。
- GET /api/v1/auth/me：当前用户。
- POST /api/v1/auth/logout：退出当前设备。
- POST /api/v1/auth/password：修改密码并撤销其他设备会话。

## 超级管理员接口
所有接口均要求超级管理员 Bearer Token。
- POST /api/v1/admin/cards：批量生成卡密；请求字段 count、duration_days。明文卡密仅在创建响应中显示一次。
- GET /api/v1/admin/cards?limit=100：查看卡密状态，不返回明文。
- POST /api/v1/admin/cards/{id}/disable：禁用未使用卡密。
- GET /api/v1/admin/tenants?limit=100：查看租户。
- PATCH /api/v1/admin/tenants/{id}/status：设置 active 或 disabled。
- PATCH /api/v1/admin/tenants/{id}/account-limit：设置账号上限。
- PATCH /api/v1/admin/tenants/{id}/expiry：设置租户到期时间，字段 expires_at 使用 RFC3339。

禁用租户后业务请求会被订阅检查拦截。恢复租户不会自动恢复后台任务；后续 Worker 必须保持任务暂停，等待人工确认后恢复。

## 安全边界
Telegram Session 和代理凭据必须加密存储，不得通过普通 API 返回。生产环境必须使用 HTTPS 和密钥管理。业务查询必须从认证上下文取得 tenant_id，不能信任客户端传入。当前仍需补充限流、审计、租户数据访问层和自动化测试，不能直接视为公网生产就绪。