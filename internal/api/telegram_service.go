package api

import (
	"context"
	"errors"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

type telegramService struct {
	app *App
}

type telegramStatusResult struct {
	Bound                 bool   `json:"bound"`
	TelegramID            any    `json:"telegram_id"`
	TelegramIDFull        any    `json:"telegram_id_full"`
	TelegramUsername      string `json:"telegram_username"`
	ForceBind             bool   `json:"force_bind"`
	CanUnbind             bool   `json:"can_unbind"`
	CanChange             bool   `json:"can_change"`
	RebindApproved        bool   `json:"rebind_approved"`
	PendingRebindRequest  bool   `json:"pending_rebind_request"`
	RebindRequestStatus   any    `json:"rebind_request_status"`
	RebindRequestID       any    `json:"rebind_request_id"`
	RebindingInProgress   bool   `json:"rebinding_in_progress"`
}

type telegramUnbindResult struct {
	User    *store.User `json:"user"`
	Message string      `json:"message"`
}

type telegramRosterStatsResult struct {
	Total      int  `json:"total"`
	Active     int  `json:"active"`
	Bound      int  `json:"bound"`
	Unbound    int  `json:"unbound"`
	KnownOnly  bool `json:"known_only"`
}

func (s *telegramService) status(u store.User) telegramStatusResult {
	forceBind := s.app.cfg().ForceBindTelegram
	admin := u.Role == store.RoleAdmin
	canUnbind := admin
	canChange := true
	pendingRebind := false
	rebindApproved := false
	var rebindStatus any
	var rebindID any

	if latestReq, hasReq := s.app.store().UserLatestRebindRequest(u.UID); hasReq {
		rebindStatus = latestReq.Status
		rebindID = latestReq.ID
		switch latestReq.Status {
		case "pending":
			pendingRebind = true
			if !admin {
				canChange = false
			}
		case "approved":
			rebindApproved = true
			canUnbind = true
		}
	}

	return telegramStatusResult{
		Bound:                u.TelegramID != 0,
		TelegramID:           nullableInt(u.TelegramID),
		TelegramIDFull:       nullableInt(u.TelegramID),
		TelegramUsername:     u.TelegramUsername,
		ForceBind:            forceBind,
		CanUnbind:            canUnbind,
		CanChange:            canChange,
		RebindApproved:       rebindApproved,
		PendingRebindRequest: pendingRebind,
		RebindRequestStatus:  rebindStatus,
		RebindRequestID:      rebindID,
		RebindingInProgress:  u.RebindingInProgress,
	}
}

func (s *telegramService) unbind(ctx context.Context, u store.User) (telegramUnbindResult, error) {
	var consumeRebindID int64
	if u.Role != store.RoleAdmin {
		latestReq, hasReq := s.app.store().UserLatestRebindRequest(u.UID)
		if !hasReq || latestReq.Status != "approved" {
			return telegramUnbindResult{}, errors.New("更换 Telegram 需要先提交换绑申请并经管理员批准")
		}
		consumeRebindID = latestReq.ID
	}

	updated, err := s.app.store().UpdateUser(u.UID, func(u *store.User) error {
		u.TelegramID = 0
		u.TelegramUsername = ""
		return nil
	})
	if err != nil {
		return telegramUnbindResult{}, err
	}

	s.app.cleanupUserTelegramResidue(u.UID, u.TelegramID)

	if consumeRebindID > 0 {
		_ = s.app.store().ConsumeRebindRequest(consumeRebindID)
	}

	_, _ = s.app.store().UpdateUser(u.UID, func(u2 *store.User) error {
		if u2.Role != store.RoleAdmin {
			u2.RebindingInProgress = true
			u2.RebindingSince = time.Now().Unix()
		}
		return nil
	})

	if updated.EmbyID != "" {
		sideCtx, sideCancel := schedulerSideEffectContext(ctx)
		_, _ = s.app.disableRemoteEmbyForWebState(sideCtx, updated)
		sideCancel()
	}

	return telegramUnbindResult{
		User:    &updated,
		Message: "Telegram unbound. rebinding required",
	}, nil
}

func (s *telegramService) rosterStats() (telegramRosterStatsResult, error) {
	chats := telegramChatIDs(s.app.cfg().TelegramGroupIDs)
	chatID := ""
	if len(chats) > 0 {
		chatID = chats[0]
	}

	stats, err := s.app.store().TelegramRosterStats(chatID)
	if err != nil {
		return telegramRosterStatsResult{}, err
	}

	entries, err := s.app.store().TelegramRoster(chatID, true)
	if err != nil {
		return telegramRosterStatsResult{}, err
	}

	bound := 0
	unbound := 0

	if len(entries) > 0 {
		for _, entry := range entries {
			if entry.IsBot {
				continue
			}
			if _, okUser := s.app.store().FindUserByTelegramID(entry.TelegramID); okUser {
				bound++
			} else {
				unbound++
			}
		}
	} else {
		for _, u := range s.app.store().ListUsers() {
			if u.TelegramID == 0 {
				continue
			}
			bound++
		}
		stats["total"] = bound
		stats["active"] = bound
		stats["known_only"] = true
	}

	stats["bound"] = bound
	stats["unbound"] = unbound

	return telegramRosterStatsResult{
		Total:     int(numeric(stats["total"])),
		Active:    int(numeric(stats["active"])),
		Bound:     bound,
		Unbound:   unbound,
		KnownOnly: boolish(stats["known_only"]),
	}, nil
}

func (a *App) telegram() *telegramService {
	return &telegramService{app: a}
}
