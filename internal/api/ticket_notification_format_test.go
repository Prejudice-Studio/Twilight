package api

import (
	"strings"
	"testing"

	"github.com/prejudice-studio/twilight/internal/store"
)

// TestTelegramEscapeHTML 锁定 Telegram HTML 子集只转义 & < >，不动引号。
func TestTelegramEscapeHTML(t *testing.T) {
	cases := map[string]string{
		"a<b>c":    "a&lt;b&gt;c",
		"x & y":    "x &amp; y",
		`"quoted"`: `"quoted"`,
		"<script>": "&lt;script&gt;",
		"plain 文本": "plain 文本",
	}
	for in, want := range cases {
		if got := telegramEscapeHTML(in); got != want {
			t.Fatalf("telegramEscapeHTML(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestStripTelegramHTML 验证富文本降级：去标签 + 反转义三实体，且不吞掉正文。
func TestStripTelegramHTML(t *testing.T) {
	in := "🎫 <b>工单更新</b>\n<blockquote>价格 &lt; 100 &amp; 折扣</blockquote>"
	want := "🎫 工单更新\n价格 < 100 & 折扣"
	if got := stripTelegramHTML(in); got != want {
		t.Fatalf("stripTelegramHTML = %q, want %q", got, want)
	}
}

// TestTicketAdminNotificationTextEscapesUserContent 是安全回归：用户可控的标题
// 与用户名即使含 < > &，也必须被转义，不能破坏我们拼的 HTML 结构 / 注入标签。
func TestTicketAdminNotificationTextEscapesUserContent(t *testing.T) {
	app := newTestApp(t)
	ticket := store.Ticket{
		ID:       42,
		Title:    "<b>xss</b> & bug",
		Status:   store.TicketStatusOpen,
		Priority: store.TicketPriorityUrgent,
		Type:     "bug",
		Username: "eve<script>",
		UID:      7,
		Content:  "正文 <img> 内容",
	}
	out := app.ticketAdminNotificationText("created", ticket, store.User{})

	if strings.Contains(out, "<b>xss</b>") {
		t.Fatalf("raw user title tag leaked into notification: %q", out)
	}
	if !strings.Contains(out, "&lt;b&gt;xss&lt;/b&gt;") {
		t.Fatalf("expected escaped title, got %q", out)
	}
	if !strings.Contains(out, "eve&lt;script&gt;") {
		t.Fatalf("expected escaped username, got %q", out)
	}
	// 我们自己的结构标签必须保留（未被转义）。
	if !strings.Contains(out, "<b>#42</b>") {
		t.Fatalf("expected ticket id in bold structure, got %q", out)
	}
	// 优先级应翻译为中文带图标，不再暴露英文枚举。
	if !strings.Contains(out, "紧急") || strings.Contains(out, "urgent") {
		t.Fatalf("expected localized priority label, got %q", out)
	}
}
