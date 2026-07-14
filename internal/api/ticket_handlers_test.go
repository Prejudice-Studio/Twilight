package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/prejudice-studio/twilight/internal/store"
)

// enableTicketSystem 在运行时打开工单系统并按需覆盖并发上限 / 图片限制。
// 直接走 runtime.Store 替换 cfg，原因同 TestSessionCookieRespectsConfiguredDomain：
// New() 之后 cfg 副本存在 runtimeState 里，单纯改 a.cfg() 返回的指针字段会被
// 下次 reload 覆盖。mutate 允许调用方进一步定制具体上限。
func enableTicketSystem(t *testing.T, app *App, mutate func(cfg *struct {
	userLimit, globalLimit, imageMaxCount int
	imageMaxSize                          int64
})) {
	t.Helper()
	opts := struct {
		userLimit, globalLimit, imageMaxCount int
		imageMaxSize                          int64
	}{userLimit: 0, globalLimit: 0, imageMaxCount: 5, imageMaxSize: 5 * 1024 * 1024}
	if mutate != nil {
		mutate(&opts)
	}
	rt := app.runtime.Load()
	next := *rt
	next.cfg.TicketSystemEnabled = true
	next.cfg.TicketUserOpenLimit = opts.userLimit
	next.cfg.TicketGlobalOpenLimit = opts.globalLimit
	next.cfg.TicketImageMaxCount = opts.imageMaxCount
	next.cfg.TicketImageMaxSize = opts.imageMaxSize
	app.runtime.Store(&next)
}

// pngBytes 返回一段最小可被 http.DetectContentType 识别为 image/png 的字节。
func pngBytes() []byte {
	return []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0, 0, 0, 0, 0}
}

// uploadTicketImage 通过 multipart 上传一张图片，返回响应记录。
func uploadTicketImage(t *testing.T, app *App, ticketID int64, filename string, content []byte, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tickets/"+strconv.FormatInt(ticketID, 10)+"/images", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Twilight-Client", "webui")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, req)
	return rr
}

func createTicket(t *testing.T, app *App, title, content string, cookies []*http.Cookie) int64 {
	t.Helper()
	rr := doJSON(app, http.MethodPost, "/api/v1/tickets",
		`{"title":"`+title+`","content":"`+content+`"}`, cookies)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create ticket status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode create ticket: %v body=%s", err, rr.Body.String())
	}
	return resp.Data.ID
}

// TestTicketUserOpenLimitEnforced 验证「每人同时处理中/待处理工单上限」服务端硬门 (子需求 A)。
func TestTicketUserOpenLimitEnforced(t *testing.T) {
	app := newTestApp(t)
	enableTicketSystem(t, app, func(c *struct {
		userLimit, globalLimit, imageMaxCount int
		imageMaxSize                          int64
	}) {
		c.userLimit = 2
	})
	cookies := registerAndLogin(t, app, "user", "User12345678")

	createTicket(t, app, "first", "content one", cookies)
	createTicket(t, app, "second", "content two", cookies)

	// 第三单应被用户上限拦下。
	rr := doJSON(app, http.MethodPost, "/api/v1/tickets", `{"title":"third","content":"content three"}`, cookies)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 for user limit, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(ErrTicketUserLimit)) {
		t.Fatalf("expected error code %s, body=%s", ErrTicketUserLimit, rr.Body.String())
	}
}

// TestTicketGlobalOpenLimitEnforced 验证「管理员全局工单数量上限」服务端硬门 (子需求 B)。
func TestTicketGlobalOpenLimitEnforced(t *testing.T) {
	app := newTestApp(t)
	enableTicketSystem(t, app, func(c *struct {
		userLimit, globalLimit, imageMaxCount int
		imageMaxSize                          int64
	}) {
		c.globalLimit = 1
	})
	first := registerAndLogin(t, app, "alice", "Alice1234567")
	second := registerAndLogin(t, app, "bob", "Bob123456789")

	createTicket(t, app, "alice-ticket", "content from alice", first)

	// 全局已满，第二个用户也应被拦下。
	rr := doJSON(app, http.MethodPost, "/api/v1/tickets", `{"title":"bob-ticket","content":"content from bob"}`, second)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 for global limit, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(ErrTicketGlobalLimit)) {
		t.Fatalf("expected error code %s, body=%s", ErrTicketGlobalLimit, rr.Body.String())
	}
}

// TestTicketImageUploadAndValidation 验证图片上传成功 + 类型校验 + 数量上限 (子需求 C/D)。
func TestTicketImageUploadAndValidation(t *testing.T) {
	app := newTestApp(t)
	enableTicketSystem(t, app, func(c *struct {
		userLimit, globalLimit, imageMaxCount int
		imageMaxSize                          int64
	}) {
		c.imageMaxCount = 2
	})
	cookies := registerAndLogin(t, app, "user", "User12345678")
	id := createTicket(t, app, "with-image", "needs an image", cookies)

	// 合法 PNG 应上传成功。
	rr := uploadTicketImage(t, app, id, "a.png", pngBytes(), cookies)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 for valid image, got %d body=%s", rr.Code, rr.Body.String())
	}

	// 非图片内容应被内容嗅探拦下。
	rr = uploadTicketImage(t, app, id, "b.png", []byte("this is plain text not an image"), cookies)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-image, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(ErrTicketImageInvalid)) {
		t.Fatalf("expected %s for non-image, body=%s", ErrTicketImageInvalid, rr.Body.String())
	}

	// 第二张合法图片占满上限。
	rr = uploadTicketImage(t, app, id, "c.png", pngBytes(), cookies)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 for second image, got %d body=%s", rr.Code, rr.Body.String())
	}

	// 第三张应触发数量上限。
	rr = uploadTicketImage(t, app, id, "d.png", pngBytes(), cookies)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 for too many images, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(ErrTicketImageTooMany)) {
		t.Fatalf("expected %s for too many images, body=%s", ErrTicketImageTooMany, rr.Body.String())
	}

	// 图片目录应按工单 ID 存放。
	dir, derr := app.ticketAttachmentDir(id)
	if derr != nil {
		t.Fatalf("ticketAttachmentDir: %v", derr)
	}
	if !bytes.Contains([]byte(dir), []byte(filepath.Join("tickets", strconv.FormatInt(id, 10)))) {
		t.Fatalf("attachment dir should be scoped by ticket id, got %q", dir)
	}
	entries, eerr := os.ReadDir(dir)
	if eerr != nil {
		t.Fatalf("read attachment dir: %v", eerr)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 files on disk, got %d", len(entries))
	}
}

// TestTicketImageTooLarge 验证单张图片大小上限 (子需求 C)。
func TestTicketImageTooLarge(t *testing.T) {
	app := newTestApp(t)
	enableTicketSystem(t, app, func(c *struct {
		userLimit, globalLimit, imageMaxCount int
		imageMaxSize                          int64
	}) {
		c.imageMaxSize = 16 // 极小上限便于触发
	})
	cookies := registerAndLogin(t, app, "user", "User12345678")
	id := createTicket(t, app, "big-image", "too big", cookies)

	big := make([]byte, 64)
	copy(big, pngBytes())
	rr := uploadTicketImage(t, app, id, "big.png", big, cookies)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for oversize image, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(ErrTicketImageTooLarge)) {
		t.Fatalf("expected %s for oversize image, body=%s", ErrTicketImageTooLarge, rr.Body.String())
	}
}

// TestDeleteTicketRemovesAttachments 验证删除工单时同步清除其图片目录 (子需求 F)。
func TestDeleteTicketRemovesAttachments(t *testing.T) {
	app := newTestApp(t)
	enableTicketSystem(t, app, nil)
	admin := registerAndLogin(t, app, "admin", "Admin123456")
	id := createTicket(t, app, "deleteme", "with attachments", admin)

	rr := uploadTicketImage(t, app, id, "a.png", pngBytes(), admin)
	if rr.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body=%s", rr.Code, rr.Body.String())
	}
	dir, derr := app.ticketAttachmentDir(id)
	if derr != nil {
		t.Fatalf("ticketAttachmentDir: %v", derr)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("expected attachment dir to exist before delete: %v", err)
	}

	del := doJSON(app, http.MethodDelete, "/api/v1/admin/tickets/"+strconv.FormatInt(id, 10), ``, admin)
	if del.Code != http.StatusOK {
		t.Fatalf("delete ticket status = %d body=%s", del.Code, del.Body.String())
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected attachment dir removed after delete, stat err = %v", err)
	}
	if _, found := app.store().Ticket(id); found {
		t.Fatalf("ticket should be gone after delete")
	}
}

// TestClosedTicketRejectsImageUpload 验证已关闭工单不再接受图片上传 (子需求 C/G)。
func TestClosedTicketRejectsImageUpload(t *testing.T) {
	app := newTestApp(t)
	enableTicketSystem(t, app, nil)
	cookies := registerAndLogin(t, app, "user", "User12345678")
	admin := registerAndLogin(t, app, "admin", "Admin123456")
	id := createTicket(t, app, "to-close", "will be closed", cookies)

	closeRR := doJSON(app, http.MethodPost, "/api/v1/tickets/"+strconv.FormatInt(id, 10)+"/close", ``, cookies)
	if closeRR.Code != http.StatusOK {
		t.Fatalf("close ticket status = %d body=%s", closeRR.Code, closeRR.Body.String())
	}

	rr := uploadTicketImage(t, app, id, "a.png", pngBytes(), cookies)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 uploading to closed ticket, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(ErrTicketAlreadyClosed)) {
		t.Fatalf("expected %s for closed ticket, body=%s", ErrTicketAlreadyClosed, rr.Body.String())
	}
	adminUpload := uploadTicketImage(t, app, id, "admin.png", pngBytes(), admin)
	if adminUpload.Code != http.StatusCreated {
		t.Fatalf("expected admin upload to closed ticket to succeed, got %d body=%s", adminUpload.Code, adminUpload.Body.String())
	}
}

// uploadedTicketImageFilename 上传一张图片并从响应里取回服务端落盘后的文件名。
func uploadedTicketImageFilename(t *testing.T, app *App, ticketID int64, name string, cookies []*http.Cookie) string {
	t.Helper()
	rr := uploadTicketImage(t, app, ticketID, name, pngBytes(), cookies)
	if rr.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			Attachments []struct {
				Filename string `json:"filename"`
			} `json:"attachments"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode upload resp: %v body=%s", err, rr.Body.String())
	}
	if len(resp.Data.Attachments) == 0 {
		t.Fatalf("expected at least one attachment, body=%s", rr.Body.String())
	}
	return resp.Data.Attachments[len(resp.Data.Attachments)-1].Filename
}

// TestClosedTicketBlocksUserImageDelete 验证工单关闭后普通用户不能删除自己的图片，
// 但管理员仍可删除 (TASK#2)。
func TestClosedTicketBlocksUserImageDelete(t *testing.T) {
	app := newTestApp(t)
	enableTicketSystem(t, app, nil)
	user := registerAndLogin(t, app, "user", "User12345678")
	admin := registerAndLogin(t, app, "admin", "Admin123456")

	id := createTicket(t, app, "freeze-img", "freeze on close", user)
	keepFile := uploadedTicketImageFilename(t, app, id, "keep.png", user)
	adminFile := uploadedTicketImageFilename(t, app, id, "admincanremove.png", user)

	// 关闭前用户可删除（这里只验证关闭后行为，关闭工单）。
	if rr := doJSON(app, http.MethodPost, "/api/v1/tickets/"+strconv.FormatInt(id, 10)+"/close", ``, user); rr.Code != http.StatusOK {
		t.Fatalf("close status = %d body=%s", rr.Code, rr.Body.String())
	}

	// 关闭后普通用户删除应被 403 拦下。
	delPath := "/api/v1/tickets/" + strconv.FormatInt(id, 10) + "/images/" + keepFile
	rr := doJSON(app, http.MethodDelete, delPath, ``, user)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 deleting image on closed ticket as user, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(ErrTicketAlreadyClosed)) {
		t.Fatalf("expected %s, body=%s", ErrTicketAlreadyClosed, rr.Body.String())
	}
	if ticket, _ := app.store().Ticket(id); !ticketHasAttachment(ticket, keepFile) {
		t.Fatalf("user image should remain after blocked delete")
	}

	// 管理员仍可删除已关闭工单的图片。
	adminDel := doJSON(app, http.MethodDelete, "/api/v1/tickets/"+strconv.FormatInt(id, 10)+"/images/"+adminFile, ``, admin)
	if adminDel.Code != http.StatusOK {
		t.Fatalf("expected admin delete to succeed, got %d body=%s", adminDel.Code, adminDel.Body.String())
	}
	if ticket, _ := app.store().Ticket(id); ticketHasAttachment(ticket, adminFile) {
		t.Fatalf("admin delete should remove attachment on closed ticket")
	}
}

// TestClosedTicketsWithAttachmentsBefore 验证清理任务的查询基础 (子需求 E)。
func TestClosedTicketsWithAttachmentsBefore(t *testing.T) {
	app := newTestApp(t)
	enableTicketSystem(t, app, nil)
	cookies := registerAndLogin(t, app, "user", "User12345678")
	id := createTicket(t, app, "retention", "retention test", cookies)

	if rr := uploadTicketImage(t, app, id, "a.png", pngBytes(), cookies); rr.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body=%s", rr.Code, rr.Body.String())
	}

	// 关闭工单后，ClosedAt 被写入。
	if rr := doJSON(app, http.MethodPost, "/api/v1/tickets/"+strconv.FormatInt(id, 10)+"/close", ``, cookies); rr.Code != http.StatusOK {
		t.Fatalf("close status = %d body=%s", rr.Code, rr.Body.String())
	}

	ticket, found := app.store().Ticket(id)
	if !found || ticket.ClosedAt <= 0 {
		t.Fatalf("expected closed ticket with ClosedAt set, found=%v ClosedAt=%d", found, ticket.ClosedAt)
	}

	// cutoff 取关闭时间之后一秒，应命中该工单。
	hit := app.store().ClosedTicketsWithAttachmentsBefore(ticket.ClosedAt + 1)
	if len(hit) != 1 || hit[0].ID != id {
		t.Fatalf("expected ticket %d in cleanup candidates, got %+v", id, hit)
	}

	// cutoff 取关闭时间之前，不应命中。
	none := app.store().ClosedTicketsWithAttachmentsBefore(ticket.ClosedAt - 1)
	for _, tk := range none {
		if tk.ID == id {
			t.Fatalf("ticket %d should not be a cleanup candidate before its ClosedAt", id)
		}
	}

	// 清理附件后，候选集合应排除该工单。
	if err := app.store().ClearTicketAttachments(id); err != nil {
		t.Fatalf("ClearTicketAttachments: %v", err)
	}
	app.removeTicketAttachmentDir(id)
	after := app.store().ClosedTicketsWithAttachmentsBefore(ticket.ClosedAt + 1)
	for _, tk := range after {
		if tk.ID == id {
			t.Fatalf("ticket %d should be excluded once attachments cleared", id)
		}
	}
}

func TestTicketRepliesSurviveStatusUpdatesAndReopenResolved(t *testing.T) {
	app := newTestApp(t)
	enableTicketSystem(t, app, nil)
	admin := registerAndLogin(t, app, "admin", "Admin123456")
	user := registerAndLogin(t, app, "user", "User12345678")
	id := createTicket(t, app, "conversation", "initial content", user)

	replyPath := "/api/v1/tickets/" + strconv.FormatInt(id, 10) + "/reply"
	if rr := doJSON(app, http.MethodPost, replyPath, `{"content":"user adds detail"}`, user); rr.Code != http.StatusOK {
		t.Fatalf("user reply status=%d body=%s", rr.Code, rr.Body.String())
	}

	adminUpdate := doJSON(app, http.MethodPut, "/api/v1/admin/tickets/"+strconv.FormatInt(id, 10), `{"status":"resolved","priority":"high","type":"all","admin_note":"admin resolution"}`, admin)
	if adminUpdate.Code != http.StatusOK {
		t.Fatalf("admin update status=%d body=%s", adminUpdate.Code, adminUpdate.Body.String())
	}
	ticket, found := app.store().Ticket(id)
	if !found {
		t.Fatalf("ticket not found after update")
	}
	if ticket.Status != store.TicketStatusResolved || ticket.ResolvedAt <= 0 {
		t.Fatalf("expected resolved ticket with timestamp, got status=%q resolved_at=%d", ticket.Status, ticket.ResolvedAt)
	}
	if len(ticket.Replies) != 2 {
		t.Fatalf("expected user reply plus admin reply, got %#v", ticket.Replies)
	}
	if ticket.Replies[0].Content != "user adds detail" || ticket.Replies[1].Content != "admin resolution" {
		t.Fatalf("unexpected replies: %#v", ticket.Replies)
	}

	if rr := doJSON(app, http.MethodPost, replyPath, `{"content":"still broken"}`, user); rr.Code != http.StatusOK {
		t.Fatalf("user reply to resolved ticket status=%d body=%s", rr.Code, rr.Body.String())
	}
	ticket, found = app.store().Ticket(id)
	if !found {
		t.Fatalf("ticket not found after reopen reply")
	}
	if ticket.Status != store.TicketStatusOpen || ticket.ResolvedAt != 0 {
		t.Fatalf("user reply should reopen resolved ticket, got status=%q resolved_at=%d", ticket.Status, ticket.ResolvedAt)
	}
	if len(ticket.Replies) != 3 {
		t.Fatalf("expected third reply retained, got %#v", ticket.Replies)
	}
}

func TestAdminTicketsAllFilterIncludesResolvedAndClosed(t *testing.T) {
	app := newTestApp(t)
	enableTicketSystem(t, app, nil)
	admin := registerAndLogin(t, app, "admin", "Admin123456")
	user := registerAndLogin(t, app, "user", "User12345678")

	openID := createTicket(t, app, "open ticket", "still open", user)
	resolvedID := createTicket(t, app, "resolved ticket", "done", user)
	closedID := createTicket(t, app, "closed ticket", "closed", user)

	if rr := doJSON(app, http.MethodPut, "/api/v1/admin/tickets/"+strconv.FormatInt(resolvedID, 10), `{"status":"resolved"}`, admin); rr.Code != http.StatusOK {
		t.Fatalf("resolve ticket status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := doJSON(app, http.MethodPost, "/api/v1/tickets/"+strconv.FormatInt(closedID, 10)+"/close", ``, user); rr.Code != http.StatusOK {
		t.Fatalf("close ticket status=%d body=%s", rr.Code, rr.Body.String())
	}

	decodeIDs := func(rr *httptest.ResponseRecorder) map[int64]bool {
		t.Helper()
		if rr.Code != http.StatusOK {
			t.Fatalf("admin list status=%d body=%s", rr.Code, rr.Body.String())
		}
		var resp struct {
			Data struct {
				Tickets []struct {
					ID int64 `json:"id"`
				} `json:"tickets"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode admin tickets: %v body=%s", err, rr.Body.String())
		}
		ids := map[int64]bool{}
		for _, ticket := range resp.Data.Tickets {
			ids[ticket.ID] = true
		}
		return ids
	}

	defaultIDs := decodeIDs(doJSON(app, http.MethodGet, "/api/v1/admin/tickets", ``, admin))
	if !defaultIDs[openID] || defaultIDs[resolvedID] || defaultIDs[closedID] {
		t.Fatalf("default admin list should include only active tickets, got %#v", defaultIDs)
	}

	allIDs := decodeIDs(doJSON(app, http.MethodGet, "/api/v1/admin/tickets?all=1", ``, admin))
	if !allIDs[openID] || !allIDs[resolvedID] || !allIDs[closedID] {
		t.Fatalf("all=1 admin list should include every status, got %#v", allIDs)
	}

	statusAllIDs := decodeIDs(doJSON(app, http.MethodGet, "/api/v1/admin/tickets?status=all", ``, admin))
	if !statusAllIDs[openID] || !statusAllIDs[resolvedID] || !statusAllIDs[closedID] {
		t.Fatalf("status=all admin list should include every status, got %#v", statusAllIDs)
	}
}

func TestTicketAdminNotificationTargetsSkipActor(t *testing.T) {
	app := newTestApp(t)
	actor, err := app.store().CreateUser(store.User{Username: "actor-admin", Role: store.RoleAdmin, Active: true, NotifyOnTicketTelegram: true, TelegramID: 1001})
	if err != nil {
		t.Fatalf("create actor admin: %v", err)
	}
	other, err := app.store().CreateUser(store.User{Username: "other-admin", Role: store.RoleAdmin, Active: true, NotifyOnTicketTelegram: true, TelegramID: 1002})
	if err != nil {
		t.Fatalf("create other admin: %v", err)
	}
	if _, err := app.store().CreateUser(store.User{Username: "disabled-notify-admin", Role: store.RoleAdmin, Active: true, NotifyOnTicketTelegram: false, TelegramID: 1003}); err != nil {
		t.Fatalf("create disabled notify admin: %v", err)
	}
	if _, err := app.store().CreateUser(store.User{Username: "normal-user", Role: store.RoleNormal, Active: true, NotifyOnTicketTelegram: true, TelegramID: 1004}); err != nil {
		t.Fatalf("create normal user: %v", err)
	}

	targets := app.ticketAdminNotificationTargets(actor)
	if len(targets) != 1 || targets[0].UID != other.UID {
		t.Fatalf("expected only the other subscribed admin, actor=%d other=%d targets=%#v", actor.UID, other.UID, targets)
	}

	userActor := store.User{UID: 9999, Username: "ticket-owner", Role: store.RoleNormal}
	targets = app.ticketAdminNotificationTargets(userActor)
	if len(targets) != 2 {
		t.Fatalf("user-origin events should notify subscribed admins, got %#v", targets)
	}
}

func TestClosedTicketReplyPolicyAllowsAdminOnly(t *testing.T) {
	app := newTestApp(t)
	enableTicketSystem(t, app, nil)
	admin := registerAndLogin(t, app, "admin", "Admin123456")
	user := registerAndLogin(t, app, "user", "User12345678")
	id := createTicket(t, app, "closed-reply", "initial content", user)
	if rr := doJSON(app, http.MethodPost, "/api/v1/tickets/"+strconv.FormatInt(id, 10)+"/close", ``, user); rr.Code != http.StatusOK {
		t.Fatalf("close ticket status=%d body=%s", rr.Code, rr.Body.String())
	}
	replyPath := "/api/v1/tickets/" + strconv.FormatInt(id, 10) + "/reply"
	userReply := doJSON(app, http.MethodPost, replyPath, `{"content":"user after close"}`, user)
	if userReply.Code != http.StatusBadRequest {
		t.Fatalf("expected user reply to closed ticket to fail, got %d body=%s", userReply.Code, userReply.Body.String())
	}
	if !bytes.Contains(userReply.Body.Bytes(), []byte(ErrTicketAlreadyClosed)) {
		t.Fatalf("expected %s, body=%s", ErrTicketAlreadyClosed, userReply.Body.String())
	}
	adminReply := doJSON(app, http.MethodPost, replyPath, `{"content":"admin diagnostic note"}`, admin)
	if adminReply.Code != http.StatusOK {
		t.Fatalf("expected admin reply to closed ticket to succeed, got %d body=%s", adminReply.Code, adminReply.Body.String())
	}
	ticket, found := app.store().Ticket(id)
	if !found {
		t.Fatal("ticket not found after admin reply")
	}
	if ticket.Status != store.TicketStatusClosed {
		t.Fatalf("admin reply should keep closed status, got %q", ticket.Status)
	}
	if len(ticket.Replies) != 1 || ticket.Replies[0].Content != "admin diagnostic note" {
		t.Fatalf("unexpected replies after admin closed-ticket reply: %#v", ticket.Replies)
	}
}

func TestTicketTypeNormalizeAndRenameUpdatesExistingTickets(t *testing.T) {
	app := newTestApp(t)
	enableTicketSystem(t, app, nil)
	if err := app.store().AddTicketType("BugReport"); err != nil {
		t.Fatalf("AddTicketType: %v", err)
	}
	user := registerAndLogin(t, app, "user", "User12345678")

	rr := doJSON(app, http.MethodPost, "/api/v1/tickets", `{"title":"typed","content":"content","type":"bugreport"}`, user)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create typed ticket status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode typed ticket: %v body=%s", err, rr.Body.String())
	}
	if resp.Data.Type != "BugReport" {
		t.Fatalf("expected configured type casing, got %q", resp.Data.Type)
	}

	count, err := app.store().RenameTicketType("bugreport", "Incident")
	if err != nil {
		t.Fatalf("RenameTicketType: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one existing ticket updated, got %d", count)
	}
	ticket, found := app.store().Ticket(resp.Data.ID)
	if !found {
		t.Fatalf("ticket not found after type rename")
	}
	if ticket.Type != "Incident" {
		t.Fatalf("expected existing ticket type renamed, got %q", ticket.Type)
	}
}
