package api

import (
	"net/http"
	"time"
)

// V2 的基础读取只描述协议能力，不返回配置秘密、完整路由库存或外部服务
// 的私有诊断。业务模块迁移后再把能力项扩展到对应的版本化 application service。
type v2Capabilities struct {
	APIVersion            string            `json:"api_version"`
	CompatibleAPIVersions []string          `json:"compatible_api_versions"`
	ServerVersion         string            `json:"server_version"`
	Features              map[string]bool   `json:"features"`
	Limits                map[string]int64  `json:"limits"`
	Links                 map[string]string `json:"links"`
}

// handleV2Health 是 V2 的公共 API liveness 端点。它只确认 API 进程仍能处理
// 请求，不探测 PostgreSQL、Emby、Telegram 或其它外部依赖；依赖健康度由
// 管理员专用的独立端点负责。
func (a *App) handleV2Health(w http.ResponseWriter, _ *http.Request, _ Params) {
	ok(w, "OK", map[string]any{
		"api_version":    "v2",
		"status":         "ok",
		"server_version": a.cfg().Version,
		"timestamp":      time.Now().Unix(),
	})
}

// handleV2Capabilities 提供前端选择协议能力所需的非敏感信息。布尔 feature
// 是公开产品能力，不包含 token、URL、数据库信息、管理员名单或用户数据。
func (a *App) handleV2Capabilities(w http.ResponseWriter, _ *http.Request, _ Params) {
	cfg := a.cfg()
	ok(w, "OK", v2Capabilities{
		APIVersion:            "v2",
		CompatibleAPIVersions: []string{"v1"},
		ServerVersion:         cfg.Version,
		Features: map[string]bool{
			"register":      cfg.RegisterEnabled,
			"emby":          cfg.EmbyURL != "" && cfg.EmbyToken != "",
			"telegram":      cfg.TelegramMode,
			"bangumi":       cfg.BangumiEnabled,
			"media_request": cfg.MediaRequestEnabled,
			"signin":        cfg.SigninEnabled,
			"invite":        cfg.InviteEnabled,
			"email":         emailConfigured(cfg),
			"tickets":       cfg.TicketSystemEnabled,
			"activity_logs": true,
			"viewing_stats": false,
		},
		Limits: map[string]int64{
			"max_upload_size":        cfg.MaxUploadSize,
			"ticket_image_max_size":  cfg.TicketImageMaxSize,
			"ticket_image_max_count": int64(cfg.TicketImageMaxCount),
		},
		Links: map[string]string{
			"openapi": "/api/v1/openapi.json",
			"docs":    "/api/v1/docs",
		},
	})
}
