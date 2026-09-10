# Twilight V2 数据库模型拆分设计

> **设计时间**: 2026-09-10
> **当前状态**: 设计阶段
> **目标**: 解决单 JSONB 文档写放大问题，支持数据库级约束和高选择性索引

---

## 一、当前问题分析

### 1.1 核心问题
当前主要业务数据存储在 PostgreSQL `twilight_state` 表的单行 JSONB 字段中：

```sql
CREATE TABLE twilight_state (
    id SERIAL PRIMARY KEY,
    state JSONB NOT NULL,
    version BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

**State 结构中的主要实体**（仍在 JSONB 中）：
- `Users` (map[int64]User) - 用户主表
- `Tickets` (map[int64]Ticket) - 工单系统
- `RegCodes` (map[string]RegCode) - 注册码
- `InviteCodes` (map[string]InviteCode) - 邀请码
- `InviteRelations` (map[int64]InviteRelation) - 邀请关系树
- `MediaRequests` (map[int64]MediaRequest) - 媒体求片
- `Announcements` (map[int64]Announcement) - 公告
- `Signin` (map[int64]Signin) - 签到记录

**已独立的实体**：
- ✅ `twilight_audit_logs` - 审计日志
- ✅ `twilight_runtime_logs` - 运行日志
- ✅ `twilight_sessions` - 会话
- ✅ `twilight_playback_*` - 播放事件、段、每日统计
- ✅ `twilight_telegram_roster` - Telegram 花名册
- ✅ `twilight_telegram_runtime` - Telegram 轮询游标

### 1.2 写放大问题
每次修改单个实体都需要：
1. SELECT 整个 JSONB (~几MB 到几十MB)
2. 反序列化到 Go struct
3. 重建内存索引（username、email、telegramID、embyID 等）
4. 修改单个字段
5. 序列化整个 State
6. UPDATE 整个 JSONB

**问题表现**：
- 内存峰值：每次写入需要加载完整状态
- CPU 开销：频繁序列化/反序列化
- 锁竞争：写操作持有全局写锁
- 写冲突：多进程（api/bot/scheduler）版本竞争
- 无法利用数据库约束（外键、唯一索引）

### 1.3 受影响的高频操作
- 用户注册、登录、状态变更
- 工单创建、回复
- 注册码生成、使用
- 邀请关系建立、删除
- 媒体求片提交、状态变更

---

## 二、拆分优先级与策略

### 2.1 拆分优先级

**阶段 1（优先级最高）**：
1. **Users** - 用户主表
   - 写频率：高（注册、登录、状态变更、Emby 绑定）
   - 依赖关系：被几乎所有其他表引用
   - 索引需求：username、email、telegram_id、emby_id
   - 预估影响：减少 80% 的写放大

**阶段 2（高优先级）**：
2. **Tickets** - 工单系统
   - 写频率：中高（创建、回复、状态变更）
   - 依赖关系：依赖 Users
   - 索引需求：owner_uid、status、created_at
   
3. **RegCodes** - 注册码
   - 写频率：中（生成、使用、启停）
   - 依赖关系：独立
   - 索引需求：code（主键）、type、status
   
4. **InviteCodes + InviteRelations** - 邀请系统
   - 写频率：中（创建、使用、关系变更）
   - 依赖关系：依赖 Users
   - 索引需求：code、parent_uid、child_uid

**阶段 3（中优先级）**：
5. **MediaRequests** - 媒体求片
   - 写频率：中（提交、状态变更）
   - 依赖关系：依赖 Users
   - 索引需求：uid、status、title

6. **Announcements** - 公告
   - 写频率：低
   - 依赖关系：独立
   - 索引需求：id、visible、pinned

### 2.2 拆分策略

**双写双读策略**：
1. **准备阶段**：创建新表 schema，不改变业务逻辑
2. **双写阶段**：写入同时更新 JSONB 和新表，读取优先新表
3. **验证阶段**：对比新表和 JSONB 数据一致性
4. **切换阶段**：只读写新表，JSONB 保留作备份
5. **清理阶段**：删除 JSONB 中的旧字段

**迁移不变量**：
- UID/ID 保持不变
- 外键关系明确
- 迁移可回滚
- 不停机迁移

---

## 三、阶段 1：Users 表设计

### 3.1 表结构设计

```sql
-- 用户主表
CREATE TABLE twilight_users (
    uid BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    
    -- 邮箱
    email VARCHAR(255),
    email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    email_verified_at BIGINT,
    
    -- Telegram
    telegram_id BIGINT UNIQUE,
    telegram_username VARCHAR(255),
    
    -- Emby
    emby_id VARCHAR(255) UNIQUE,
    emby_username VARCHAR(255),
    emby_disabled BOOLEAN NOT NULL DEFAULT FALSE,
    
    -- 角色与状态
    role INT NOT NULL DEFAULT 1, -- 0=admin, 1=normal, 2=whitelist
    active BOOLEAN NOT NULL DEFAULT TRUE,
    expired_at BIGINT NOT NULL DEFAULT -1, -- -1=永久
    
    -- 外观
    avatar VARCHAR(512),
    background VARCHAR(512),
    
    -- Bangumi
    bgm_mode BOOLEAN NOT NULL DEFAULT FALSE,
    bgm_manage_mode BOOLEAN NOT NULL DEFAULT FALSE,
    bgm_token VARCHAR(512),
    
    -- 时间戳
    created_at BIGINT NOT NULL,
    register_time BIGINT NOT NULL,
    
    -- Emby 注册资格
    emby_grant_locked BOOLEAN NOT NULL DEFAULT FALSE,
    registration_source VARCHAR(64),
    registration_code VARCHAR(255),
    pending_emby BOOLEAN NOT NULL DEFAULT FALSE,
    pending_emby_days INT,
    
    -- 通知偏好
    notify_on_login_telegram BOOLEAN NOT NULL DEFAULT FALSE,
    notify_on_login_email BOOLEAN NOT NULL DEFAULT FALSE,
    notify_on_ticket_telegram BOOLEAN NOT NULL DEFAULT FALSE,
    
    -- 签到与续期
    signin_auto_renewal BOOLEAN NOT NULL DEFAULT FALSE,
    
    -- 密码安全偏好
    require_email_for_password_change BOOLEAN NOT NULL DEFAULT FALSE,
    require_email_for_emby_password_change BOOLEAN NOT NULL DEFAULT FALSE,
    require_old_password_for_emby_password_change BOOLEAN NOT NULL DEFAULT FALSE,
    
    -- 遗留 API Key
    legacy_api_key_hash VARCHAR(255),
    legacy_api_key_prefix VARCHAR(16),
    legacy_api_key_suffix VARCHAR(16),
    legacy_api_key_status BOOLEAN NOT NULL DEFAULT FALSE,
    legacy_permissions JSONB, -- []string
    
    -- Telegram 换绑中
    rebinding_in_progress BOOLEAN NOT NULL DEFAULT FALSE,
    rebinding_since BIGINT,
    
    -- 已读公告
    seen_announcement_ids JSONB, -- []int64
    
    -- 审计字段
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- 索引
CREATE INDEX idx_users_username ON twilight_users(username);
CREATE INDEX idx_users_email ON twilight_users(email) WHERE email IS NOT NULL;
CREATE INDEX idx_users_email_verified ON twilight_users(email) WHERE email_verified = TRUE;
CREATE INDEX idx_users_telegram_id ON twilight_users(telegram_id) WHERE telegram_id IS NOT NULL;
CREATE INDEX idx_users_emby_id ON twilight_users(emby_id) WHERE emby_id IS NOT NULL;
CREATE INDEX idx_users_role ON twilight_users(role);
CREATE INDEX idx_users_active ON twilight_users(active);
CREATE INDEX idx_users_expired_at ON twilight_users(expired_at);
CREATE INDEX idx_users_created_at ON twilight_users(created_at);

-- 触发器：自动更新 updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON twilight_users
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
```

### 3.2 迁移步骤

#### Step 1: 创建表与索引（不影响业务）
```sql
-- 执行上述 CREATE TABLE 和 CREATE INDEX
```

#### Step 2: 数据迁移脚本
```go
func (s *Store) MigrateUsersToTable() error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // 1. 批量插入用户到新表
    tx, err := s.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()
    
    stmt, err := tx.Prepare(`
        INSERT INTO twilight_users (
            uid, username, password_hash, email, email_verified, email_verified_at,
            telegram_id, telegram_username, emby_id, emby_username, emby_disabled,
            role, active, expired_at, avatar, background,
            bgm_mode, bgm_manage_mode, bgm_token,
            created_at, register_time,
            emby_grant_locked, registration_source, registration_code,
            pending_emby, pending_emby_days,
            notify_on_login_telegram, notify_on_login_email, notify_on_ticket_telegram,
            signin_auto_renewal,
            require_email_for_password_change,
            require_email_for_emby_password_change,
            require_old_password_for_emby_password_change,
            legacy_api_key_hash, legacy_api_key_prefix, legacy_api_key_suffix,
            legacy_api_key_status, legacy_permissions,
            rebinding_in_progress, rebinding_since,
            seen_announcement_ids
        ) VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
            $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30,
            $31, $32, $33, $34, $35, $36, $37, $38, $39, $40, $41
        ) ON CONFLICT (uid) DO NOTHING
    `)
    if err != nil {
        return err
    }
    defer stmt.Close()
    
    for _, u := range s.state.Users {
        legacyPerms, _ := json.Marshal(u.LegacyPermissions)
        seenAnnouncements, _ := json.Marshal(u.SeenAnnouncementIDs)
        
        _, err = stmt.Exec(
            u.UID, u.Username, u.PasswordHash,
            nullString(u.Email), u.EmailVerified, nullInt64(u.EmailVerifiedAt),
            nullInt64(u.TelegramID), nullString(u.TelegramUsername),
            nullString(u.EmbyID), nullString(u.EmbyUsername), u.EmbyDisabled,
            u.Role, u.Active, u.ExpiredAt,
            nullString(u.Avatar), nullString(u.Background),
            u.BGMMode, u.BGMManageMode, nullString(u.BGMToken),
            u.CreatedAt, u.RegisterTime,
            u.EmbyGrantLocked, nullString(u.RegistrationSource), nullString(u.RegistrationCode),
            u.PendingEmby, nullIntPtr(u.PendingEmbyDays),
            u.NotifyOnLoginTelegram, u.NotifyOnLoginEmail, u.NotifyOnTicketTelegram,
            u.SigninAutoRenewal,
            u.RequireEmailForPasswordChange,
            u.RequireEmailForEmbyPasswordChange,
            u.RequireOldPasswordForEmbyPasswordChange,
            nullString(u.LegacyAPIKeyHash), nullString(u.LegacyAPIKeyPrefix),
            nullString(u.LegacyAPIKeySuffix), u.LegacyAPIKeyStatus, legacyPerms,
            u.RebindingInProgress, nullInt64(u.RebindingSince),
            seenAnnouncements,
        )
        if err != nil {
            return fmt.Errorf("migrate user %d: %w", u.UID, err)
        }
    }
    
    return tx.Commit()
}
```

#### Step 3: 实现双写
```go
func (s *Store) CreateUser(u User) (User, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    var created User
    err := s.mutateAndSaveLocked(func() error {
        // 检查冲突（保持原有逻辑）
        if s.usernameExistsLocked(u.Username) || ... {
            return ErrConflict
        }
        
        // 分配 UID
        u.UID = s.state.NextUserID
        s.state.NextUserID++
        now := time.Now().Unix()
        if u.CreatedAt == 0 {
            u.CreatedAt = now
        }
        if u.RegisterTime == 0 {
            u.RegisterTime = now
        }
        u.Active = true
        
        // 写入 JSONB（旧逻辑）
        s.state.Users[u.UID] = u
        s.maintainUserIndexes(User{}, u, u.UID)
        
        // 双写：同时写入新表
        err := s.insertUserToTable(u)
        if err != nil {
            // 双写失败，回滚整个操作
            return fmt.Errorf("dual write to table failed: %w", err)
        }
        
        created = u
        return nil
    })
    if err != nil {
        return User{}, err
    }
    return created, nil
}

func (s *Store) insertUserToTable(u User) error {
    // 注意：此函数在 s.mu 锁内调用，且在 mutateAndSaveLocked 的事务中
    // 需要确保 JSONB UPSERT 和 Table INSERT 在同一个 PostgreSQL 事务中
    // 当前架构需要调整为：先更新内存，再一次性提交 JSONB + Table
    
    // 实现策略：在 saveLocked 中增加 table 写入逻辑
    // 暂时标记为需要写入，在 saveLocked 统一处理
    return nil
}
```

#### Step 4: 读取优先新表
```go
func (s *Store) FindUserByUsername(username string) (User, bool) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    
    // 优先从新表读取
    u, err := s.findUserByUsernameFromTable(username)
    if err == nil {
        return u, true
    }
    
    // 降级到 JSONB
    if uid, exists := s.usernameMap[strings.ToLower(username)]; exists {
        if u, ok := s.state.Users[uid]; ok {
            return u, true
        }
    }
    return User{}, false
}
```

### 3.3 事务一致性设计

**问题**：当前 `mutateAndSaveLocked` 只更新 JSONB，需要扩展支持同时更新 table。

**方案**：
1. 在 `mutateAndSaveLocked` 中收集需要同步到 table 的变更
2. `saveLocked` 中使用 PostgreSQL 事务：
   ```sql
   BEGIN;
   UPDATE twilight_state SET state = $1, version = $2 WHERE id = 1 AND version = $3;
   -- 同时执行 table 的 INSERT/UPDATE/DELETE
   COMMIT;
   ```
3. 任何一步失败，整个事务回滚

**实现草图**：
```go
type tablePendingWrites struct {
    usersToInsert []User
    usersToUpdate []User
    usersToDelete []int64
    // ... 其他表
}

func (s *Store) mutateAndSaveLocked(fn func() error) error {
    // 1. 快照当前状态用于回滚
    snapshot := s.snapshotStateLocked()
    
    // 2. 清空待写入队列
    s.pendingWrites = tablePendingWrites{}
    
    // 3. 执行修改（可能会添加到 pendingWrites）
    if err := fn(); err != nil {
        return err
    }
    
    // 4. 保存（JSONB + Tables 在同一个 PostgreSQL 事务）
    if err := s.saveLockedWithTables(); err != nil {
        s.restoreStateLocked(snapshot)
        return err
    }
    
    return nil
}

func (s *Store) saveLockedWithTables() error {
    tx, err := s.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()
    
    // 1. 更新 JSONB
    stateBytes, _ := json.Marshal(s.state)
    result, err := tx.Exec(`
        UPDATE twilight_state 
        SET state = $1, version = version + 1 
        WHERE id = 1 AND version = $2
        RETURNING version
    `, stateBytes, s.stateVersion)
    if err != nil {
        return err
    }
    rowsAffected, _ := result.RowsAffected()
    if rowsAffected == 0 {
        return errStateVersionConflict
    }
    s.stateVersion++
    
    // 2. 同步写入 tables
    for _, u := range s.pendingWrites.usersToInsert {
        _, err := tx.Exec(`INSERT INTO twilight_users (...) VALUES (...)`, ...)
        if err != nil {
            return err
        }
    }
    for _, u := range s.pendingWrites.usersToUpdate {
        _, err := tx.Exec(`UPDATE twilight_users SET ... WHERE uid = $1`, u.UID, ...)
        if err != nil {
            return err
        }
    }
    for _, uid := range s.pendingWrites.usersToDelete {
        _, err := tx.Exec(`DELETE FROM twilight_users WHERE uid = $1`, uid)
        if err != nil {
            return err
        }
    }
    
    return tx.Commit()
}
```

### 3.4 验证与切换

#### 验证阶段
```go
func (s *Store) VerifyUsersMigration() (bool, []string) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    
    inconsistencies := []string{}
    
    // 1. 对比用户数量
    jsonCount := len(s.state.Users)
    var tableCount int
    s.db.QueryRow("SELECT COUNT(*) FROM twilight_users").Scan(&tableCount)
    if jsonCount != tableCount {
        inconsistencies = append(inconsistencies, 
            fmt.Sprintf("count mismatch: JSONB=%d, Table=%d", jsonCount, tableCount))
    }
    
    // 2. 抽样对比字段
    for uid, jsonUser := range s.state.Users {
        var tableUser User
        err := s.db.QueryRow(`SELECT ... FROM twilight_users WHERE uid = $1`, uid).
            Scan(...)
        if err != nil {
            inconsistencies = append(inconsistencies, 
                fmt.Sprintf("uid=%d not found in table", uid))
            continue
        }
        
        // 对比关键字段
        if jsonUser.Username != tableUser.Username {
            inconsistencies = append(inconsistencies, 
                fmt.Sprintf("uid=%d username mismatch", uid))
        }
        // ... 更多字段对比
    }
    
    return len(inconsistencies) == 0, inconsistencies
}
```

#### 切换阶段
```go
// 切换标记：环境变量或配置文件
// TWILIGHT_USERS_SOURCE=table  // 只读写新表
// TWILIGHT_USERS_SOURCE=jsonb  // 只读写 JSONB（回滚）
// TWILIGHT_USERS_SOURCE=dual   // 双读双写（默认，迁移期间）

func (s *Store) FindUserByUsername(username string) (User, bool) {
    source := getUsersDataSource() // 读取配置
    
    switch source {
    case "table":
        // 只从新表读
        return s.findUserByUsernameFromTable(username)
    case "jsonb":
        // 只从 JSONB 读
        s.mu.RLock()
        defer s.mu.RUnlock()
        if uid, exists := s.usernameMap[strings.ToLower(username)]; exists {
            if u, ok := s.state.Users[uid]; ok {
                return u, true
            }
        }
        return User{}, false
    case "dual":
    default:
        // 优先新表，降级 JSONB
        u, err := s.findUserByUsernameFromTable(username)
        if err == nil {
            return u, true
        }
        // 降级逻辑...
    }
}
```

---

## 四、阶段 2：Tickets 表设计

### 4.1 表结构

```sql
-- 工单主表
CREATE TABLE twilight_tickets (
    id BIGSERIAL PRIMARY KEY,
    owner_uid BIGINT NOT NULL REFERENCES twilight_users(uid) ON DELETE CASCADE,
    title VARCHAR(512) NOT NULL,
    content TEXT NOT NULL,
    type VARCHAR(128) NOT NULL,
    priority VARCHAR(32) NOT NULL DEFAULT 'medium', -- low, medium, high, urgent
    status VARCHAR(32) NOT NULL DEFAULT 'open', -- open, replied, closed
    revision INT NOT NULL DEFAULT 0, -- 乐观锁
    
    notify_telegram BOOLEAN, -- NULL=未设置，使用全局默认
    
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    closed_at BIGINT,
    
    CONSTRAINT chk_priority CHECK (priority IN ('low', 'medium', 'high', 'urgent')),
    CONSTRAINT chk_status CHECK (status IN ('open', 'replied', 'closed'))
);

-- 工单回复表
CREATE TABLE twilight_ticket_replies (
    id BIGSERIAL PRIMARY KEY,
    ticket_id BIGINT NOT NULL REFERENCES twilight_tickets(id) ON DELETE CASCADE,
    from_uid BIGINT NOT NULL REFERENCES twilight_users(uid) ON DELETE CASCADE,
    is_admin BOOLEAN NOT NULL,
    content TEXT NOT NULL,
    attachments JSONB, -- []string
    created_at BIGINT NOT NULL
);

-- 工单附件表（可选，或直接用 JSONB）
CREATE TABLE twilight_ticket_attachments (
    id BIGSERIAL PRIMARY KEY,
    ticket_id BIGINT NOT NULL REFERENCES twilight_tickets(id) ON DELETE CASCADE,
    reply_id BIGINT REFERENCES twilight_ticket_replies(id) ON DELETE CASCADE,
    filename VARCHAR(512) NOT NULL,
    uploaded_by BIGINT NOT NULL REFERENCES twilight_users(uid),
    uploaded_at BIGINT NOT NULL
);

-- 索引
CREATE INDEX idx_tickets_owner_uid ON twilight_tickets(owner_uid);
CREATE INDEX idx_tickets_status ON twilight_tickets(status);
CREATE INDEX idx_tickets_created_at ON twilight_tickets(created_at DESC);
CREATE INDEX idx_tickets_type ON twilight_tickets(type);
CREATE INDEX idx_ticket_replies_ticket_id ON twilight_ticket_replies(ticket_id);
CREATE INDEX idx_ticket_replies_created_at ON twilight_ticket_replies(created_at);
```

### 4.2 迁移步骤
- 同 Users，分阶段：创建表 → 数据迁移 → 双写 → 验证 → 切换

---

## 五、阶段 3-5：其他表设计

### 5.1 RegCodes 表

```sql
CREATE TABLE twilight_regcodes (
    code VARCHAR(64) PRIMARY KEY,
    type INT NOT NULL, -- 0=注册, 1=续期, 2=白名单
    days INT NOT NULL,
    source VARCHAR(32) NOT NULL DEFAULT 'admin', -- admin, invite
    
    valid_from BIGINT,
    valid_until BIGINT,
    max_uses INT NOT NULL DEFAULT 1,
    use_count INT NOT NULL DEFAULT 0,
    
    paused BOOLEAN NOT NULL DEFAULT FALSE,
    paused_seconds BIGINT NOT NULL DEFAULT 0,
    pause_start BIGINT,
    
    used_by_uids JSONB, -- []int64
    used_by_uid BIGINT, -- 最后使用者
    used_at BIGINT, -- 最后使用时间
    used BOOLEAN NOT NULL DEFAULT FALSE, -- 是否已使用
    active BOOLEAN NOT NULL DEFAULT TRUE, -- 是否可用
    
    created_by_uid BIGINT REFERENCES twilight_users(uid) ON DELETE SET NULL,
    created_at BIGINT NOT NULL,
    note TEXT
);

CREATE INDEX idx_regcodes_type ON twilight_regcodes(type);
CREATE INDEX idx_regcodes_source ON twilight_regcodes(source);
CREATE INDEX idx_regcodes_active ON twilight_regcodes(active);
CREATE INDEX idx_regcodes_used ON twilight_regcodes(used);
CREATE INDEX idx_regcodes_created_at ON twilight_regcodes(created_at DESC);
```

### 5.2 InviteCodes 表

```sql
CREATE TABLE twilight_invite_codes (
    code VARCHAR(64) PRIMARY KEY,
    parent_uid BIGINT NOT NULL REFERENCES twilight_users(uid) ON DELETE CASCADE,
    type INT NOT NULL, -- 0=邀请, 2=续期
    days INT NOT NULL,
    
    used BOOLEAN NOT NULL DEFAULT FALSE,
    used_by_uid BIGINT REFERENCES twilight_users(uid) ON DELETE SET NULL,
    used_at BIGINT,
    use_count INT NOT NULL DEFAULT 0,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    
    created_at BIGINT NOT NULL,
    note TEXT
);

CREATE INDEX idx_invite_codes_parent_uid ON twilight_invite_codes(parent_uid);
CREATE INDEX idx_invite_codes_used_by_uid ON twilight_invite_codes(used_by_uid);
CREATE INDEX idx_invite_codes_active ON twilight_invite_codes(active);
```

### 5.3 InviteRelations 表

```sql
CREATE TABLE twilight_invite_relations (
    id BIGSERIAL PRIMARY KEY,
    parent_uid BIGINT NOT NULL REFERENCES twilight_users(uid) ON DELETE CASCADE,
    child_uid BIGINT NOT NULL UNIQUE REFERENCES twilight_users(uid) ON DELETE CASCADE,
    created_at BIGINT NOT NULL,
    
    CONSTRAINT chk_not_self_invite CHECK (parent_uid != child_uid)
);

CREATE INDEX idx_invite_relations_parent_uid ON twilight_invite_relations(parent_uid);
CREATE INDEX idx_invite_relations_child_uid ON twilight_invite_relations(child_uid);
CREATE UNIQUE INDEX idx_invite_relations_child ON twilight_invite_relations(child_uid);
```

### 5.4 MediaRequests 表

```sql
CREATE TABLE twilight_media_requests (
    id BIGSERIAL PRIMARY KEY,
    uid BIGINT NOT NULL REFERENCES twilight_users(uid) ON DELETE CASCADE,
    
    source VARCHAR(32) NOT NULL, -- tmdb, bangumi
    source_id VARCHAR(128) NOT NULL,
    title VARCHAR(512) NOT NULL,
    media_info JSONB NOT NULL,
    
    status VARCHAR(32) NOT NULL DEFAULT 'unhandled',
    revision INT NOT NULL DEFAULT 0,
    
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    handled_by_uid BIGINT REFERENCES twilight_users(uid) ON DELETE SET NULL,
    handled_at BIGINT,
    note TEXT,
    
    CONSTRAINT chk_source CHECK (source IN ('tmdb', 'bangumi', 'emby')),
    CONSTRAINT chk_status CHECK (status IN ('unhandled', 'accepted', 'rejected', 'completed', 'downloading'))
);

CREATE INDEX idx_media_requests_uid ON twilight_media_requests(uid);
CREATE INDEX idx_media_requests_status ON twilight_media_requests(status);
CREATE INDEX idx_media_requests_source ON twilight_media_requests(source);
CREATE INDEX idx_media_requests_title ON twilight_media_requests(title);
CREATE INDEX idx_media_requests_created_at ON twilight_media_requests(created_at DESC);
```

---

## 六、实施计划

### 6.1 里程碑

**M1: Users 表（预计 2 周）**
- Week 1:
  - [ ] Day 1-2: 设计 schema，创建表和索引
  - [ ] Day 3-4: 实现数据迁移脚本
  - [ ] Day 5: 实现双写逻辑
- Week 2:
  - [ ] Day 1-2: 实现读取优先新表
  - [ ] Day 3: 完整验证与测试
  - [ ] Day 4: 灰度切换（dual → table）
  - [ ] Day 5: 监控与调优

**M2: Tickets 表（预计 1 周）**
- [ ] Day 1-2: Schema + 迁移
- [ ] Day 3: 双写 + 读取
- [ ] Day 4-5: 验证 + 切换

**M3: RegCodes 表（预计 1 周）**
- 同上

**M4: Invite 表（预计 1 周）**
- 同上

**M5: MediaRequests 表（预计 1 周）**
- 同上

### 6.2 回滚策略
1. 每个阶段保持 JSONB 完整性，可随时切回
2. 配置标记控制读写来源
3. 验证失败立即停止下一阶段
4. 保留 JSONB 至少 1 个月后再清理

### 6.3 性能指标
- [ ] 用户注册 QPS 提升 5x
- [ ] 工单创建延迟降低 80%
- [ ] 内存峰值降低 60%
- [ ] 写冲突率降低 90%

---

## 七、风险与缓解

### 7.1 风险
1. **数据不一致**：双写期间 JSONB 和 Table 可能不同步
   - 缓解：事务保证原子性，验证脚本检查一致性
   
2. **性能回退**：Table 读写可能比 JSONB 慢
   - 缓解：充分索引，灰度切换，性能测试
   
3. **迁移中断**：迁移期间服务重启
   - 缓解：幂等迁移脚本，可重复执行
   
4. **外键级联**：DELETE CASCADE 可能误删数据
   - 缓解：软删除 + 定期清理，或改用 SET NULL

### 7.2 监控指标
- JSONB vs Table 数据一致性
- 双写失败率
- 读取延迟（p50, p99）
- 写入延迟（p50, p99）
- 版本冲突率

---

## 八、后续优化

### 8.1 进一步拆分
- Announcements → twilight_announcements
- Signin → twilight_signin
- Devices → twilight_devices
- EmailVerifications → twilight_email_verifications
- BindCodes → twilight_bind_codes

### 8.2 缓存策略
- Redis 缓存热点用户
- 内存缓存用户索引（username → uid）
- 查询结果缓存（30s TTL）

### 8.3 分库分表（长期）
- 按 UID 哈希分片用户表
- 按时间分区审计日志
- 读写分离（主从复制）

---

## 九、参考资料

### 9.1 相关文档
- `docs/v2/v1-audit.md` - V1 审计基线
- `docs/v2/architecture.md` - V2 架构设计
- `docs/guides/modular-architecture.md` - 模块化架构指南
- `internal/store/store.go` - 当前 Store 实现

### 9.2 技术参考
- PostgreSQL JSONB vs Relational: https://www.postgresql.org/docs/current/datatype-json.html
- Optimistic Locking: https://www.postgresql.org/docs/current/mvcc.html
- Schema Migration Best Practices: https://fly.io/blog/safe-database-migrations/

---

**最后更新**: 2026-09-10
**设计者**: Claude (Opus 4.6)
**审核状态**: 待审核
