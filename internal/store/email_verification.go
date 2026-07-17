package store

import (
	"crypto/subtle"
	"errors"
	"sort"
	"strings"
)

// EmailVerification 是一次性邮箱验证码记录。安全约定：
//   - 永不存验证码明文，只存 CodeHash（由 api 层用服务端密钥做 HMAC-SHA256）；
//   - 带尝试上限（Attempts/MaxAttempts），命中上限即作废，防在线爆破；
//   - 带 ExpiresAt（TTL），过期即作废；
//   - LastSentAt 供重发冷却判定。
//
// Purpose 取值：
//   - "bind"                 绑定 / 验证邮箱（登录态，UID 必填）
//   - "reset_password"       登出态找回（重置系统密码，按 email 命中已验证账号）
//   - "change_password"      面板内改系统密码（登录态，UID 必填）
//   - "change_emby_password" 面板内改 Emby 密码（登录态，UID 必填）
type EmailVerification struct {
	ID          string `json:"id"`
	Purpose     string `json:"purpose"`
	Email       string `json:"email"`
	UID         int64  `json:"uid,omitempty"`
	CodeHash    string `json:"code_hash"`
	Attempts    int    `json:"attempts"`
	MaxAttempts int    `json:"max_attempts"`
	CreatedAt   int64  `json:"created_at"`
	ExpiresAt   int64  `json:"expires_at"`
	LastSentAt  int64  `json:"last_sent_at"`
}

type EmailVerificationAccountSnapshot struct {
	UID              int64
	Username         string
	Email            string
	EmailVerified    bool
	EmailVerifiedAt  int64
	TelegramID       int64
	TelegramUsername string
	Role             int
	Active           bool
}

type EmailVerificationAdminSnapshot struct {
	Records        []EmailVerification
	UsernamesByUID map[int64]string
	Accounts       []EmailVerificationAccountSnapshot
	Verified       int
	ExpiredPending int
}

// EmailVerificationResult 是 ConsumeEmailVerificationAtomic 的判定结果。
type EmailVerificationResult int

const (
	EmailVerificationNotFound EmailVerificationResult = iota
	EmailVerificationExpired
	EmailVerificationMismatch // 码错误但仍有剩余尝试次数
	EmailVerificationTooMany  // 错误次数耗尽，记录已作废
	EmailVerificationOK
)

// errEmailVerificationNoChange 仅用于让 mutateAndSaveLocked 在"未命中、无状态
// 变更"时回滚（等价 no-op）并跳过落盘；调用方捕获该哨兵后按正常结果返回。
var errEmailVerificationNoChange = errors.New("email verification: no change")

func normalizeEmailKey(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// PutEmailVerification 原子地清掉同一 (purpose,email) 下的旧记录再写入新记录，
// 保证每个用途+邮箱最多只有一份在用验证码（重复发码不累积，最新码生效）。
func (s *Store) PutEmailVerification(v EmailVerification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		target := normalizeEmailKey(v.Email)
		for id, existing := range s.state.EmailVerifications {
			if existing.Purpose == v.Purpose && normalizeEmailKey(existing.Email) == target {
				delete(s.state.EmailVerifications, id)
			}
		}
		s.state.EmailVerifications[v.ID] = v
		return nil
	})
}

// FindActiveEmailVerification 返回匹配 (purpose,email) 且未过期的记录（用于登出态
// 找回：客户端只提交 email+code，没有 id；也用于发码前的重发冷却判定）。
func (s *Store) FindActiveEmailVerification(purpose, email string, now int64) (EmailVerification, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	target := normalizeEmailKey(email)
	var best EmailVerification
	found := false
	for _, v := range s.state.EmailVerifications {
		if v.Purpose != purpose || normalizeEmailKey(v.Email) != target {
			continue
		}
		if v.ExpiresAt > 0 && v.ExpiresAt <= now {
			continue
		}
		if !found || v.LastSentAt > best.LastSentAt {
			best = v
			found = true
		}
	}
	return best, found
}

// ConsumeEmailVerificationAtomic 校验候选哈希并按结果原子更新状态：
//   - 过期：删除记录，返回 Expired；
//   - 命中：删除记录（消费），返回 OK；
//   - 不命中且尝试未达上限：Attempts+1 落盘，返回 Mismatch；
//   - 不命中且达上限：删除记录，返回 TooMany；
//   - 记录不存在：无状态变更，返回 NotFound。
//
// candidateHash 由 api 层用服务端密钥对 (id|code) 计算；store 只做定长常量时间比较，
// 不接触明文码与密钥。
func (s *Store) ConsumeEmailVerificationAtomic(id, candidateHash string, now int64) (EmailVerification, EmailVerificationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out EmailVerification
	result := EmailVerificationNotFound
	err := s.mutateAndSaveLocked(func() error {
		v, ok := s.state.EmailVerifications[id]
		if !ok {
			result = EmailVerificationNotFound
			return errEmailVerificationNoChange
		}
		out = v
		if v.ExpiresAt > 0 && v.ExpiresAt <= now {
			delete(s.state.EmailVerifications, id)
			result = EmailVerificationExpired
			return nil
		}
		if v.CodeHash != "" && subtle.ConstantTimeCompare([]byte(v.CodeHash), []byte(candidateHash)) == 1 {
			delete(s.state.EmailVerifications, id)
			result = EmailVerificationOK
			return nil
		}
		v.Attempts++
		out = v
		if v.MaxAttempts > 0 && v.Attempts >= v.MaxAttempts {
			delete(s.state.EmailVerifications, id)
			result = EmailVerificationTooMany
			return nil
		}
		s.state.EmailVerifications[id] = v
		result = EmailVerificationMismatch
		return nil
	})
	if errors.Is(err, errEmailVerificationNoChange) {
		return out, result, nil
	}
	if err != nil {
		return EmailVerification{}, result, err
	}
	return out, result, nil
}

// ListEmailVerifications 返回当前所有在用验证码记录的拷贝（含已过期但尚未被调度
// 清理的），供管理员审查。记录里仍带 CodeHash，api 层负责脱敏后再下发。
func (s *Store) ListEmailVerifications() []EmailVerification {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]EmailVerification, 0, len(s.state.EmailVerifications))
	for _, v := range s.state.EmailVerifications {
		out = append(out, v)
	}
	return out
}

func (s *Store) EmailVerificationAdminSnapshot(now int64) EmailVerificationAdminSnapshot {
	s.mu.RLock()
	records := make([]EmailVerification, 0, len(s.state.EmailVerifications))
	var usernames map[int64]string
	expiredPending := 0
	for _, v := range s.state.EmailVerifications {
		if v.ExpiresAt > 0 && v.ExpiresAt <= now {
			expiredPending++
		}
		if v.UID != 0 {
			if u, ok := s.state.Users[v.UID]; ok {
				if usernames == nil {
					usernames = make(map[int64]string)
				}
				usernames[v.UID] = u.Username
			}
		}
		records = append(records, v)
	}
	accounts := make([]EmailVerificationAccountSnapshot, 0)
	verified := 0
	for _, uid := range s.userUIDs {
		u, ok := s.state.Users[uid]
		if !ok || strings.TrimSpace(u.Email) == "" {
			continue
		}
		if u.EmailVerified {
			verified++
		}
		accounts = append(accounts, EmailVerificationAccountSnapshot{
			UID:              u.UID,
			Username:         u.Username,
			Email:            u.Email,
			EmailVerified:    u.EmailVerified,
			EmailVerifiedAt:  u.EmailVerifiedAt,
			TelegramID:       u.TelegramID,
			TelegramUsername: u.TelegramUsername,
			Role:             u.Role,
			Active:           u.Active,
		})
	}
	s.mu.RUnlock()

	sort.SliceStable(records, func(i, j int) bool {
		if records[i].LastSentAt == records[j].LastSentAt {
			return records[i].ID < records[j].ID
		}
		return records[i].LastSentAt > records[j].LastSentAt
	})
	return EmailVerificationAdminSnapshot{
		Records:        records,
		UsernamesByUID: usernames,
		Accounts:       accounts,
		Verified:       verified,
		ExpiredPending: expiredPending,
	}
}

func (s *Store) EmailVerification(id string) (EmailVerification, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.state.EmailVerifications[id]
	return v, ok
}

func (s *Store) DeleteEmailVerification(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		if _, ok := s.state.EmailVerifications[id]; !ok {
			return ErrNotFound
		}
		delete(s.state.EmailVerifications, id)
		return nil
	})
}

// CleanupExpiredEmailVerifications 清理过期验证码，由调度任务周期调用。
func (s *Store) CleanupExpiredEmailVerifications(now int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	deleted := 0
	err := s.mutateAndSaveLocked(func() error {
		for id, v := range s.state.EmailVerifications {
			if v.ExpiresAt > 0 && v.ExpiresAt <= now {
				delete(s.state.EmailVerifications, id)
				deleted++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

// ClearUnverifiedEmails 清空所有 EmailVerified=false 的用户的 Email 字段。
func (s *Store) ClearUnverifiedEmails() (total int, cleared int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err = s.mutateAndSaveLocked(func() error {
		for uid, u := range s.state.Users {
			if u.Email != "" && !u.EmailVerified {
				old := u
				total++
				u.Email = ""
				s.state.Users[uid] = u
				s.maintainUserIndexes(old, u, uid)
				cleared++
			}
		}
		return nil
	})
	return
}

// CleanupUnverifiedEmailsByAge 清除所有未验证邮箱中，用户账号创建时间早于 cutoff 的记录。
// 避免 ClearUnverifiedEmails 那种"注册不到 5 分钟就清掉邮箱"的激进行为，
// 给用户留足验证时间（默认 cutoff=24h）。
func (s *Store) CleanupUnverifiedEmailsByAge(cutoffUnix int64) (total int, cleared int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err = s.mutateAndSaveLocked(func() error {
		for uid, u := range s.state.Users {
			if u.Email != "" && !u.EmailVerified && u.CreatedAt > 0 && u.CreatedAt < cutoffUnix {
				old := u
				total++
				u.Email = ""
				s.state.Users[uid] = u
				s.maintainUserIndexes(old, u, uid)
				cleared++
			}
		}
		return nil
	})
	return
}

// EmailVerifiedOwner 返回已把 email 标记为"已验证"的其它账号（excludeUID 之外），
// 用于绑定 / 管理员强制绑定时的占用冲突判定。
func (s *Store) EmailVerifiedOwner(email string, excludeUID int64) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	target := normalizeEmailKey(email)
	if target == "" {
		return User{}, false
	}
	if s.verifiedEmailMap != nil {
		if uid, ok := s.verifiedEmailMap[target]; ok {
			if uid != excludeUID {
				u, found := s.state.Users[uid]
				if found && u.EmailVerified && normalizeEmailKey(u.Email) == target {
					return u, true
				}
			}
			for otherUID, u := range s.state.Users {
				if otherUID == excludeUID {
					continue
				}
				if u.EmailVerified && normalizeEmailKey(u.Email) == target {
					return u, true
				}
			}
		}
		return User{}, false
	}
	for uid, u := range s.state.Users {
		if uid == excludeUID {
			continue
		}
		if u.EmailVerified && normalizeEmailKey(u.Email) == target {
			return u, true
		}
	}
	return User{}, false
}

// EmailAlreadyUsed 检查邮箱是否已被任何账号使用（已验证或未验证均算占用）。
// 用于注册时防止两个用户填写同一个邮箱。
func (s *Store) EmailAlreadyUsed(email string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	target := normalizeEmailKey(email)
	if target == "" {
		return false
	}
	if s.emailMap != nil {
		if uid, ok := s.emailMap[target]; ok {
			u, found := s.state.Users[uid]
			return found && normalizeEmailKey(u.Email) == target
		}
		return false
	}
	for _, u := range s.state.Users {
		if u.Email != "" && normalizeEmailKey(u.Email) == target {
			return true
		}
	}
	return false
}

// FindUserByEmailVerified 按"已验证邮箱"精确命中账号（登出态找回用）。
// 只认 EmailVerified=true 的账号：未验证邮箱不足以证明归属，否则把别人邮箱
// 写成自己的未验证邮箱即可劫持对方账号的找回入口。
func (s *Store) FindUserByEmailVerified(email string) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	target := normalizeEmailKey(email)
	if target == "" {
		return User{}, false
	}
	if s.verifiedEmailMap != nil {
		if uid, ok := s.verifiedEmailMap[target]; ok {
			u, found := s.state.Users[uid]
			if found && u.EmailVerified && normalizeEmailKey(u.Email) == target {
				return u, true
			}
		}
		return User{}, false
	}
	for _, u := range s.state.Users {
		if u.EmailVerified && normalizeEmailKey(u.Email) == target {
			return u, true
		}
	}
	return User{}, false
}

// SetUserEmailVerifiedAtomic 绑定邮箱并设置验证状态。约定：
//   - email 非空则更新 User.Email；为空则保留现有邮箱（用于仅切换验证态）。
//   - verified=true 且 force=false 时做占用冲突校验（其它已验证账号占用 → ErrConflict）。
//   - verified=false 时清空 EmailVerifiedAt（撤销验证）。
func (s *Store) SetUserEmailVerifiedAtomic(uid int64, email string, verified, force bool, now int64) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var updated User
	err := s.mutateAndSaveLocked(func() error {
		u, ok := s.state.Users[uid]
		if !ok {
			return ErrNotFound
		}
		old := u
		targetEmail := u.Email
		if strings.TrimSpace(email) != "" {
			targetEmail = strings.TrimSpace(email)
		}
		if verified && !force {
			norm := normalizeEmailKey(targetEmail)
			if norm != "" {
				if s.verifiedEmailMap != nil {
					if otherUID, exists := s.verifiedEmailMap[norm]; exists {
						if otherUID != uid {
							return ErrConflict
						}
						for scanUID, other := range s.state.Users {
							if scanUID == uid {
								continue
							}
							if other.EmailVerified && normalizeEmailKey(other.Email) == norm {
								return ErrConflict
							}
						}
					}
				} else {
					for otherUID, other := range s.state.Users {
						if otherUID == uid {
							continue
						}
						if other.EmailVerified && normalizeEmailKey(other.Email) == norm {
							return ErrConflict
						}
					}
				}
			}
		}
		u.Email = targetEmail
		u.EmailVerified = verified
		if verified {
			u.EmailVerifiedAt = now
		} else {
			u.EmailVerifiedAt = 0
		}
		s.state.Users[uid] = u
		s.maintainUserIndexes(old, u, uid)
		updated = u
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return updated, nil
}
