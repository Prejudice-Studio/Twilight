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
func (a *App) handleTelegramUpdateBatch(ctx context.Context, updates []telegramUpdate) {
	processTelegramUpdateBatch(ctx, updates, telegramUpdateBatchMaxWorkers, a.handleTelegramUpdateSafely)
}

func (a *App) handleTelegramUpdateSafely(ctx context.Context, update *telegramUpdate) {
	defer func() {
		if recovered := recover(); recovered != nil {
			var updateID int64
			if update != nil {
				updateID = update.UpdateID
			}
			zap.L().Error(
				"telegram update panic",
				zap.Int64("update_id", updateID),
				zap.String("panic", redactSensitiveText(fmt.Sprintf("%v", recovered))),
			)
		}
	}()
	a.handleTelegramUpdate(ctx, update)
}

func processTelegramUpdateBatch(
	ctx context.Context,
	updates []telegramUpdate,
	maxWorkers int,
	handle func(context.Context, *telegramUpdate),
) {
	if len(updates) == 0 || handle == nil {
		return
	}
	if len(updates) == 1 {
		handle(ctx, &updates[0])
		return
	}

	groups := groupTelegramUpdateIndexesByOrdering(updates)
	if len(groups) == 1 {
		for _, index := range groups[0] {
			handle(ctx, &updates[index])
		}
		return
	}
	if maxWorkers <= 0 || maxWorkers > len(groups) {
		maxWorkers = len(groups)
	}

	jobs := make(chan []int)
	var workers sync.WaitGroup
	workers.Add(maxWorkers)
	for range maxWorkers {
		go func() {
			defer workers.Done()
			for group := range jobs {
				for _, index := range group {
					handle(ctx, &updates[index])
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

// groupTelegramUpdateIndexesByOrdering builds connected components over chat
// and user identities without copying the typed update values.
func groupTelegramUpdateIndexesByOrdering(updates []telegramUpdate) [][]int {
	parents := make([]int, len(updates))
	owners := make(map[telegramUpdateStreamKey]int, len(updates)*2)
	for i := range updates {
		parents[i] = i
		keys, count := telegramUpdateOrderingKeys(&updates[i])
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
	groups := make([][]int, 0, len(updates))
	for i := range updates {
		root := telegramFindUpdateGroup(parents, i)
		position, ok := positions[root]
		if !ok {
			position = len(groups)
			positions[root] = position
			groups = append(groups, nil)
		}
		groups[position] = append(groups[position], i)
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

func telegramUpdateOrderingKey(update *telegramUpdate) int64 {
	keys, count := telegramUpdateOrderingKeys(update)
	if count == 0 {
		return 0
	}
	return keys[0].id
}

func telegramUpdateOrderingKeys(update *telegramUpdate) ([4]telegramUpdateStreamKey, int) {
	var keys [4]telegramUpdateStreamKey
	count := 0
	add := func(kind byte, id int64) {
		if id == 0 || count >= len(keys) {
			return
		}
		keys[count] = telegramUpdateStreamKey{kind: kind, id: id}
		count++
	}
	if update == nil {
		return keys, count
	}
	if message := update.Message; message != nil {
		chatID, fromID := telegramMessageOrderingIDs(message)
		add(telegramUpdateStreamChat, chatID)
		add(telegramUpdateStreamUser, fromID)
		return keys, count
	}
	if callback := update.CallbackQuery; callback != nil {
		if message := callback.Message; message != nil {
			chatID, _ := telegramMessageOrderingIDs(message)
			add(telegramUpdateStreamChat, chatID)
		}
		add(telegramUpdateStreamUser, callback.From.ID)
		return keys, count
	}
	event := update.ChatMember
	if event == nil {
		event = update.MyChatMember
	}
	if event != nil {
		add(telegramUpdateStreamChat, event.Chat.ID)
		add(telegramUpdateStreamUser, event.From.ID)
		add(telegramUpdateStreamUser, event.NewChatMember.User.ID)
	}
	return keys, count
}

func telegramMessageOrderingIDs(message *telegramMessage) (chatID, fromID int64) {
	if message == nil {
		return 0, 0
	}
	return message.Chat.ID, message.From.ID
}
