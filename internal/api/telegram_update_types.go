package api

// telegramUpdate contains only the Telegram update fields consumed by
// Twilight. encoding/json ignores the rest without materializing dynamic maps.
type telegramUpdate struct {
	UpdateID      int64                     `json:"update_id"`
	Message       *telegramMessage          `json:"message,omitempty"`
	CallbackQuery *telegramCallbackQuery    `json:"callback_query,omitempty"`
	ChatMember    *telegramChatMemberUpdate `json:"chat_member,omitempty"`
	MyChatMember  *telegramChatMemberUpdate `json:"my_chat_member,omitempty"`
}

type telegramMessage struct {
	MessageID      int64                 `json:"message_id"`
	From           telegramUser          `json:"from"`
	SenderChat     telegramChat          `json:"sender_chat"`
	Chat           telegramChat          `json:"chat"`
	Text           string                `json:"text"`
	ReplyToMessage *telegramReplyMessage `json:"reply_to_message,omitempty"`
}

type telegramReplyMessage struct {
	From telegramUser `json:"from"`
}

type telegramCallbackQuery struct {
	ID      string           `json:"id"`
	From    telegramUser     `json:"from"`
	Message *telegramMessage `json:"message,omitempty"`
	Data    string           `json:"data"`
}

type telegramChatMemberUpdate struct {
	Chat          telegramChat       `json:"chat"`
	From          telegramUser       `json:"from"`
	NewChatMember telegramChatMember `json:"new_chat_member"`
}

type telegramChatMember struct {
	User   telegramUser `json:"user"`
	Status string       `json:"status"`
}

type telegramUser struct {
	ID       int64  `json:"id"`
	IsBot    bool   `json:"is_bot"`
	Username string `json:"username"`
}

type telegramChat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Username string `json:"username"`
}
