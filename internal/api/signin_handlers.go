package api

import (
	"crypto/rand"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/store"
)

// 签到子系统 handlers + 配置/计算辅助。
// 从 handlers.go 拆出，让"签到 + 连签奖励"这条业务线独立成文件，
// 避免 handlers.go 持续往 3000+ 行膨胀。
// 拆分原则：仅做「按业务域归位」，不修改任何对外行为；现有 router/路由表不变。

const (
	signinDefaultCurrencyName        = "积分"
	signinRenewalDisabledMessage     = "积分续期功能未开启"
	signinRenewalPermanentMessage    = "永久账号无需续期"
	signinRenewalInsufficientMessage = "积分不足，无法续期"
	signinRenewalUnavailableMessage  = "当前账号无需或无法使用积分续期"
	signinRenewalSuccessMessage      = "续期成功"
)

func (a *App) handleSigninConfig(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", signinConfigPayload(*a.cfg()))
}

func (a *App) handleSigninMe(w http.ResponseWriter, r *http.Request, _ Params) {
	user := current(r).User
	si := a.store().Signin(user.UID)
	payload := signinSummaryPayload(*a.cfg(), si)
	a.attachSigninAutoRenewal(payload, *a.cfg(), si.Points, user)
	ok(w, "OK", payload)
}

func (a *App) handleSignin(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.requireSigninEnabled(w) {
		return
	}
	dailyPoints := signinDailyPoints(*a.cfg())
	si, createdToday, err := a.store().AddSigninWithOptions(current(r).User.UID, dailyPoints, func(streak int) int {
		return signinBonusForStreak(*a.cfg(), streak)
	}, a.cfg().SigninResetAfterMiss)
	if statusFromError(w, err) {
		return
	}
	bonusPoints := 0
	if !createdToday {
		dailyPoints = 0
	} else if len(si.Records) > 0 {
		last := si.Records[len(si.Records)-1]
		if last.Date == time.Now().Format("2006-01-02") {
			dailyPoints = last.Points
			bonusPoints = last.BonusPoints
		}
	}
	payload := signinActionPayload(*a.cfg(), si, createdToday, dailyPoints, bonusPoints)
	a.attachSigninAutoRenewal(payload, *a.cfg(), si.Points, current(r).User)
	if createdToday {
		a.audit(r, "signin", "user", 0, map[string]any{"points": dailyPoints, "bonus": bonusPoints})
		ok(w, "签到成功", payload)
		return
	}
	ok(w, "今日已签到", payload)
}

func (a *App) handleSigninHistory(w http.ResponseWriter, r *http.Request, _ Params) {
	limit := queryInt(r, "limit", 30)
	if limit <= 0 || limit > 365 {
		limit = 30
	}
	records := a.store().SigninHistoryRecords(current(r).User.UID, limit)
	items := make([]map[string]any, 0, len(records))
	for _, record := range records {
		total := record.Total
		if total == 0 {
			total = record.Points + record.BonusPoints
		}
		streak := record.Streak
		if streak <= 0 {
			streak = 1
		}
		items = append(items, map[string]any{
			"date":         record.Date,
			"daily_points": record.Points,
			"bonus_points": record.BonusPoints,
			"total":        total,
			"streak":       streak,
			"created_at":   record.CreatedAt,
		})
	}
	ok(w, "OK", map[string]any{"records": items, "currency_name": signinCurrencyName(*a.cfg())})
}

func (a *App) handleSigninRenew(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.requireSigninEnabled(w) {
		return
	}
	cfg := *a.cfg()
	if !signinRenewalEnabled(cfg) {
		failWithCode(w, http.StatusForbidden, ErrSigninRenewalDisabled, signinRenewalDisabledMessage)
		return
	}
	p := current(r)
	if a.requireNonEmbyAdmin(w, r, p.User) {
		return
	}
	if rejectSelfServiceRenewalWithoutEmby(w, p.User) {
		return
	}
	if expiryIsPermanent(p.User.ExpiredAt) {
		failWithCode(w, http.StatusConflict, ErrConflict, signinRenewalPermanentMessage)
		return
	}
	cost := cfg.SigninRenewalCost
	days := cfg.SigninRenewalDays
	u, si, err := a.spendSigninRenewal(p.User.UID, cost, days, time.Now(), false)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrEmbyRequired):
			failWithCode(w, http.StatusConflict, ErrRenewRequiresEmby, "请先绑定或开通 Emby 账号，再使用积分续期")
		case errors.Is(err, store.ErrInsufficientPoints):
			failWithCode(w, http.StatusConflict, ErrSigninInsufficientPoints, signinRenewalInsufficientMessage)
		case errors.Is(err, store.ErrConflict):
			failWithCode(w, http.StatusConflict, ErrConflict, signinRenewalUnavailableMessage)
		default:
			statusFromError(w, err)
		}
		return
	}
	a.audit(r, "renew_with_signin_points", "user", 0, map[string]any{"spent_points": cost, "days": days})
	renewalPayload := signinRenewalPayload(cfg, si.Points)
	a.attachSigninAutoRenewalToRenewal(renewalPayload, cfg, u)
	ok(w, signinRenewalSuccessMessage, map[string]any{
		"currency_name":    signinCurrencyName(cfg),
		"spent_points":     cost,
		"remaining_points": si.Points,
		"renewal":          renewalPayload,
		"expire_status":    expireStatus(u.ExpiredAt),
		"expired_at":       publicExpiryUnix(u.ExpiredAt),
		"user":             publicUser(u),
	})
}

func signinCurrencyName(cfg config.Config) string {
	if strings.TrimSpace(cfg.SigninCurrencyName) == "" {
		return signinDefaultCurrencyName
	}
	return strings.TrimSpace(cfg.SigninCurrencyName)
}

func signinConfigPayload(cfg config.Config) map[string]any {
	return map[string]any{
		"enabled":              cfg.SigninEnabled,
		"currency_name":        signinCurrencyName(cfg),
		"daily_min":            signinDailyMin(cfg),
		"daily_max":            signinDailyMax(cfg),
		"streak_bonus_enabled": cfg.SigninStreakBonusEnabled,
		"bonus_table":          signinBonusTable(cfg),
		"reset_after_miss":     cfg.SigninResetAfterMiss,
		"renewal":              signinRenewalPayload(cfg, 0),
	}
}

func signinSummaryPayload(cfg config.Config, si store.Signin) map[string]any {
	today := time.Now().Format("2006-01-02")
	longest := si.LongestStreak
	if longest < si.Streak {
		longest = si.Streak
	}
	for _, record := range si.Records {
		if record.Streak > longest {
			longest = record.Streak
		}
	}
	nextBonusInDays, nextBonusPoints := signinNextBonus(cfg, si.Streak)
	return map[string]any{
		"enabled":            cfg.SigninEnabled,
		"currency_name":      signinCurrencyName(cfg),
		"current_points":     si.Points,
		"current_streak":     si.Streak,
		"longest_streak":     longest,
		"total_points":       si.Points,
		"last_signin_date":   emptyNil(si.LastSignin),
		"today_signed":       si.LastSignin == today,
		"next_bonus_in_days": nextBonusInDays,
		"next_bonus_points":  nextBonusPoints,
		"renewal":            signinRenewalPayload(cfg, si.Points),
	}
}

func signinActionPayload(cfg config.Config, si store.Signin, created bool, dailyPoints, bonusPoints int) map[string]any {
	totalToday := dailyPoints + bonusPoints
	payload := signinSummaryPayload(cfg, si)
	payload["created"] = created
	payload["today_signed"] = true
	payload["daily_points"] = dailyPoints
	payload["bonus_points"] = bonusPoints
	payload["total_today"] = totalToday
	return payload
}

func signinDailyMin(cfg config.Config) int {
	if cfg.SigninDailyMin <= 0 {
		return 1
	}
	return cfg.SigninDailyMin
}

func signinDailyMax(cfg config.Config) int {
	min := signinDailyMin(cfg)
	if cfg.SigninDailyMax < min {
		return min
	}
	return cfg.SigninDailyMax
}

func signinRenewalEnabled(cfg config.Config) bool {
	return cfg.SigninRenewalEnabled && cfg.SigninRenewalCost > 0 && cfg.SigninRenewalDays > 0
}

func signinAutoRenewalEnabled(cfg config.Config) bool {
	return cfg.SigninEnabled && signinRenewalEnabled(cfg) && cfg.SigninAutoRenewalEnabled
}

func signinRenewalPayload(cfg config.Config, points int) map[string]any {
	cost := cfg.SigninRenewalCost
	if cost < 0 {
		cost = 0
	}
	days := cfg.SigninRenewalDays
	if days < 0 {
		days = 0
	}
	enabled := signinRenewalEnabled(cfg)
	return map[string]any{
		"enabled":              enabled,
		"cost":                 cost,
		"days":                 days,
		"affordable":           enabled && points >= cost,
		"auto_renewal_enabled": signinAutoRenewalEnabled(cfg),
	}
}

func (a *App) attachSigninAutoRenewal(payload map[string]any, cfg config.Config, points int, user store.User) {
	renewal, _ := payload["renewal"].(map[string]any)
	if renewal == nil {
		renewal = signinRenewalPayload(cfg, points)
		payload["renewal"] = renewal
	}
	a.attachSigninAutoRenewalToRenewal(renewal, cfg, user)
}

func (a *App) attachSigninAutoRenewalToRenewal(renewal map[string]any, cfg config.Config, user store.User) {
	renewal["auto_renewal_user_enabled"] = user.SigninAutoRenewal
	renewal["auto_renewal_available"] = signinAutoRenewalEnabled(cfg) &&
		!a.userIsProtected(user) &&
		validateSelfServiceRenewalTarget(user) == nil &&
		user.ExpiredAt > 0 &&
		!expiryIsPermanent(user.ExpiredAt)
}

func (a *App) spendSigninRenewal(uid int64, cost, days int, now time.Time, automatic bool) (store.User, store.Signin, error) {
	return a.store().SpendSigninPointsAndUpdateUser(uid, cost, func(u *store.User) error {
		if err := validateSelfServiceRenewalTarget(*u); err != nil {
			return err
		}
		if expiryIsPermanent(u.ExpiredAt) {
			return store.ErrConflict
		}
		if automatic {
			if !u.SigninAutoRenewal || !u.Active || a.userIsProtected(*u) || u.ExpiredAt <= 0 || u.ExpiredAt >= now.Unix() {
				return store.ErrConflict
			}
		}
		renewExpiryAndReactivate(u, addDaysToExpiry(u.ExpiredAt, days, now))
		return nil
	})
}

// signinDailyPoints 用 crypto/rand 生成 [min, max] 区间内的整数。
// 注意这里的 fallback 行为：crypto/rand 失败时返回 min（不是 panic），
// 因为签到积分相对低风险——给个最小值不构成安全漏洞，避免熵故障导致用户
// 拿不到签到奖励。和 randomCode 的"必须不可预测"语义不同。
func signinDailyPoints(cfg config.Config) int {
	min := signinDailyMin(cfg)
	max := signinDailyMax(cfg)
	if max <= min {
		return min
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		return min
	}
	return min + int(n.Int64())
}

func signinBonusForStreak(cfg config.Config, streak int) int {
	if !cfg.SigninStreakBonusEnabled || streak <= 0 {
		return 0
	}
	for i, day := range cfg.SigninStreakBonusDays {
		if day == streak && i < len(cfg.SigninStreakBonusPoints) {
			points := cfg.SigninStreakBonusPoints[i]
			if points > 0 {
				return points
			}
			return 0
		}
	}
	return 0
}

func signinBonusTable(cfg config.Config) []map[string]any {
	table := make([]map[string]any, 0, len(cfg.SigninStreakBonusDays))
	for i, day := range cfg.SigninStreakBonusDays {
		if day <= 0 || i >= len(cfg.SigninStreakBonusPoints) {
			continue
		}
		points := cfg.SigninStreakBonusPoints[i]
		if points <= 0 {
			continue
		}
		table = append(table, map[string]any{"streak_days": day, "bonus_points": points})
	}
	return table
}

func signinNextBonus(cfg config.Config, streak int) (any, any) {
	if !cfg.SigninStreakBonusEnabled {
		return nil, nil
	}
	nextDays := 0
	nextPoints := 0
	for i, day := range cfg.SigninStreakBonusDays {
		if day <= streak || i >= len(cfg.SigninStreakBonusPoints) || cfg.SigninStreakBonusPoints[i] <= 0 {
			continue
		}
		if nextDays == 0 || day < nextDays {
			nextDays = day
			nextPoints = cfg.SigninStreakBonusPoints[i]
		}
	}
	if nextDays == 0 {
		return nil, nil
	}
	return nextDays - streak, nextPoints
}
