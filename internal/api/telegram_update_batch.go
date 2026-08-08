package api

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"
)

const telegramUpdateBatchMaxWorkers = 8

// handleTelegramUpdateBatch keeps updates from one chat ordered while allowing
// independent chats to make progress concurrently. It returns only after every
// update effect completes, so the caller can durably advance getUpdates offset.
func (a *App) handleTelegramUpdateBatch(ctx context.Context, updates []map[string]any) {
	processTelegramUpdateBatch(ctx, updates, telegramUpdateBatchMaxWorkers, a.handleTelegramUpdateSafely)
}

func (a *App) handleTelegramUpdateSafely(ctx context.Context, update map[string]any) {
	defer func() {
		if recovered := recover(); recovered != nil {
			zap.L().Error(
				"telegram update panic",
				zap.Int64("update_id", numeric(update["update_id"])),
				zap.String("panic", redactSensitiveText(fmt.Sprintf("%v", recovered))),
			)
		}
	}()
	a.handleTelegramUpdate(ctx, update)
}

func processTelegramUpdateBatch(
	ctx context.Context,
	updates []map[string]any,
	maxWorkers int,
	handle func(context.Context, map[string]any),
) {
	if len(updates) == 0 || handle == nil {
		return
	}
	if len(updates) == 1 {
		handle(ctx, updates[0])
		return
	}

	groups := groupTelegramUpdatesByOrdering(updates)
	if len(groups) == 1 {
		for _, update := range groups[0] {
			handle(ctx, update)
		}
		return
	}
	if maxWorkers <= 0 || maxWorkers > len(groups) {
		maxWorkers = len(groups)
	}

	jobs := make(chan []map[string]any)
	var workers sync.WaitGroup
	workers.Add(maxWorkers)
	for range maxWorkers {
		go func() {
			defer workers.Done()
			for group := range jobs {
				for _, update := range group {
					handle(ctx, update)
				}
			}
		}()
	}
	for _, group := range groups {
		jobs <- group
	}
	close(jobs)
	workers.Wait()
}

type telegramUpdateStreamKey struct {
	kind byte
	id   int64
}

const (
	telegramUpdateStreamChat byte = iota + 1
	telegramUpdateStreamUser
	telegramUpdateStreamUnknown
)

// groupTelegramUpdatesByOrdering builds connected components over chat and
// user identities. Updates sharing either identity remain in Telegram order;
// only components with no shared chat or user may run concurrently.
func groupTelegramUpdatesByOrdering(updates []map[string]any) [][]map[string]any {
	parents := make([]int, len(updates))
	owners := make(map[telegramUpdateStreamKey]int, len(updates)*2)
	for i, update := range updates {
		parents[i] = i
		keys, count := telegramUpdateOrderingKeys(update)
		if count == 0 {
			keys[0] = telegramUpdateStreamKey{kind: telegramUpdateStreamUnknown}
			count = 1
		}
		for _, key := range keys[:count] {
			if previous, ok := owners[key]; ok {
				telegramUnionUpdateGroups(parents, i, previous)
			} else {
				owners[key] = i
			}
		}
	}

	positions := make(map[int]int, len(updates))
	groups := make([][]map[string]any, 0, len(updates))
	for i, update := range updates {
		root := telegramFindUpdateGroup(parents, i)
		position, ok := positions[root]
		if !ok {
			position = len(groups)
			positions[root] = position
			groups = append(groups, nil)
		}
		groups[position] = append(groups[position], update)
	}
	return groups
}

func telegramFindUpdateGroup(parents []int, current int) int {
	root := current
	for parents[root] != root {
		root = parents[root]
	}
	for parents[current] != current {
		next := parents[current]
		parents[current] = root
		current = next
	}
	return root
}

func telegramUnionUpdateGroups(parents []int, left, right int) {
	leftRoot := telegramFindUpdateGroup(parents, left)
	rightRoot := telegramFindUpdateGroup(parents, right)
	if leftRoot != rightRoot {
		parents[leftRoot] = rightRoot
	}
}

func telegramUpdateOrderingKey(update map[string]any) int64 {
	keys, count := telegramUpdateOrderingKeys(update)
	if count == 0 {
		return 0
	}
	return keys[0].id
}

func telegramUpdateOrderingKeys(update map[string]any) ([4]telegramUpdateStreamKey, int) {
	var keys [4]telegramUpdateStreamKey
	count := 0
	add := func(kind byte, id int64) {
		if id == 0 || count >= len(keys) {
			return
		}
		keys[count] = telegramUpdateStreamKey{kind: kind, id: id}
		count++
	}
	if message, _ := update["message"].(map[string]any); message != nil {
		chatID, fromID := telegramMessageOrderingIDs(message)
		add(telegramUpdateStreamChat, chatID)
		add(telegramUpdateStreamUser, fromID)
		return keys, count
	}
	if callback, _ := update["callback_query"].(map[string]any); callback != nil {
		if message, _ := callback["message"].(map[string]any); message != nil {
			chatID, _ := telegramMessageOrderingIDs(message)
			add(telegramUpdateStreamChat, chatID)
		}
		if from, _ := callback["from"].(map[string]any); from != nil {
			add(telegramUpdateStreamUser, numeric(from["id"]))
		}
		return keys, count
	}
	for _, field := range []string{"chat_member", "my_chat_member"} {
		if event, _ := update[field].(map[string]any); event != nil {
			if chat, _ := event["chat"].(map[string]any); chat != nil {
				add(telegramUpdateStreamChat, numeric(chat["id"]))
			}
			if from, _ := event["from"].(map[string]any); from != nil {
				add(telegramUpdateStreamUser, numeric(from["id"]))
			}
			if member, _ := event["new_chat_member"].(map[string]any); member != nil {
				if user, _ := member["user"].(map[string]any); user != nil {
					add(telegramUpdateStreamUser, numeric(user["id"]))
				}
			}
			return keys, count
		}
	}
	return keys, count
}

func telegramMessageOrderingIDs(message map[string]any) (chatID, fromID int64) {
	if chat, _ := message["chat"].(map[string]any); chat != nil {
		chatID = numeric(chat["id"])
	}
	if from, _ := message["from"].(map[string]any); from != nil {
		fromID = numeric(from["id"])
	}
	return chatID, fromID
}
