package api

import "net/http"

// handleV2Announcements 是**公开**公告列表：只有可见公告与总数，不含任何按用户
// 计算的强制已读状态。它挂在 AuthPublic 路由上，匿名可读，与 V1 的
// /api/v1/announcements 保持同语义。
//
// 曾经错挂成 handleAnnouncementsMe（私有载荷）。AuthPublic 下 authenticate 直接
// 放行、不解析会话，于是连登录用户在这里拿到的也是 UID=0 的强制已读状态——永远
// 为空，强制已读公告不会弹给任何人。按用户计算的载荷只能走 /me/announcements。
func (a *App) handleV2Announcements(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAnnouncements(w, r, p)
}

// handleV2AnnouncementsMe 是登录用户视角：可见公告 + 该用户未确认的强制已读公告。
func (a *App) handleV2AnnouncementsMe(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleAnnouncementsMe(w, r, p)
}

func (a *App) handleV2AckAnnouncements(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	a.handleAckAnnouncements(w, r, p)
}
