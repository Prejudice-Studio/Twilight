package api

import (
	"crypto/rand"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/store"
)

// 签到子系统 handlers + 配置/计算辅助。
// 从 handlers.go 拆出，让"签到 + 连签奖励"这条业务线独立成文件，
// 避免 handlers.go 持续往 3000+ 行膨胀。
// 拆分原则：仅做「按业务域归位」，不修改任何对外行为；现有 router/路由表不变。

func (a *App) handleSigninConfig(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", signinConfigPayload(*a.cfg()))
}

func (a *App) handleSigninMe(w http.ResponseWriter, r *http.Request, _ Params) {
	si := a.store().Signin(current(r).User.UID)
	ok(w, "OK", signinSummaryPayload(*a.cfg(), si))
}

func (a *App) handleSignin(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.cfg().SigninEnabled {
		failWithCode(w, http.StatusForbidden, ErrSigninDisabled, "签到功能未开启")
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
	if createdToday {
		ok(w, "签到成功", payload)
		return
	}
	ok(w, "今日已签到", payload)
}

func (a *App) handleSigninHistory(w http.ResponseWriter, r *http.Request, _ Params) {
	si := a.store().Signin(current(r).User.UID)
	records := append([]store.SigninRecord(nil), si.Records...)
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt > records[j].CreatedAt })
	limit := queryInt(r, "limit", 30)
	if limit <= 0 || limit > 365 {
		limit = 30
	}
	if len(records) > limit {
		records = records[:limit]
	}
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

func signinCurrencyName(cfg config.Config) string {
	if strings.TrimSpace(cfg.SigninCurrencyName) == "" {
		return "积分"
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
