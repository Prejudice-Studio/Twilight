# API Key 接入

API Key 用于让脚本或第三方系统在没有浏览器会话的情况下调用 Twilight 接口。

## 使用场景

- 自动化脚本读取用户或系统状态。
- 第三方服务提交受控请求。
- 管理员为特定集成创建可撤销凭据。

## 安全要求

- API Key 等同密码，不要写入日志、截图或公开仓库。
- 丢失或泄露后应立即禁用或删除。
- 所有公网调用必须使用 HTTPS。
- 优先使用权限范围更小的 Key，而不是共享管理员会话。

## 调用示例

```http
GET /api/v1/users/me/apikeys HTTP/1.1
Host: example.com
Accept: application/json
X-Twilight-API-Key: <key>
```

实际可调用接口取决于后端权限实现和 Key 的状态。
