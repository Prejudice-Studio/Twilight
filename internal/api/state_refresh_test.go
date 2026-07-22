package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

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
