package api

// 公告（Announcement）域 handler。从 handlers.go 抽出来的目的：
//   - handlers.go 长期聚合 9+ 业务域 2000+ 行；公告增删改查 + 渲染白名单
//     原本散在中段，新人接手时无法快速定位"前端 /admin/announcements 是哪
//     条链路驱动的"；
//   - 渲染模式（plain / markdown / bbcode）必须经 safeAnnouncementRenderMode
//     白名单兜底，否则前端 markdown viewer 会把恶意 HTML 渲染出来；保留这
//     个工具与 handler 同文件，避免新人调用时漏掉白名单；
//   - int64Value 仅在公告 expires_at 字段使用，跟随域迁移。
//
// 修改时务必保持与原有契约一致：
//   - admin/普通用户分别走 handleAdminAnnouncements / handleAnnouncements
//     —— 后者只能拿到 visible=true 的子集；
//   - failWithCode 走 errcode.go，不在这里临时新增；
//   - publicUser 不在公告里直接展示作者，仅记录 CreatedByUID 作为审计。

import (
	"net/http"
	"strings"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) handleAdminAnnouncements(w http.ResponseWriter, r *http.Request, _ Params) {
	anns := a.store().ListAnnouncements(true)
	ok(w, "OK", map[string]any{"announcements": anns, "total": len(anns)})
}

func (a *App) handleAnnouncements(w http.ResponseWriter, r *http.Request, _ Params) {
	anns := a.store().ListAnnouncements(false)
	ok(w, "OK", map[string]any{"announcements": anns, "total": len(anns)})
}

func (a *App) handleCreateAnnouncement(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	ann, err := a.store().UpsertAnnouncement(store.Announcement{
		Title:        firstNonEmpty(stringValue(payload, "title"), "鍏憡"),
		Content:      stringValue(payload, "content"),
		Visible:      boolValue(payload, "visible", true),
		Level:        firstNonEmpty(stringValue(payload, "level"), "info"),
		RenderMode:   safeAnnouncementRenderMode(stringValue(payload, "render_mode")),
		Pinned:       boolValue(payload, "pinned", false),
		CreatedByUID: current(r).User.UID,
		ExpiredAt:    int64Value(payload, "expires_at", int64Value(payload, "expired_at", 0)),
	})
	if statusFromError(w, err) {
		return
	}
	created(w, "announcement created", ann)
}

func (a *App) handleUpdateAnnouncement(w http.ResponseWriter, r *http.Request, params Params) {
	id, _ := int64Param(params, "announcement_id")
	payload := decodeMap(r)
	existing := store.Announcement{ID: id, Title: "鍏憡", Level: "info", Visible: true, RenderMode: "plain"}
	for _, ann := range a.store().ListAnnouncements(true) {
		if ann.ID == id {
			existing = ann
			break
		}
	}
	ann, err := a.store().UpsertAnnouncement(store.Announcement{
		ID:           id,
		Title:        firstNonEmpty(stringValue(payload, "title"), existing.Title, "鍏憡"),
		Content:      firstNonEmpty(stringValue(payload, "content"), existing.Content),
		Visible:      boolValue(payload, "visible", existing.Visible),
		Level:        firstNonEmpty(stringValue(payload, "level"), existing.Level, "info"),
		RenderMode:   safeAnnouncementRenderMode(firstNonEmpty(stringValue(payload, "render_mode"), existing.RenderMode)),
		Pinned:       boolValue(payload, "pinned", existing.Pinned),
		CreatedByUID: existing.CreatedByUID,
		CreatedAt:    existing.CreatedAt,
		ExpiredAt:    int64Value(payload, "expires_at", int64Value(payload, "expired_at", existing.ExpiredAt)),
	})
	if statusFromError(w, err) {
		return
	}
	ok(w, "announcement updated", ann)
}

func safeAnnouncementRenderMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "markdown", "bbcode":
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return "plain"
	}
}

func int64Value(payload map[string]any, key string, fallback int64) int64 {
	if _, ok := payload[key]; !ok {
		return fallback
	}
	return numeric(payload[key])
}

func (a *App) handleDeleteAnnouncement(w http.ResponseWriter, r *http.Request, params Params) {
	id, _ := int64Param(params, "announcement_id")
	if statusFromError(w, a.store().DeleteAnnouncement(id)) {
		return
	}
	ok(w, "announcement deleted", nil)
}
