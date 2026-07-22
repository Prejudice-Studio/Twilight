package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

func writePersistedStateForTest(t *testing.T, app *App, mutate func(*store.State)) {
	t.Helper()
	raw, err := app.store().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var state store.State
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	mutate(&state)
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteFileAtomicSync(app.store().Path(), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAdminRegcodeListRefreshesPersistedState(t *testing.T) {
	app := newTestApp(t)
	admin := registerAndLogin(t, app, "admin", "Admin123456")
	if err := app.store().UpsertRegCode(store.RegCode{Code: "STALE-REG", Type: 1, Days: 7, ValidityTime: -1, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	writePersistedStateForTest(t, app, func(state *store.State) {
		delete(state.RegCodes, "STALE-REG")
	})

	rr := doJSON(app, http.MethodGet, "/api/v1/admin/regcodes?search=STALE-REG", ``, admin)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin regcode list status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode regcode list: %v body=%s", err, rr.Body.String())
	}
	if resp.Data.Total != 0 {
		t.Fatalf("admin regcode list served stale deleted code, total=%d body=%s", resp.Data.Total, rr.Body.String())
	}
}

func TestAdminTicketListAndDetailRefreshPersistedState(t *testing.T) {
	app := newTestApp(t)
	enableTicketSystem(t, app, nil)
	admin := registerAndLogin(t, app, "admin", "Admin123456")
	user := registerAndLogin(t, app, "ticket-user", "User12345678")
	id := createTicket(t, app, "stale ticket", "content", user)
	writePersistedStateForTest(t, app, func(state *store.State) {
		delete(state.Tickets, id)
	})

	list := doJSON(app, http.MethodGet, "/api/v1/admin/tickets?all=1&page=1&per_page=20", ``, admin)
	if list.Code != http.StatusOK {
		t.Fatalf("admin ticket list status=%d body=%s", list.Code, list.Body.String())
	}
	var listResp struct {
		Data struct {
			Tickets []struct {
				ID int64 `json:"id"`
			} `json:"tickets"`
		} `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode ticket list: %v body=%s", err, list.Body.String())
	}
	for _, ticket := range listResp.Data.Tickets {
		if ticket.ID == id {
			t.Fatalf("admin ticket list served stale deleted ticket %d: body=%s", id, list.Body.String())
		}
	}

	detail := doJSON(app, http.MethodGet, "/api/v1/admin/tickets/"+strconv.FormatInt(id, 10), ``, admin)
	if detail.Code != http.StatusNotFound {
		t.Fatalf("admin ticket detail should refresh and return 404, got %d body=%s", detail.Code, detail.Body.String())
	}
}

func TestRegisterAndRenewRefreshDeletedRegCode(t *testing.T) {
	app := newTestApp(t)
	registerAndLogin(t, app, "admin", "Admin123456")
	userCookies := registerAndLogin(t, app, "renew-user", "User123456")
	app.cfg().RegisterCodeLimit = true
	if err := app.store().UpsertRegCode(store.RegCode{Code: "REGISTER-STALE", Type: 1, Days: 7, ValidityTime: -1, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	writePersistedStateForTest(t, app, func(state *store.State) {
		delete(state.RegCodes, "REGISTER-STALE")
	})

	register := doJSON(app, http.MethodPost, "/api/v1/users/register", `{"username":"stale-register","password":"User123456","reg_code":"REGISTER-STALE"}`, nil)
	if register.Code != http.StatusBadRequest {
		t.Fatalf("register should refresh deleted regcode and fail, got %d body=%s", register.Code, register.Body.String())
	}

	if err := app.store().UpsertRegCode(store.RegCode{Code: "RENEW-STALE", Type: 2, Days: 7, ValidityTime: -1, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	writePersistedStateForTest(t, app, func(state *store.State) {
		delete(state.RegCodes, "RENEW-STALE")
	})
	renew := doJSON(app, http.MethodPost, "/api/v1/users/me/renew", `{"reg_code":"RENEW-STALE"}`, userCookies)
	if renew.Code != http.StatusBadRequest {
		t.Fatalf("renew should refresh deleted regcode and fail, got %d body=%s", renew.Code, renew.Body.String())
	}
}

func TestInviteReadsRefreshPersistedState(t *testing.T) {
	app := newTestApp(t)
	admin := registerAndLogin(t, app, "admin", "Admin123456")
	parentCookies := registerAndLogin(t, app, "invite-parent", "Parent123456")
	parent, ok := app.store().FindUserByUsername("invite-parent")
	if !ok {
		t.Fatal("parent user missing")
	}
	if err := app.store().UpsertInviteCode(store.InviteCode{Code: "STALE-INVITE", UID: parent.UID, InviterUID: parent.UID, Days: 7, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	writePersistedStateForTest(t, app, func(state *store.State) {
		delete(state.InviteCodes, "STALE-INVITE")
	})

	check := doJSON(app, http.MethodGet, "/api/v1/invite/check?code=STALE-INVITE", ``, nil)
	if check.Code != http.StatusNotFound {
		t.Fatalf("invite check should refresh deleted invite, got %d body=%s", check.Code, check.Body.String())
	}
	ownCodes := doJSON(app, http.MethodGet, "/api/v1/invite/codes", ``, parentCookies)
	if ownCodes.Code != http.StatusOK {
		t.Fatalf("invite codes status=%d body=%s", ownCodes.Code, ownCodes.Body.String())
	}
	var ownResp struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(ownCodes.Body.Bytes(), &ownResp); err != nil {
		t.Fatal(err)
	}
	if ownResp.Data.Total != 0 {
		t.Fatalf("own invite code list served stale deleted invite, body=%s", ownCodes.Body.String())
	}
	adminCodes := doJSON(app, http.MethodGet, "/api/v1/admin/invite/codes", ``, admin)
	if adminCodes.Code != http.StatusOK {
		t.Fatalf("admin invite codes status=%d body=%s", adminCodes.Code, adminCodes.Body.String())
	}
	var adminResp struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(adminCodes.Body.Bytes(), &adminResp); err != nil {
		t.Fatal(err)
	}
	if adminResp.Data.Total != 0 {
		t.Fatalf("admin invite code list served stale deleted invite, body=%s", adminCodes.Body.String())
	}
}

func TestAdminInviteQuickMaintenanceDetachesAndRenews(t *testing.T) {
	app := newTestApp(t)
	admin := registerAndLogin(t, app, "admin", "Admin123456")
	now := time.Now()
	parent, err := app.store().CreateUser(store.User{Username: "quick-parent", PasswordHash: "x", Role: store.RoleNormal, Active: true, ExpiredAt: now.AddDate(0, 0, 30).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	child, err := app.store().CreateUser(store.User{Username: "quick-child", PasswordHash: "x", Role: store.RoleNormal, Active: true, ExpiredAt: now.AddDate(0, 0, -1).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertInviteCode(store.InviteCode{Code: "QUICK-INVITE", UID: parent.UID, InviterUID: parent.UID, Days: 7, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.store().ConsumeInviteCode("QUICK-INVITE", child.UID); err != nil {
		t.Fatal(err)
	}
	resp := doJSON(app, http.MethodPost, "/api/v1/admin/invite/quick-maintenance", `{"confirm":"INVITE_QUICK_MAINTENANCE","scope":"all","detach":true,"renew_days":10}`, admin)
	if resp.Code != http.StatusOK {
		t.Fatalf("quick maintenance status=%d body=%s", resp.Code, resp.Body.String())
	}
	if _, ok := app.store().ParentOf(child.UID); ok {
		t.Fatal("quick maintenance should detach invite relation")
	}
	updated, _ := app.store().User(child.UID)
	if !updated.Active || updated.ExpiredAt < now.AddDate(0, 0, 9).Unix() {
		t.Fatalf("quick maintenance should renew and reactivate child: %#v", updated)
	}
	invite, ok := app.store().InviteCode("QUICK-INVITE")
	if !ok {
		t.Fatal("detach should clear usage but keep invite code record")
	}
	if invite.UsedByUID != 0 || invite.Used || invite.UseCount != 0 || !invite.Active {
		t.Fatalf("quick detach should clear invite usage: %#v", invite)
	}
}
