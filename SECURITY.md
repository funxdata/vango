# 安全策略

## 支持的版本

| 版本 | 支持 |
| --- | --- |
| 0.x | ✅ 接受安全修复 |

## 报告漏洞

请 **不要** 通过公开 issue 报告安全问题。请发送邮件至 security@funxdata.com，并尽量包含：

- 受影响的包与版本；
- 复现步骤或最小复现仓库；
- 影响面评估（读/写、权限边界、是否需要认证）。

我们会在 3 个工作日内确认，并在修复发布后于 CHANGELOG 中致谢（除非你希望匿名）。

## 安全相关的默认行为

- HTTP 服务默认设置 `ReadHeaderTimeout`（10s）与 `IdleTimeout`（120s），避免慢速攻击与空闲连接堆积；
  读写超时按需通过 `web.WithTimeouts` 开启。
- `App` 默认启用请求体上限 `BodyLimit`（4 MiB，可用 `vango.WithMaxBodyBytes` 调整）与
  `SecureHeaders` 安全响应头；`Context.BindJSON`、`BindQuery` 的 400 响应不包含底层解析错误。
- `errs.From` 不会把内部错误细节返回给客户端，原始错误只进入日志。
- 密码使用 PBKDF2-SHA256（21 万次迭代）加盐哈希，校验使用恒定时间比较；
  可接受的迭代次数有上限，防止被篡改的哈希串造成 CPU 耗尽。
- JWT 密钥至少 32 字节；校验使用恒定时间比较，并检查 `exp` / `nbf`；
  配置了 issuer / audience 时会一并校验，避免同一密钥在多个服务间复用令牌。
- `web.RateLimit` 以直连地址为键；部署在反向代理后需显式启用
  `web.RealIP(web.TrustedProxies()...)`，只有可信代理的 `X-Forwarded-For` / `X-Real-IP` 才会被采信。
- `web.CORS` 不会把 `AllowOrigins: ["*"]` 与 `AllowCredentials: true` 组合成回显任意来源的行为；
  该组合视为无效配置，不返回跨域头。
- `httpx` 默认限制响应体读取为 16 MiB（`httpx.WithMaxResponseBytes` 可调），
  错误信息省略 URL 的 query，避免把 token 写入日志。
- `db.Update`、`db.Delete`、`db.UpdateMap` 拒绝空 where，避免误更新/误删全表；
  表名与列名会被校验，值一律使用占位符。
