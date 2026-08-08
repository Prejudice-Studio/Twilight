package api

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTelegramUpdateBatchPreservesOrderWithinChat(t *testing.T) {
	updates := []map[string]any{
		telegramTestMessageUpdate(1, 100),
		telegramTestMessageUpdate(2, 200),
		telegramTestMessageUpdate(3, 100),
		telegramTestMessageUpdate(4, 200),
	}
	seen := map[int64][]int64{}
	var mu sync.Mutex
	processTelegramUpdateBatch(context.Background(), updates, 2, func(_ context.Context, update map[string]any) {
		chatID := telegramUpdateOrderingKey(update)
		mu.Lock()
		seen[chatID] = append(seen[chatID], numeric(update["update_id"]))
		mu.Unlock()
	})

	if got := seen[100]; !sameTelegramUpdateIDs(got, []int64{1, 3}) {
		t.Fatalf("chat 100 order=%v, want [1 3]", got)
	}
	if got := seen[200]; !sameTelegramUpdateIDs(got, []int64{2, 4}) {
		t.Fatalf("chat 200 order=%v, want [2 4]", got)
	}
}

func TestTelegramUpdateBatchPreservesOrderAcrossSharedUserAndChat(t *testing.T) {
	updates := []map[string]any{
		telegramTestMessageUpdateFrom(1, 100, 7),
		telegramTestMessageUpdateFrom(2, 200, 7),
		telegramTestMessageUpdateFrom(3, 200, 8),
		telegramTestMessageUpdateFrom(4, 300, 9),
	}
	groups := groupTelegramUpdatesByOrdering(updates)
	if len(groups) != 2 {
		t.Fatalf("group count=%d, want 2", len(groups))
	}
	shared := make([]int64, 0, len(groups[0]))
	for _, update := range groups[0] {
		shared = append(shared, numeric(update["update_id"]))
	}
	if !sameTelegramUpdateIDs(shared, []int64{1, 2, 3}) {
		t.Fatalf("shared user/chat group order=%v, want [1 2 3]", shared)
	}
}

func TestTelegramUpdateBatchRunsIndependentChatsConcurrentlyAndWaits(t *testing.T) {
	updates := []map[string]any{
		telegramTestMessageUpdate(1, 100),
		telegramTestMessageUpdate(2, 200),
	}
	started := make(chan int64, len(updates))
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		processTelegramUpdateBatch(context.Background(), updates, 2, func(_ context.Context, update map[string]any) {
			started <- telegramUpdateOrderingKey(update)
			<-release
		})
		close(done)
	}()

	for range updates {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("independent chat was blocked behind another chat")
		}
	}
	select {
	case <-done:
		t.Fatal("batch returned before update effects completed")
	default:
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("batch did not return after all update effects completed")
	}
}

func TestTelegramUpdateBatchBoundsConcurrency(t *testing.T) {
	updates := make([]map[string]any, 0, 20)
	for i := int64(1); i <= 20; i++ {
		updates = append(updates, telegramTestMessageUpdate(i, i))
	}
	var active atomic.Int32
	var peak atomic.Int32
	processTelegramUpdateBatch(context.Background(), updates, 3, func(_ context.Context, _ map[string]any) {
		current := active.Add(1)
		for {
			previous := peak.Load()
			if current <= previous || peak.CompareAndSwap(previous, current) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		active.Add(-1)
	})
	if got := peak.Load(); got < 2 || got > 3 {
		t.Fatalf("peak concurrency=%d, want 2..3", got)
	}
}

func TestTelegramUpdateOrderingKeyCoversCallbacksAndMembership(t *testing.T) {
	callback := map[string]any{
		"callback_query": map[string]any{
			"from":    map[string]any{"id": int64(7)},
			"message": map[string]any{"chat": map[string]any{"id": int64(-100)}},
		},
	}
	if got := telegramUpdateOrderingKey(callback); got != -100 {
		t.Fatalf("callback ordering key=%d, want -100", got)
	}
	inlineCallback := map[string]any{"callback_query": map[string]any{"from": map[string]any{"id": int64(7)}}}
	if got := telegramUpdateOrderingKey(inlineCallback); got != 7 {
		t.Fatalf("inline callback ordering key=%d, want 7", got)
	}
	membership := map[string]any{"chat_member": map[string]any{"chat": map[string]any{"id": int64(-200)}}}
	if got := telegramUpdateOrderingKey(membership); got != -200 {
		t.Fatalf("membership ordering key=%d, want -200", got)
	}
}

func telegramTestMessageUpdate(updateID, chatID int64) map[string]any {
	return telegramTestMessageUpdateFrom(updateID, chatID, chatID)
}

func telegramTestMessageUpdateFrom(updateID, chatID, fromID int64) map[string]any {
	return map[string]any{
		"update_id": updateID,
		"message": map[string]any{
			"chat": map[string]any{"id": chatID},
			"from": map[string]any{"id": fromID},
		},
	}
}

func sameTelegramUpdateIDs(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
