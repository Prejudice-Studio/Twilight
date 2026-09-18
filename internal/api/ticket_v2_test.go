package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/prejudice-studio/twilight/internal/store"
)

func TestV2AdminTicketResourcesUseResourceShapeAndPreserveConversation(t *testing.T) {
	app := newTestApp(t)
	enableTicketSystem(t, app, nil)
	admin := registerAdmin(t, app, "v2-admin", "Admin123456")

	user := registerAndLogin(t, app, "v2-ticket-user", "User12345678")
	id := createTicket(t, app, "v2 resource", "initial message", user)

	if rr := doJSON(app, http.MethodPost, "/api/v1/tickets/"+strconv.FormatInt(id, 10)+"/reply", `{"content":"user detail"}`, user); rr.Code != http.StatusOK {
		t.Fatalf("user reply status=%d body=%s", rr.Code, rr.Body.String())
	}

	list := doJSON(app, http.MethodGet, "/api/v2/admin/tickets?page=1&per_page=20", "", admin)
	if list.Code != http.StatusOK {
		t.Fatalf("v2 list status=%d body=%s", list.Code, list.Body.String())
	}
	var listEnvelope struct {
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
	if err := json.Unmarshal(list.Body.Bytes(), &listEnvelope); err != nil {
		t.Fatalf("decode v2 list: %v body=%s", err, list.Body.String())
	}
	if listEnvelope.Data.Pagination.Page != 1 || listEnvelope.Data.Pagination.PerPage != 20 || listEnvelope.Data.Pagination.Total < 1 {
		t.Fatalf("unexpected v2 pagination: %+v", listEnvelope.Data.Pagination)
	}
	for _, item := range listEnvelope.Data.Items {
		if int64(item["id"].(float64)) != id {
			continue
		}
		if _, exists := item["replies"]; exists {
			t.Fatalf("v2 queue must omit reply bodies: %#v", item)
		}
		if item["reply_count"] != float64(1) {
			t.Fatalf("v2 queue reply count=%v", item["reply_count"])
		}
	}

	path := "/api/v2/admin/tickets/" + strconv.FormatInt(id, 10)
	update := doJSON(app, http.MethodPatch, path, `{"status":"resolved","admin_note":"internal note"}`, admin)
	if update.Code != http.StatusOK {
		t.Fatalf("v2 patch status=%d body=%s", update.Code, update.Body.String())
	}
	reply := doJSON(app, http.MethodPost, path+"/replies", `{"content":"admin answer"}`, admin)
	if reply.Code != http.StatusOK {
		t.Fatalf("v2 reply status=%d body=%s", reply.Code, reply.Body.String())
	}

	detail := doJSON(app, http.MethodGet, path, "", admin)
	if detail.Code != http.StatusOK {
		t.Fatalf("v2 detail status=%d body=%s", detail.Code, detail.Body.String())
	}
	var detailEnvelope struct {
		Data struct {
			Item struct {
				AdminNote string `json:"admin_note"`
				Replies   []struct {
					Content string `json:"content"`
				} `json:"replies"`
			} `json:"item"`
		} `json:"data"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &detailEnvelope); err != nil {
		t.Fatalf("decode v2 detail: %v body=%s", err, detail.Body.String())
	}
	if detailEnvelope.Data.Item.AdminNote != "internal note" || len(detailEnvelope.Data.Item.Replies) != 2 || detailEnvelope.Data.Item.Replies[0].Content != "user detail" || detailEnvelope.Data.Item.Replies[1].Content != "admin answer" {
		t.Fatalf("v2 detail lost conversation or note: %+v", detailEnvelope.Data.Item)
	}
}

func TestV2AdminTicketResourcesRejectNormalUsers(t *testing.T) {
	app := newTestApp(t)
	enableTicketSystem(t, app, nil)
	user := registerAndLogin(t, app, "v2-normal", "User12345678")
	if rr := doJSON(app, http.MethodGet, "/api/v2/admin/tickets", "", user); rr.Code != http.StatusForbidden {
		t.Fatalf("expected v2 admin list to reject normal user, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestV2UserTicketReplyUsesOwnershipAndAtomicAppend(t *testing.T) {
	app := newTestApp(t)
	enableTicketSystem(t, app, nil)
	owner := registerAndLogin(t, app, "v2-reply-owner", "ReplyOwner123456")
	other := registerAndLogin(t, app, "v2-reply-other", "ReplyOther123456")
	id := createTicket(t, app, "reply boundary", "initial", owner)
	path := "/api/v2/tickets/" + strconv.FormatInt(id, 10) + "/replies"

	reply := doJSON(app, http.MethodPost, path, "{\"content\":\"owner reply\"}", owner)
	if reply.Code != http.StatusOK || reply.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("owner v2 reply status=%d cache=%q body=%s", reply.Code, reply.Header().Get("Cache-Control"), reply.Body.String())
	}
	forbidden := doJSON(app, http.MethodPost, path, "{\"content\":\"not owner\"}", other)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("other user v2 reply status=%d body=%s", forbidden.Code, forbidden.Body.String())
	}
	ticket, found := app.store().Ticket(id)
	if !found || len(ticket.Replies) != 1 || ticket.Replies[0].Content != "owner reply" {
		t.Fatalf("v2 reply changed ticket unexpectedly: found=%v ticket=%+v", found, ticket)
	}
}

func TestV2AdminTicketTypeResourceUsesPathIdentity(t *testing.T) {
	app := newTestApp(t)
	admin := registerAdmin(t, app, "v2-type-admin", "Admin123456")

	if err := app.store().AddTicketType("BugReport"); err != nil {
		t.Fatalf("add type: %v", err)
	}
	rename := doJSON(app, http.MethodPatch, "/api/v2/admin/ticket-types/BugReport", `{"name":"Incident"}`, admin)
	if rename.Code != http.StatusOK {
		t.Fatalf("v2 rename type status=%d body=%s", rename.Code, rename.Body.String())
	}
	if got := store.NormalizeTicketType(app.store().TicketTypes(), "incident"); got != "Incident" {
		t.Fatalf("expected renamed type, got %q", got)
	}
}
