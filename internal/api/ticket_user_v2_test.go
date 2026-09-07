package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/prejudice-studio/twilight/internal/store"
)

func TestV2UserTicketResourcesUseBoundedSummaryAndOwnedConversation(t *testing.T) {
	app := newTestApp(t)
	enableTicketSystem(t, app, nil)
	_ = registerAndLogin(t, app, "v2-ticket-admin", "Admin123456")
	user := registerAndLogin(t, app, "v2-ticket-owner", "Owner123456")
	id := createTicket(t, app, "V2 ticket", "private opening", user)

	if _, err := app.store().AddTicketReply(id, store.TicketReply{
		UID: 1, Username: "v2-ticket-owner", Role: store.RoleNormal, Content: "first reply",
	}); err != nil {
		t.Fatalf("add initial reply: %v", err)
	}

	list := doJSON(app, http.MethodGet, "/api/v2/tickets?page=1&per_page=20", "", user)
	if list.Code != http.StatusOK {
		t.Fatalf("v2 user list status=%d body=%s", list.Code, list.Body.String())
	}
	var listEnvelope struct {
		Data struct {
			Items      []map[string]any `json:"items"`
			Pagination struct {
				Total      int `json:"total"`
				TotalPages int `json:"total_pages"`
			} `json:"pagination"`
		} `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listEnvelope); err != nil {
		t.Fatalf("decode v2 user list: %v", err)
	}
	if listEnvelope.Data.Pagination.Total != 1 || listEnvelope.Data.Pagination.TotalPages != 1 || len(listEnvelope.Data.Items) != 1 {
		t.Fatalf("unexpected v2 user list: %#v", listEnvelope.Data)
	}
	if _, exists := listEnvelope.Data.Items[0]["content"]; exists {
		t.Fatalf("v2 user list exposed content: %#v", listEnvelope.Data.Items[0])
	}
	if _, exists := listEnvelope.Data.Items[0]["replies"]; exists {
		t.Fatalf("v2 user list exposed replies: %#v", listEnvelope.Data.Items[0])
	}

	path := "/api/v2/tickets/" + strconv.FormatInt(id, 10)
	detail := doJSON(app, http.MethodGet, path, "", user)
	if detail.Code != http.StatusOK {
		t.Fatalf("v2 user detail status=%d body=%s", detail.Code, detail.Body.String())
	}
	var detailEnvelope struct {
		Data struct {
			Item struct {
				Content     string              `json:"content"`
				Replies     []store.TicketReply `json:"replies"`
				Attachments []map[string]any    `json:"attachments"`
			} `json:"item"`
		} `json:"data"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &detailEnvelope); err != nil {
		t.Fatalf("decode v2 user detail: %v", err)
	}
	if detailEnvelope.Data.Item.Content != "private opening" || len(detailEnvelope.Data.Item.Replies) != 1 {
		t.Fatalf("unexpected v2 user detail: %#v", detailEnvelope.Data.Item)
	}

	reply := doJSON(app, http.MethodPost, path+"/replies", `{"content":"second reply"}`, user)
	if reply.Code != http.StatusOK {
		t.Fatalf("v2 user reply status=%d body=%s", reply.Code, reply.Body.String())
	}
	updated := doJSON(app, http.MethodGet, path, "", user)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), "first reply") || !strings.Contains(updated.Body.String(), "second reply") {
		t.Fatalf("v2 user reply did not preserve conversation: %s", updated.Body.String())
	}
}

func TestV2UserTicketDetailDoesNotAllowCrossUserRead(t *testing.T) {
	app := newTestApp(t)
	enableTicketSystem(t, app, nil)
	owner := registerAndLogin(t, app, "v2-owner", "Owner123456")
	other := registerAndLogin(t, app, "v2-other", "Other123456")
	id := createTicket(t, app, "private", "private body", owner)

	response := doJSON(app, http.MethodGet, "/api/v2/tickets/"+strconv.FormatInt(id, 10), "", other)
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-user detail status=%d body=%s", response.Code, response.Body.String())
	}
}
