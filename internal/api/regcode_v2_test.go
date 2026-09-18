package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/prejudice-studio/twilight/internal/store"
)

func TestV2AdminRegcodeResourcesUseBoundedResourceShape(t *testing.T) {
	app := newTestApp(t)
	admin := registerAdmin(t, app, "v2-reg-admin", "Admin123456")


	created := doJSON(app, http.MethodPost, "/api/v2/admin/regcodes", `{"type":1,"days":30,"validity_time":-1,"use_count_limit":-1,"count":1,"note":"v2"}`, admin)
	if created.Code != http.StatusOK {
		t.Fatalf("v2 create status=%d body=%s", created.Code, created.Body.String())
	}
	var createdEnvelope struct {
		Data struct {
			Codes []string `json:"codes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdEnvelope); err != nil {
		t.Fatalf("decode v2 create: %v body=%s", err, created.Body.String())
	}
	if len(createdEnvelope.Data.Codes) != 1 {
		t.Fatalf("expected one generated code, got %#v", createdEnvelope.Data.Codes)
	}
	code := createdEnvelope.Data.Codes[0]

	list := doJSON(app, http.MethodGet, "/api/v2/admin/regcodes?page=1&per_page=20", "", admin)
	if list.Code != http.StatusOK {
		t.Fatalf("v2 list status=%d body=%s", list.Code, list.Body.String())
	}
	var listEnvelope struct {
		Data struct {
			Items      []map[string]any `json:"items"`
			Pagination struct {
				Page       int `json:"page"`
				PerPage    int `json:"per_page"`
				TotalPages int `json:"total_pages"`
			} `json:"pagination"`
		} `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listEnvelope); err != nil {
		t.Fatalf("decode v2 list: %v body=%s", err, list.Body.String())
	}
	if listEnvelope.Data.Pagination.Page != 1 || listEnvelope.Data.Pagination.PerPage != 20 || listEnvelope.Data.Pagination.TotalPages < 1 || len(listEnvelope.Data.Items) != 1 {
		t.Fatalf("unexpected v2 list shape: %+v", listEnvelope.Data)
	}
	if listEnvelope.Data.Items[0]["code"] != code || listEnvelope.Data.Items[0]["validity_time"] != float64(-1) {
		t.Fatalf("unexpected v2 regcode item: %#v", listEnvelope.Data.Items[0])
	}

	itemPath := "/api/v2/admin/regcodes/" + code
	item := doJSON(app, http.MethodGet, itemPath, "", admin)
	if item.Code != http.StatusOK || !json.Valid(item.Body.Bytes()) {
		t.Fatalf("v2 item status=%d body=%s", item.Code, item.Body.String())
	}
	patched := doJSON(app, http.MethodPatch, itemPath, `{"note":"updated by v2","days":45}`, admin)
	if patched.Code != http.StatusOK {
		t.Fatalf("v2 patch status=%d body=%s", patched.Code, patched.Body.String())
	}
	if current, ok := app.store().RegCode(code); !ok || current.Note != "updated by v2" || current.Days != 45 {
		t.Fatalf("v2 patch did not persist the shared state transition: %#v", current)
	}
	usage := doJSON(app, http.MethodGet, itemPath+"/usage", "", admin)
	if usage.Code != http.StatusOK {
		t.Fatalf("v2 usage status=%d body=%s", usage.Code, usage.Body.String())
	}
	if !json.Valid(usage.Body.Bytes()) {
		t.Fatalf("v2 usage returned invalid JSON: %s", usage.Body.String())
	}

	deleted := doJSON(app, http.MethodDelete, itemPath, "", admin)
	if deleted.Code != http.StatusOK {
		t.Fatalf("v2 delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	if _, ok := app.store().RegCode(code); ok {
		t.Fatalf("v2 delete left code in store")
	}
}

func TestRegcodeListFilterKeepsQueryAndBatchDeletionInSync(t *testing.T) {
	query := url.Values{
		"type":   {"all"},
		"status": {"active"},
		"source": {"admin"},
		"search": {"summer"},
	}
	fromQuery := regcodeListFilterFromQuery(query)
	fromPayload := regcodeListFilterFromPayload(map[string]any{
		"filter": map[string]any{"type": "all", "status": "active", "source": "admin", "search": "summer"},
	})
	codes := []store.RegCode{
		{Code: "SUMMER-ADMIN", Type: 1, Note: "Summer event", Active: true},
		{Code: "SUMMER-INVITE", Type: 1, Note: "Summer event", Source: "invite", Active: true},
		{Code: "SUMMER-DISABLED", Type: 2, Note: "Summer event", Active: false},
		{Code: "WINTER-ADMIN", Type: 3, Note: "Winter event", Active: true},
	}
	for _, code := range codes {
		if fromQuery.matches(code) != fromPayload.matches(code) {
			t.Fatalf("query and batch filter diverged for %q", code.Code)
		}
	}
	if !fromQuery.matches(codes[0]) || fromQuery.matches(codes[1]) || fromQuery.matches(codes[2]) || fromQuery.matches(codes[3]) {
		t.Fatalf("unexpected shared filter result")
	}
}

func TestV2AdminRegcodeResourcesRejectNormalUsers(t *testing.T) {
	app := newTestApp(t)
	user := registerAndLogin(t, app, "v2-reg-user", "User12345678")
	if rr := doJSON(app, http.MethodGet, "/api/v2/admin/regcodes", "", user); rr.Code != http.StatusForbidden {
		t.Fatalf("expected normal user to be rejected, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestV2RegcodeUsageKeepsTelegramOnlyIdentity(t *testing.T) {
	app := newTestApp(t)
	admin := registerAdmin(t, app, "v2-reg-usage-admin", "Admin123456")

	code := "V2-USAGE-TEST"
	if err := app.store().UpsertRegCode(store.RegCode{Code: code, Type: 1, Days: 30, ValidityTime: -1, UseCountLimit: -1, Active: true, UsedByTelegramIDs: []int64{987654321}}); err != nil {
		t.Fatalf("upsert regcode: %v", err)
	}
	usage := doJSON(app, http.MethodGet, "/api/v2/admin/regcodes/"+code+"/usage", "", admin)
	if usage.Code != http.StatusOK {
		t.Fatalf("v2 usage status=%d body=%s", usage.Code, usage.Body.String())
	}
	var envelope struct {
		Data struct {
			Item struct {
				TelegramOnly []struct {
					TelegramID int64 `json:"telegram_id"`
				} `json:"telegram_only"`
			} `json:"item"`
		} `json:"data"`
	}
	if err := json.Unmarshal(usage.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode v2 usage: %v", err)
	}
	if len(envelope.Data.Item.TelegramOnly) != 1 || envelope.Data.Item.TelegramOnly[0].TelegramID != 987654321 {
		t.Fatalf("telegram-only usage identity lost: %+v", envelope.Data.Item.TelegramOnly)
	}
}
