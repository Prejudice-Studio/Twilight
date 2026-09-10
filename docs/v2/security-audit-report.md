# Twilight V2 安全审计报告

> **审计时间**: 2026-09-10
> **审计范围**: V2 代码库全部 API 路由、认证、权限、限流、数据验证
> **审计人员**: Claude (Opus 4.6)
> **当前状态**: 审计进行中

---

## 一、执行摘要

### 1.1 审计概览
- **审计文件数**: 195 个 API 文件
- **HTTP 处理器**: 524 个
- **路由总数**: 336 条（Public 31, User 110, Admin 182, API Key 13）
- **状态变更路由**: 待统计（POST/PUT/DELETE/PATCH）
- **限流调用**: 待统计

### 1.2 风险等级分类
- 🔴 **高风险（Critical）**: 需立即修复，可能导致数据泄露或权限绕过
- 🟠 **中风险（High）**: 需优先修复，可能导致业务逻辑错误或部分权限绕过
- 🟡 **低风险（Medium）**: 建议修复，可能导致信息泄露或体验问题
- 🟢 **信息（Low）**: 最佳实践建议，不影响安全

### 1.3 已发现问题汇总
待完成审计后填写

---

## 二、CSRF 保护审计

### 2.1 当前状态
**发现**: ❌ **缺少全局 CSRF 保护机制**

#### 代码审查
1. **ServeHTTP 入口** (`internal/api/app.go:718`)
   - ✅ 有全局 Rate Limit
   - ✅ 有 CORS 处理
   - ❌ **没有 CSRF Token 验证**

2. **状态变更路由**
   - 所有 POST/PUT/DELETE/PATCH 路由**未强制 CSRF 保护**
   - 依赖 SameSite Cookie（但未明确配置）

#### 当前保护措施
1. **Session Cookie 配置** (待确认)
   ```go
   // 需要确认 SetCookie 是否设置了 SameSite=Lax/Strict
   ```

2. **CORS 配置**
   ```go
   // app.go 有 CORS 处理，但未审查 OPTIONS 预检是否足够
   if a.applyCORS(lw, r) && r.Method == http.MethodOptions {
       lw.WriteHeader(http.StatusNoContent)
       return
   }
   ```

### 2.2 风险评估
🔴 **高风险**: CSRF 攻击风险

**攻击场景**:
1. 用户已登录 Twilight
2. 访问恶意网站
3. 恶意网站发起跨站请求：
   ```html
   <form action="https://twilight.example.com/api/v1/users/me/password" method="POST">
     <input name="old_password" value="user_old_pass">
     <input name="new_password" value="attacker_password">
   </form>
   <script>document.forms[0].submit();</script>
   ```
4. 用户密码被修改

**影响范围**:
- 密码修改
- Emby 绑定/解绑
- Telegram 绑定
- 用户资料修改
- 管理员批量操作
- 工单创建/回复
- 所有状态变更操作

### 2.3 修复建议

#### 方案 1: Double Submit Cookie（推荐）
```go
// 1. 登录时生成 CSRF Token
func (a *App) issueSessionCookies(w http.ResponseWriter, token string, expires time.Time) {
    // Session Cookie
    http.SetCookie(w, &http.Cookie{
        Name:     a.cfg().SessionCookie,
        Value:    token,
        Path:     "/",
        Expires:  expires,
        HttpOnly: true,
        Secure:   a.cfg().SecureCookie,
        SameSite: http.SameSiteLaxMode, // 关键
    })
    
    // CSRF Token Cookie (可被 JS 读取)
    csrfToken := generateCSRFToken()
    http.SetCookie(w, &http.Cookie{
        Name:     "twilight_csrf",
        Value:    csrfToken,
        Path:     "/",
        Expires:  expires,
        HttpOnly: false, // JS 需要读取
        Secure:   a.cfg().SecureCookie,
        SameSite: http.SameSiteLaxMode,
    })
}

// 2. ServeHTTP 中验证
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // ... 现有逻辑 ...
    
    // CSRF 保护（状态变更方法）
    if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
        if !a.verifyCSRF(r) {
            failWithCode(lw, http.StatusForbidden, ErrCSRFTokenInvalid, "CSRF 验证失败")
            return
        }
    }
    
    // ... 路由匹配 ...
}

// 3. 验证逻辑
func (a *App) verifyCSRF(r *http.Request) bool {
    // 读取 Cookie 中的 Token
    cookie, err := r.Cookie("twilight_csrf")
    if err != nil {
        return false
    }
    
    // 读取 Header 中的 Token (前端需要发送)
    headerToken := r.Header.Get("X-CSRF-Token")
    if headerToken == "" {
        return false
    }
    
    // 常量时间比较
    return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(headerToken)) == 1
}

func generateCSRFToken() string {
    b := make([]byte, 32)
    rand.Read(b)
    return base64.URLEncoding.EncodeToString(b)
}
```

#### 方案 2: SameSite=Strict + Referer Check（简化方案）
```go
// 1. 强制 SameSite=Strict
http.SetCookie(w, &http.Cookie{
    // ...
    SameSite: http.SameSiteStrictMode,
})

// 2. 验证 Origin/Referer
func (a *App) verifyOrigin(r *http.Request) bool {
    origin := r.Header.Get("Origin")
    if origin == "" {
        origin = r.Header.Get("Referer")
    }
    if origin == "" {
        return false
    }
    
    parsed, err := url.Parse(origin)
    if err != nil {
        return false
    }
    
    // 允许的域名
    allowedHosts := []string{
        r.Host,
        a.cfg().ServerDomain,
    }
    
    for _, host := range allowedHosts {
        if parsed.Host == host {
            return true
        }
    }
    return false
}
```

#### 前端适配
```typescript
// V2 SSR form action 自动处理
<form method="post" action="/api/v1/users/me/password">
  <input type="hidden" name="csrf_token" value={csrfToken}>
  <!-- ... -->
</form>

// V1 客户端 fetch
fetch('/api/v1/...', {
  method: 'POST',
  headers: {
    'X-CSRF-Token': getCookie('twilight_csrf'),
  },
  body: JSON.stringify(data)
})
```

---

## 三、Rate Limit 覆盖审计

### 3.1 当前实现分析

#### 已有限流机制
1. **全局限流** (`app.go:788`)
   ```go
   if !a.allowRate(r.Context(), rateKey("global:", clientIP), 
       a.cfg().RateLimitGlobalPerMinute, time.Minute) {
       failWithCode(lw, http.StatusTooManyRequests, ErrGlobalRateLimited, "请求过于频繁，请稍后再试")
       return
   }
   ```
   - ✅ 所有请求都经过全局限流
   - ✅ 按 IP 限制
   - ✅ Redis + 内存降级

2. **限流器实现** (`ratelimit.go`)
   - ✅ Redis 优先，失败降级内存
   - ✅ 容量上限 10,000 个桶
   - ✅ 降级告警节流（30s 一次）
   - ✅ 定期清理过期桶（5 分钟）

#### 限流配置
待确认以下配置项：
- `RateLimitGlobalPerMinute` - 全局限流（当前值？）
- 敏感操作是否有独立限流？

### 3.2 需要限流的敏感操作

#### 🔴 高优先级（必须独立限流）
1. **登录** (`/api/v1/auth/login`, `/api/v2/auth/login`)
   - 当前: 待确认
   - 建议: 5 次/分钟/IP, 10 次/分钟/账号
   
2. **注册** (`/api/v1/auth/register`, `/api/v2/auth/register`)
   - 当前: 待确认
   - 建议: 3 次/分钟/IP
   
3. **密码重置** (`/api/v1/auth/forgot-password/*`)
   - 当前: 待确认
   - 建议: 3 次/小时/IP, 5 次/小时/邮箱
   
4. **邮箱验证码发送** (`/api/v1/users/me/email/send-verification`)
   - 当前: 待确认
   - 建议: 3 次/小时/账号

5. **Telegram 绑定码生成** (`/api/v1/users/me/telegram/bind`)
   - 当前: 待确认
   - 建议: 5 次/小时/账号

#### 🟠 中优先级（建议独立限流）
6. **注册码检查** (`/api/v1/users/me/regcode/check`)
   - 当前: 待确认
   - 建议: 10 次/分钟/IP（防止爆破）

7. **工单创建** (`/api/v1/tickets`)
   - 当前: 待确认
   - 建议: 5 次/小时/账号

8. **媒体求片** (`/api/v1/media/request`)
   - 当前: 待确认
   - 建议: 10 次/小时/账号

9. **文件上传** (`/api/v1/users/me/avatar`, `/api/v1/users/me/background`)
   - 当前: 待确认
   - 建议: 10 次/小时/账号

#### 🟡 低优先级（可选）
10. **媒体搜索** (`/api/v1/media/search`)
    - 建议: 30 次/分钟/账号

### 3.3 审计方法
```bash
# 扫描所有限流调用
grep -rn "allowRate\|rateLimit" internal/api/*.go

# 查找敏感操作处理器
grep -rn "handleLogin\|handleRegister\|handleForgotPassword" internal/api/*.go
```

### 3.4 修复建议

#### 敏感操作限流模板
```go
// 登录限流
func (a *App) handleLogin(w http.ResponseWriter, r *http.Request, _ Params) {
    clientIP := a.clientIP(r)
    
    // 1. IP 级别限流（防止分布式爆破）
    if !a.allowRate(r.Context(), rateKey("login:ip:", clientIP), 5, time.Minute) {
        failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "登录请求过于频繁，请 1 分钟后重试")
        return
    }
    
    // 解析请求
    input := decodeLoginInput(r)
    
    // 2. 账号级别限流（防止针对性爆破）
    identifier := input.Username
    if identifier == "" {
        identifier = input.Email
    }
    if !a.allowRate(r.Context(), rateKey("login:account:", identifier), 10, time.Minute) {
        failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "该账号登录尝试过多，请 1 分钟后重试")
        return
    }
    
    // 3. 执行登录逻辑
    // ...
}

// 邮箱验证码发送限流
func (a *App) handleSendEmailVerification(w http.ResponseWriter, r *http.Request, _ Params) {
    p := a.current(r)
    
    // 账号级别限流
    if !a.allowRate(r.Context(), rateKey("email:send:", p.User.UID), 3, time.Hour) {
        failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "验证码发送过于频繁，请 1 小时后重试")
        return
    }
    
    // IP 级别限流（防止滥用）
    clientIP := a.clientIP(r)
    if !a.allowRate(r.Context(), rateKey("email:send:ip:", clientIP), 10, time.Hour) {
        failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "验证码发送过于频繁")
        return
    }
    
    // 发送验证码
    // ...
}
```

#### 配置化限流
```toml
# config.toml
[RateLimit]
global_per_minute = 60

# 认证相关
login_per_minute_ip = 5
login_per_minute_account = 10
register_per_minute_ip = 3
forgot_password_per_hour_ip = 3
forgot_password_per_hour_email = 5

# 邮箱相关
email_verification_per_hour_account = 3
email_verification_per_hour_ip = 10

# Telegram 相关
telegram_bind_per_hour_account = 5

# 业务相关
ticket_create_per_hour_account = 5
media_request_per_hour_account = 10
file_upload_per_hour_account = 10
regcode_check_per_minute_ip = 10
```

---

## 四、权限边界审计

### 4.1 认证机制分析

#### 认证级别
```go
const (
    AuthPublic  = 0  // 公开访问
    AuthUser    = 1  // 需要登录
    AuthAdmin   = 2  // 需要管理员
    AuthAPIKey  = 3  // API Key 访问
)
```

#### 认证流程
1. **用户认证** (`authenticateUser`)
   - ✅ Bearer Token 或 Cookie
   - ✅ Session 验证
   - ✅ 账号禁用检查
   - ⚠️ 待确认：旧 Session 是否自动失效

2. **管理员认证** (`authenticate`)
   - ✅ 先验证用户身份
   - ✅ 检查 Role == RoleAdmin
   - ✅ 最后一个管理员保护
   - ⚠️ 待确认：是否检查 `cfg.AdminUIDs` 和 `cfg.AdminUsernames`

3. **API Key 认证** (`authenticateAPIKey`)
   - ✅ X-API-Key Header
   - ✅ Authorization: Bearer/APIKey
   - ✅ Query 参数（受限场景）
   - ✅ 权限 Scope 检查

### 4.2 需要审计的权限点

#### 🔴 关键权限操作（必须严格检查）
1. **用户管理**
   - [ ] 修改其他用户资料（应禁止）
   - [ ] 查看其他用户敏感信息（应禁止）
   - [ ] 删除其他用户（仅管理员）
   - [ ] 修改用户角色（仅管理员）
   
2. **Emby 管理**
   - [ ] 绑定/解绑其他用户 Emby（应禁止）
   - [ ] 查看其他用户 Emby 凭据（应禁止）
   - [ ] 管理员强制绑定（仅管理员）
   
3. **工单系统**
   - [ ] 查看其他用户工单（应禁止）
   - [ ] 修改其他用户工单（应禁止）
   - [ ] 管理员查看所有工单（仅管理员）
   
4. **文件上传**
   - [ ] 覆盖其他用户文件（应禁止）
   - [ ] 读取其他用户文件（应禁止）

#### 审计方法
```bash
# 1. 查找所有用户自助操作
grep -rn "/users/me/" internal/api/routes.go

# 2. 查找 UID 参数解析
grep -rn "uidFromPathOrCurrent\|parseUID" internal/api/*.go

# 3. 查找资源归属检查
grep -rn "owner_uid\|OwnerUID\|uid.*!=" internal/api/*.go
```

### 4.3 IDOR 漏洞扫描

#### 典型 IDOR 场景
```go
// ❌ 危险：直接使用路径参数，未检查归属
func (a *App) handleGetTicket(w http.ResponseWriter, r *http.Request, p Params) {
    ticketID := parseInt64(p["ticketId"])
    ticket, exists := a.store().GetTicket(ticketID)
    if !exists {
        fail(w, http.StatusNotFound, "工单不存在")
        return
    }
    // ⚠️ 缺少归属检查！
    ok(w, "success", ticket)
}

// ✅ 安全：检查资源归属
func (a *App) handleGetTicket(w http.ResponseWriter, r *http.Request, p Params) {
    principal := a.current(r)
    ticketID := parseInt64(p["ticketId"])
    ticket, exists := a.store().GetTicket(ticketID)
    if !exists {
        fail(w, http.StatusNotFound, "工单不存在")
        return
    }
    
    // 归属检查
    if ticket.OwnerUID != principal.User.UID && principal.User.Role != store.RoleAdmin {
        fail(w, http.StatusForbidden, "无权访问此工单")
        return
    }
    
    ok(w, "success", ticket)
}
```

#### 审计清单
- [ ] `/api/v1/tickets/:ticketId` - 工单详情
- [ ] `/api/v1/users/me/apikeys/:keyId` - API Key 详情
- [ ] `/api/v1/users/me/background` - 背景图
- [ ] `/api/v1/users/me/avatar` - 头像
- [ ] `/api/v1/tickets/:ticketId/attachments/:filename` - 工单附件
- [ ] 所有带 `:id` / `:uid` 参数的路由

---

## 五、输入验证审计

### 5.1 当前验证机制

#### 已有验证
1. **用户名验证** (`internal/validate/validate.go`)
   - ✅ 长度限制
   - ✅ 字符限制
   - ✅ 保留词检查

2. **密码强度** (`ValidatePasswordStrength`)
   - ✅ 最小长度
   - ✅ 复杂度要求
   - ⚠️ 待确认：是否有最大长度限制

3. **邮箱格式** (`ValidateEmailFormat`)
   - ✅ 正则验证
   - ⚠️ 待确认：是否防止一次性邮箱

4. **JSON 解码** (`app.go`)
   - ✅ 大小限制
   - ✅ 嵌套深度限制
   - ✅ 单一值要求（拒绝多值）

### 5.2 需要验证的输入点

#### 🔴 高危输入（可能导致注入）
1. **SQL 注入**
   - ✅ 使用参数化查询
   - [ ] 审计所有 SQL 语句

2. **命令注入**
   - [ ] 文件名（上传、下载）
   - [ ] 系统命令调用

3. **路径穿越**
   - [ ] 文件上传路径
   - [ ] 文件读取路径
   - [ ] 备份恢复路径

#### 🟠 中危输入（可能导致业务逻辑错误）
4. **整数溢出**
   - [ ] Days 参数（续期天数）
   - [ ] ExpiredAt 计算
   - [ ] 积分/金额

5. **字符串长度**
   - [ ] 用户名（已验证）
   - [ ] 标题、内容
   - [ ] 备注、描述
   
6. **数组/对象大小**
   - [ ] 批量操作数量
   - [ ] 邀请列表
   - [ ] 附件数量

### 5.3 修复建议

#### 通用验证辅助函数
```go
// 验证文件名安全性
func validateSafeFilename(filename string) error {
    if filename == "" {
        return fmt.Errorf("文件名不能为空")
    }
    if len(filename) > 255 {
        return fmt.Errorf("文件名过长")
    }
    if strings.Contains(filename, "..") {
        return fmt.Errorf("文件名包含非法字符")
    }
    if strings.ContainsAny(filename, "/\\:*?\"<>|") {
        return fmt.Errorf("文件名包含非法字符")
    }
    return nil
}

// 验证整数范围
func validateIntRange(value int, min, max int, name string) error {
    if value < min || value > max {
        return fmt.Errorf("%s 必须在 %d 到 %d 之间", name, min, max)
    }
    return nil
}

// 验证字符串长度
func validateStringLength(value string, min, max int, name string) error {
    length := len([]rune(value))
    if length < min || length > max {
        return fmt.Errorf("%s 长度必须在 %d 到 %d 个字符之间", name, min, max)
    }
    return nil
}

// 验证数组大小
func validateArraySize(size int, max int, name string) error {
    if size > max {
        return fmt.Errorf("%s 数量不能超过 %d", name, max)
    }
    return nil
}
```

---

## 六、敏感信息泄露审计

### 6.1 日志脱敏

#### 当前实现
```go
// app.go:726
panicMsg := redactSensitiveText(fmt.Sprintf("%v", recovered))
```
- ✅ Panic 消息脱敏
- ⚠️ 待确认：`redactSensitiveText` 实现是否完整

#### 需要脱敏的字段
- password
- token (session, api_key, emby_token, bot_token, bgm_token)
- api_key
- secret
- credential
- private_key
- cookie
- authorization

### 6.2 错误消息

#### 安全错误消息原则
```go
// ❌ 泄露信息
fail(w, http.StatusNotFound, "用户 admin 不存在")  // 泄露用户名是否存在

// ✅ 通用消息
fail(w, http.StatusUnauthorized, "用户名或密码错误")  // 不区分用户名/密码哪个错
```

#### 审计清单
- [ ] 登录失败消息（是否区分用户名不存在 vs 密码错误）
- [ ] 注册失败消息（是否泄露已存在用户）
- [ ] 邮箱验证消息（是否泄露邮箱是否已注册）
- [ ] 密码重置消息（是否泄露账号是否存在）

### 6.3 API 响应

#### 敏感字段过滤
```go
// publicUser 函数应该过滤敏感字段
func publicUser(u store.User) map[string]any {
    return map[string]any{
        "uid": u.UID,
        "username": u.Username,
        "role": u.Role,
        "active": u.Active,
        "expired_at": u.ExpiredAt,
        // ❌ 不要返回
        // "password_hash": u.PasswordHash,
        // "emby_username": u.EmbyUsername, // 仅管理员可见
        // "telegram_id": u.TelegramID,      // 仅本人可见
    }
}
```

#### 审计方法
```bash
# 查找所有返回用户信息的地方
grep -rn "publicUser\|ok(w.*user" internal/api/*.go
```

---

## 七、会话管理审计

### 7.1 Session 生命周期

#### 当前实现
- ✅ Session 存储在 Redis/PostgreSQL
- ⚠️ 待确认：Session 过期时间
- ⚠️ 待确认：密码修改后是否撤销旧 Session
- ⚠️ 待确认：账号禁用后是否撤销 Session

#### 关键场景
1. **密码修改**
   ```go
   // ✅ 应该撤销所有旧 Session
   func (a *App) handleChangePassword(w http.ResponseWriter, r *http.Request, _ Params) {
       // 修改密码
       // ...
       
       // 撤销所有 Session
       a.sessions().RevokeAllForUser(r.Context(), p.User.UID)
       
       // 重新登录
       newToken, expires, _ := a.sessions().Create(r.Context(), p.User.UID)
       a.issueSessionCookies(w, newToken, expires)
   }
   ```

2. **账号禁用**
   ```go
   // ✅ 应该立即撤销 Session
   func (a *App) handleDisableUser(w http.ResponseWriter, r *http.Request, p Params) {
       uid := parseUID(p["uid"])
       a.store().UpdateUser(uid, func(u *store.User) {
           u.Active = false
       })
       
       // 撤销该用户所有 Session
       a.sessions().RevokeAllForUser(r.Context(), uid)
   }
   ```

3. **Emby/Telegram 绑定变更**
   - ⚠️ 待确认：是否需要撤销 Session

### 7.2 Cookie 安全

#### 当前配置
待审查 `issueSessionCookies` 实现：
```go
http.SetCookie(w, &http.Cookie{
    Name:     a.cfg().SessionCookie,
    Value:    token,
    Path:     "/",
    Expires:  expires,
    HttpOnly: true,            // ✅ 必须
    Secure:   a.cfg().SecureCookie,  // ⚠️ 生产环境必须 true
    SameSite: http.SameSiteLaxMode,  // ⚠️ 待确认
})
```

#### 建议配置
```go
SameSite: http.SameSiteLaxMode,  // 或 Strict（更安全但可能影响体验）
Secure:   true,                  // HTTPS 生产环境强制
HttpOnly: true,                  // 防止 XSS 窃取
```

---

## 八、文件上传安全审计

### 8.1 当前实现

#### 文件上传路径
- `/api/v1/users/me/avatar`
- `/api/v1/users/me/background`
- `/api/v1/tickets/:ticketId/attachments`
- `/api/v1/admin/config/upload-auth-background`
- `/api/v1/admin/config/upload-server-icon`

#### 安全检查
待审查 `upload_handlers.go`:
- [ ] 文件类型验证（MIME）
- [ ] 文件大小限制
- [ ] 文件名安全（路径穿越）
- [ ] 图片内容验证（防止恶意图片）
- [ ] 存储路径隔离

### 8.2 风险点

#### 🔴 路径穿越
```go
// ❌ 危险
filename := r.FormValue("filename")
savePath := filepath.Join(uploadDir, filename)  // 如果 filename = "../../../etc/passwd"

// ✅ 安全
filename := filepath.Base(r.FormValue("filename"))  // 去除路径
filename = strings.ReplaceAll(filename, "..", "")   // 防止绕过
savePath := filepath.Join(uploadDir, filename)
```

#### 🟠 恶意文件
- [ ] 是否验证图片格式（防止伪造）
- [ ] 是否限制图片尺寸（防止资源耗尽）
- [ ] 是否重新编码图片（防止嵌入恶意代码）

### 8.3 修复建议

#### 完整的文件上传验证
```go
func (a *App) handleUploadAvatar(w http.ResponseWriter, r *http.Request, _ Params) {
    p := a.current(r)
    
    // 1. 限流
    if !a.allowRate(r.Context(), rateKey("upload:avatar:", p.User.UID), 10, time.Hour) {
        failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "上传过于频繁")
        return
    }
    
    // 2. 解析文件
    file, header, err := r.FormFile("avatar")
    if err != nil {
        fail(w, http.StatusBadRequest, "文件解析失败")
        return
    }
    defer file.Close()
    
    // 3. 大小限制
    if header.Size > 10<<20 { // 10MB
        fail(w, http.StatusBadRequest, "文件大小不能超过 10MB")
        return
    }
    
    // 4. MIME 类型验证
    allowedMIMEs := []string{"image/jpeg", "image/png", "image/gif", "image/webp"}
    contentType := header.Header.Get("Content-Type")
    if !contains(allowedMIMEs, contentType) {
        fail(w, http.StatusBadRequest, "不支持的文件类型")
        return
    }
    
    // 5. 文件名安全
    filename := filepath.Base(header.Filename)
    filename = strings.ReplaceAll(filename, "..", "")
    if err := validateSafeFilename(filename); err != nil {
        fail(w, http.StatusBadRequest, err.Error())
        return
    }
    
    // 6. 生成安全的随机文件名
    ext := filepath.Ext(filename)
    safeFilename := fmt.Sprintf("%d_%s%s", p.User.UID, generateRandomString(16), ext)
    
    // 7. 存储路径隔离
    savePath := filepath.Join(a.cfg().UploadDir, "avatar", safeFilename)
    if err := ensureDir(filepath.Dir(savePath)); err != nil {
        fail(w, http.StatusInternalServerError, "保存失败")
        return
    }
    
    // 8. 保存文件
    dst, err := os.Create(savePath)
    if err != nil {
        fail(w, http.StatusInternalServerError, "保存失败")
        return
    }
    defer dst.Close()
    
    if _, err := io.Copy(dst, io.LimitReader(file, header.Size+1)); err != nil {
        os.Remove(savePath)
        fail(w, http.StatusInternalServerError, "保存失败")
        return
    }
    
    // 9. 更新用户头像 URL
    a.store().UpdateUser(p.User.UID, func(u *store.User) {
        u.Avatar = "/uploads/avatar/" + safeFilename
    })
    
    ok(w, "上传成功", map[string]any{"url": "/uploads/avatar/" + safeFilename})
}
```

---

## 九、SQL 注入审计

### 9.1 当前状态
✅ **使用参数化查询，风险较低**

#### PostgreSQL 查询
```go
// ✅ 安全：参数化查询
rows, err := s.db.Query("SELECT * FROM twilight_users WHERE username = $1", username)

// ❌ 危险（未发现）
// rows, err := s.db.Query(fmt.Sprintf("SELECT * FROM twilight_users WHERE username = '%s'", username))
```

### 9.2 审计方法
```bash
# 查找所有 SQL 语句
grep -rn "db.Query\|db.Exec\|db.QueryRow" internal/store/*.go | grep -v "\$[0-9]"
```

### 9.3 审计结果
待扫描后填写

---

## 十、XSS 防护审计

### 10.1 前端渲染

#### V2 SSR (SvelteKit)
- ✅ 默认转义输出
- ⚠️ 待确认：是否有 `{@html}` 使用

#### V1 客户端 (React)
- ✅ JSX 默认转义
- ⚠️ 待确认：是否有 `dangerouslySetInnerHTML`

### 10.2 风险点

#### 用户生成内容
- 工单标题/内容
- 公告内容
- 用户名、备注
- 媒体求片标题

#### 审计方法
```bash
# Svelte
grep -rn "{@html" webui-v2/src/**/*.svelte

# React
grep -rn "dangerouslySetInnerHTML" webui/src/**/*.tsx
```

### 10.3 修复建议

#### 安全渲染
```svelte
<!-- ✅ 安全：自动转义 -->
<p>{ticket.title}</p>

<!-- ❌ 危险 -->
<p>{@html ticket.content}</p>

<!-- ✅ 如果必须支持富文本，使用 DOMPurify -->
<script>
import DOMPurify from 'isomorphic-dompurify';
const safeHTML = DOMPurify.sanitize(ticket.content);
</script>
<p>{@html safeHTML}</p>
```

---

## 十一、待完成审计项

### 11.1 代码审查
- [ ] 扫描所有状态变更路由的限流
- [ ] 检查所有资源归属验证
- [ ] 审查所有文件上传处理
- [ ] 检查所有 SQL 语句
- [ ] 扫描所有 XSS 风险点
- [ ] 审查所有敏感信息泄露点

### 11.2 动态测试
- [ ] CSRF 攻击测试
- [ ] IDOR 漏洞测试
- [ ] 文件上传漏洞测试
- [ ] SQL 注入测试（黑盒）
- [ ] XSS 漏洞测试
- [ ] 权限绕过测试

### 11.3 配置审查
- [ ] Session Cookie 配置
- [ ] CORS 配置
- [ ] Rate Limit 配置
- [ ] 文件上传配置
- [ ] 日志配置

---

## 十二、修复优先级

### P0 - 立即修复（1 周内）
1. 🔴 **实现 CSRF 保护**（Double Submit Cookie 或 SameSite=Strict）
2. 🔴 **补充敏感操作限流**（登录、注册、密码重置、验证码）
3. 🔴 **审查 IDOR 漏洞**（工单、文件、API Key 等资源归属检查）

### P1 - 高优先级（2 周内）
4. 🟠 **完善文件上传验证**（MIME、大小、路径安全、图片验证）
5. 🟠 **审查敏感信息泄露**（错误消息、日志脱敏、API 响应）
6. 🟠 **完善会话管理**（密码修改撤销Session、账号禁用撤销Session）

### P2 - 中优先级（1 个月内）
7. 🟡 **补充输入验证**（长度、范围、格式）
8. 🟡 **XSS 防护检查**（扫描 `{@html}` 和 `dangerouslySetInnerHTML`）
9. 🟡 **SQL 注入扫描**（确认所有查询都参数化）

---

## 十三、总结与建议

### 13.1 当前安全态势
- ✅ **基础安全较好**: 参数化查询、全局限流、认证机制完善
- ⚠️ **中等风险**: 缺少 CSRF 保护、敏感操作限流不足
- 🔴 **需要加强**: IDOR 防护、文件上传安全、会话管理

### 13.2 快速改进建议
1. 立即实现 CSRF 保护（1-2 天）
2. 补充 5 个关键操作的限流（1-2 天）
3. 编写资源归属检查辅助函数，全面应用（3-5 天）

### 13.3 长期安全建设
1. 建立安全测试流程（单元测试、集成测试）
2. 定期安全扫描（依赖漏洞、代码扫描）
3. 安全培训与 Code Review 规范
4. 渗透测试（每季度一次）

---

**审计状态**: 🔄 进行中（框架完成，具体审计项待执行）
**下一步**: 开始执行代码扫描和动态测试
**预计完成时间**: 2026-09-12

