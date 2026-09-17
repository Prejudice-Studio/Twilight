package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/prejudice-studio/twilight/internal/store"
)

func TestV2AdminInviteResourcesUseBoundedShapes(t *testing.T) {
	app := newTestApp(t)
	app.cfg().InviteEnabled = true
	admin := registerAdmin(t, app, "v2-invite-admin", "Admin123456")
	owner, err := app.store().CreateUser(store.User{Username: "v2-invite-owner", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertInviteCode(store.InviteCode{
		Code: "V2-INVITE-CODE", UID: owner.UID, InviterUID: owner.UID,
		Days: 30, UseCountLimit: 1, Active: true, Note: "v2 invite",
	}); err != nil {
		t.Fatal(err)
	}

	tree := doJSON(app, http.MethodGet, "/api/v2/admin/invite/tree", "", admin)
	if tree.Code != http.StatusOK {
		t.Fatalf("v2 invite tree status=%d body=%s", tree.Code, tree.Body.String())
	}
	var treeEnvelope struct {
		Data struct {
			Item struct {
				Rows       []map[string]any `json:"rows"`
				TotalNodes int              `json:"total_nodes"`
				Pages      int              `json:"pages"`
			} `json:"item"`
		} `json:"data"`
	}
	if err := json.Unmarshal(tree.Body.Bytes(), &treeEnvelope); err != nil {
		t.Fatalf("decode v2 invite tree: %v", err)
	}
	if treeEnvelope.Data.Item.TotalNodes != 1 || len(treeEnvelope.Data.Item.Rows) != 1 || treeEnvelope.Data.Item.Pages != 1 {
		t.Fatalf("v2 invite tree did not return a bounded resource item: %+v", treeEnvelope.Data.Item)
	}

	codes := doJSON(app, http.MethodGet, "/api/v2/admin/invite/codes?page=1&per_page=1&search=v2-invite", "", admin)
	if codes.Code != http.StatusOK {
		t.Fatalf("v2 invite codes status=%d body=%s", codes.Code, codes.Body.String())
	}
	var codesEnvelope struct {
		Data struct {
			Items      []map[string]any `json:"items"`
			Pagination struct {
				Page       int `json:"page"`
				PerPage    int `json:"per_page"`
				Total      int `json:"total"`
				TotalPages int `json:"total_pages"`
			} `json:"pagination"`
		} `json:"data"`
	}
	if err := json.Unmarshal(codes.Body.Bytes(), &codesEnvelope); err != nil {
		t.Fatalf("decode v2 invite codes: %v", err)
	}
	if codesEnvelope.Data.Pagination.Page != 1 || codesEnvelope.Data.Pagination.PerPage != 1 || codesEnvelope.Data.Pagination.Total != 1 || len(codesEnvelope.Data.Items) != 1 {
		t.Fatalf("unexpected v2 invite pagination: %+v", codesEnvelope.Data)
	}
	if codesEnvelope.Data.Items[0]["code"] != "V2-INVITE-CODE" {
		t.Fatalf("unexpected v2 invite item: %#v", codesEnvelope.Data.Items[0])
	}
}

func TestV2AdminInviteConfigExposesOnlyInviteFields(t *testing.T) {
	app := newTestApp(t)
	admin := registerAdmin(t, app, "v2-invite-config-admin", "Admin123456")
	response := doJSON(app, http.MethodGet, "/api/v2/admin/invite/config/schema", "", admin)
	if response.Code != http.StatusOK {
		t.Fatalf("v2 invite config status=%d body=%s", response.Code, response.Body.String())
	}
	if cacheControl := response.Header().Get("Cache-Control"); cacheControl != "private, no-store" {
		t.Fatalf("unexpected v2 invite config cache policy %q", cacheControl)
	}
	var envelope struct {
		Data struct {
			Sections []struct {
				Key    string `json:"key"`
				Fields []struct {
					Key string `json:"key"`
				} `json:"fields"`
			} `json:"sections"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode v2 invite config: %v", err)
	}
	if len(envelope.Data.Sections) != 1 || envelope.Data.Sections[0].Key != "SAR" {
		t.Fatalf("unexpected v2 invite config sections: %+v", envelope.Data.Sections)
	}
	for _, field := range envelope.Data.Sections[0].Fields {
		if !inviteAdminConfigKeys[field.Key] {
			t.Fatalf("v2 invite config leaked non-invite field %q", field.Key)
		}
	}
}

func TestV2AdminInviteTreeAppliesCollapsedSearchAndSelectedProjection(t *testing.T) {
	app := newTestApp(t)
	admin := registerAdmin(t, app, "v2-invite-tree-admin", "Admin123456")
	parent, err := app.store().CreateUser(store.User{Username: "v2-tree-parent", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	child, err := app.store().CreateUser(store.User{Username: "v2-tree-child", Role: store.RoleNormal, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store().UpsertInviteCode(store.InviteCode{Code: "V2-TREE-EDGE", UID: parent.UID, InviterUID: parent.UID, Days: 30, UseCountLimit: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.store().ConsumeInviteCode("V2-TREE-EDGE", child.UID); err != nil {
		t.Fatal(err)
	}

	decodeRows := func(path string) struct {
		Rows []struct {
			UID int64 `json:"uid"`
		} `json:"rows"`
		Selected struct {
			UID int64 `json:"uid"`
		} `json:"selected"`
	} {
		response := doJSON(app, http.MethodGet, path, "", admin)
		if response.Code != http.StatusOK {
			t.Fatalf("v2 invite tree status=%d body=%s", response.Code, response.Body.String())
		}
		var envelope struct {
			Data struct {
				Item struct {
					Rows []struct {
						UID int64 `json:"uid"`
					} `json:"rows"`
					Selected struct {
						UID int64 `json:"uid"`
					} `json:"selected"`
				} `json:"item"`
			} `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode v2 invite tree: %v", err)
		}
		return envelope.Data.Item
	}

	collapsed := decodeRows("/api/v2/admin/invite/tree?collapsed=" + strconv.FormatInt(parent.UID, 10) + "&selected=" + strconv.FormatInt(child.UID, 10))
	if len(collapsed.Rows) != 1 || collapsed.Rows[0].UID != parent.UID || collapsed.Selected.UID != child.UID {
		t.Fatalf("collapsed tree did not preserve root and selected child: %+v", collapsed)
	}
	searched := decodeRows("/api/v2/admin/invite/tree?search=v2-tree-child&collapsed=" + strconv.FormatInt(parent.UID, 10))
	if len(searched.Rows) != 2 || searched.Rows[0].UID != parent.UID || searched.Rows[1].UID != child.UID {
		t.Fatalf("search should reopen the matched path: %+v", searched)
	}
}

func TestV2AdminInviteResourcesRejectNormalUsers(t *testing.T) {
	app := newTestApp(t)
	user := registerAndLogin(t, app, "v2-invite-user", "User12345678")
	response := doJSON(app, http.MethodGet, "/api/v2/admin/invite/tree", "", user)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected normal user rejection, got %d body=%s", response.Code, response.Body.String())
	}
}
