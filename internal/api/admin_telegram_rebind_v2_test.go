package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/prejudice-studio/twilight/internal/store"
)

func TestV2AdminTelegramRebindResourcesUseGuardedReviewFlow(t *testing.T) {
	app := newTestApp(t)
	_ = registerAdmin(t, app, "admin", "Admin123456")
	userCookies := registerAndLogin(t, app, "v2rebinduser", "User123456")
	headers := map[string]string{"X-Twilight-Client": "webui"}

	// 换绑申请的前置条件是"当前账号已绑定 Telegram"，否则后端直接 TG_NOT_BOUND。
	// 绑定只能由 Telegram 侧确认流程写入，测试里直接在持久层置上 TG 身份。
	var rebindUID int64
	for _, u := range app.store().ListUsers() {
		if u.Username == "v2rebinduser" {
			rebindUID = u.UID
			break
		}
	}
	if rebindUID == 0 {
		t.Fatalf("test user v2rebinduser not found after registration")
	}
	if _, err := app.store().UpdateUser(rebindUID, func(u *store.User) error {
		u.TelegramID = 42001
		u.TelegramUsername = "v2rebindtg"
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	created := doJSONWithHeaders(app, http.MethodPost, "/api/v1/users/me/telegram/rebind-request", `{"reason":"lost account"}`, userCookies, headers)
	if created.Code != http.StatusOK {
		t.Fatalf("create rebind request status=%d body=%s", created.Code, created.Body.String())
	}
	pending := app.store().ListRebindRequests("pending")
	if len(pending) != 1 {
		t.Fatalf("pending requests=%d", len(pending))
	}

	adminCookies := loginCookies(t, app, "admin", "Admin123456")
	list := doJSONWithHeaders(app, http.MethodGet, "/api/v2/admin/telegram/rebind-requests?status=pending&page=1&per_page=20", "", adminCookies, headers)
	if list.Code != http.StatusOK || list.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("v2 list status=%d cache=%q body=%s", list.Code, list.Header().Get("Cache-Control"), list.Body.String())
	}

	approved := doJSONWithHeaders(app, http.MethodPost, fmt.Sprintf("/api/v2/admin/telegram/rebind-requests/%d/approve", pending[0].ID), `{"admin_note":"checked"}`, adminCookies, headers)
	if approved.Code != http.StatusOK {
		t.Fatalf("v2 approve status=%d body=%s", approved.Code, approved.Body.String())
	}
	request, ok := app.store().UserLatestRebindRequest(pending[0].UID)
	if !ok || request.Status != "approved" || request.AdminNote != "checked" || request.ReviewerUID == 0 {
		t.Fatalf("unexpected reviewed request: %#v found=%v", request, ok)
	}

	forbidden := doJSONWithHeaders(app, http.MethodGet, "/api/v2/admin/telegram/rebind-requests", "", userCookies, headers)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("non-admin v2 list status=%d body=%s", forbidden.Code, forbidden.Body.String())
	}
}
