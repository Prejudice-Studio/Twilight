package api

import (
	"errors"
	"strings"

	"github.com/prejudice-studio/twilight/internal/store"
)

var (
	errTicketReplyForbidden = errors.New("ticket reply forbidden")
	errTicketReplyEmpty     = errors.New("ticket reply empty")
	errTicketReplyTooLong   = errors.New("ticket reply too long")
)

const maxTicketReplyLength = 5000

// appendTicketReply is the application operation shared by legacy and V2
// transport handlers. It performs the ownership/status check against the
// latest Store snapshot and appends through the Store's atomic mutation;
// callers remain responsible for rate limits, audit and notifications.
func (a *App) appendTicketReply(ticketID int64, actor store.User, content string) (store.Ticket, store.Ticket, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return store.Ticket{}, store.Ticket{}, errTicketReplyEmpty
	}
	if len(content) > maxTicketReplyLength {
		return store.Ticket{}, store.Ticket{}, errTicketReplyTooLong
	}

	existing, found := a.store().Ticket(ticketID)
	if !found {
		return store.Ticket{}, store.Ticket{}, store.ErrNotFound
	}
	if actor.Role != store.RoleAdmin && existing.UID != actor.UID {
		return store.Ticket{}, existing, errTicketReplyForbidden
	}
	if actor.Role != store.RoleAdmin && !store.TicketStatusAllowsConversation(existing.Status) {
		return store.Ticket{}, existing, store.ErrTicketClosed
	}

	updated, err := a.store().AddTicketReply(ticketID, store.TicketReply{
		UID: actor.UID, Username: actor.Username, Role: actor.Role, Content: content,
	})
	if err != nil {
		return store.Ticket{}, existing, err
	}
	return updated, existing, nil
}
