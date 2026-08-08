package api

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTelegramUpdateBatchPreservesOrderWithinChat(t *testing.T) {
	updates := []telegramUpdate{
		telegramTestMessageUpdate(1, 100),
		telegramTestMessageUpdate(2, 200),
		telegramTestMessageUpdate(3, 100),
		telegramTestMessageUpdate(4, 200),
	}
	seen := map[int64][]int64{}
	var mu sync.Mutex
	processTelegramUpdateBatch(context.Background(), updates, 2, func(_ context.Context, update *telegramUpdate) {
		chatID := telegramUpdateOrderingKey(update)
		mu.Lock()
		seen[chatID] = append(seen[chatID], update.UpdateID)
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
	updates := []telegramUpdate{
		telegramTestMessageUpdateFrom(1, 100, 7),
		telegramTestMessageUpdateFrom(2, 200, 7),
		telegramTestMessageUpdateFrom(3, 200, 8),
		telegramTestMessageUpdateFrom(4, 300, 9),
	}
	groups := groupTelegramUpdateIndexesByOrdering(updates)
	if len(groups) != 2 {
		t.Fatalf("group count=%d, want 2", len(groups))
	}
	shared := make([]int64, 0, len(groups[0]))
	for _, index := range groups[0] {
		shared = append(shared, updates[index].UpdateID)
	}
	if !sameTelegramUpdateIDs(shared, []int64{1, 2, 3}) {
		t.Fatalf("shared user/chat group order=%v, want [1 2 3]", shared)
	}
}

func TestTelegramUpdateBatchRunsIndependentChatsConcurrentlyAndWaits(t *testing.T) {
	updates := []telegramUpdate{
		telegramTestMessageUpdate(1, 100),
		telegramTestMessageUpdate(2, 200),
	}
	started := make(chan int64, len(updates))
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		processTelegramUpdateBatch(context.Background(), updates, 2, func(_ context.Context, update *telegramUpdate) {
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
	updates := make([]telegramUpdate, 0, 20)
	for i := int64(1); i <= 20; i++ {
		updates = append(updates, telegramTestMessageUpdate(i, i))
	}
	var active atomic.Int32
	var peak atomic.Int32
	processTelegramUpdateBatch(context.Background(), updates, 3, func(_ context.Context, _ *telegramUpdate) {
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
	callback := telegramUpdate{
		CallbackQuery: &telegramCallbackQuery{
			From:    telegramUser{ID: 7},
			Message: &telegramMessage{Chat: telegramChat{ID: -100}},
		},
	}
	if got := telegramUpdateOrderingKey(&callback); got != -100 {
		t.Fatalf("callback ordering key=%d, want -100", got)
	}
	inlineCallback := telegramUpdate{CallbackQuery: &telegramCallbackQuery{From: telegramUser{ID: 7}}}
	if got := telegramUpdateOrderingKey(&inlineCallback); got != 7 {
		t.Fatalf("inline callback ordering key=%d, want 7", got)
	}
	membership := telegramUpdate{ChatMember: &telegramChatMemberUpdate{Chat: telegramChat{ID: -200}}}
	if got := telegramUpdateOrderingKey(&membership); got != -200 {
		t.Fatalf("membership ordering key=%d, want -200", got)
	}
}

func TestTelegramCallbackDataParsersAreStrictAndAllocationFree(t *testing.T) {
	mode, action, token, ok := telegramParsePanelCallback("gadm:act:close:panel-token")
	if !ok || mode != "act" || action != "close" || token != "panel-token" {
		t.Fatalf("panel callback parse=%q %q %q %v", mode, action, token, ok)
	}
	mode, action, token, ok = telegramParsePanelCallback("gadm:auth:panel-token")
	if !ok || mode != "auth" || action != "" || token != "panel-token" {
		t.Fatalf("panel auth parse=%q %q %q %v", mode, action, token, ok)
	}
	for _, malformed := range []string{"", "gadm", "gadm:act", "gadm:act:close", "gadm:act:close:token:extra", "gadm:auth:token:extra"} {
		if _, _, _, parsed := telegramParsePanelCallback(malformed); parsed {
			t.Fatalf("malformed panel callback accepted: %q", malformed)
		}
	}

	jsToken, index, ok := telegramParseDeveloperJSCallback("djs:callback-token:7")
	if !ok || jsToken != "callback-token" || index != 7 {
		t.Fatalf("developer callback parse=%q %d %v", jsToken, index, ok)
	}
	for _, malformed := range []string{"", "djs", "djs:token", "djs::1", "djs:token:x", "djs:token:1:extra"} {
		if _, _, parsed := telegramParseDeveloperJSCallback(malformed); parsed {
			t.Fatalf("malformed developer callback accepted: %q", malformed)
		}
	}

	if allocations := testing.AllocsPerRun(100, func() {
		_, _, _, _ = telegramParsePanelCallback("gadm:act:close:panel-token")
		_, _, _ = telegramParseDeveloperJSCallback("djs:callback-token:7")
	}); allocations != 0 {
		t.Fatalf("callback parser allocated %.2f objects per run", allocations)
	}
}

func telegramTestMessageUpdate(updateID, chatID int64) telegramUpdate {
	return telegramTestMessageUpdateFrom(updateID, chatID, chatID)
}

func telegramTestMessageUpdateFrom(updateID, chatID, fromID int64) telegramUpdate {
	return telegramUpdate{
		UpdateID: updateID,
		Message: &telegramMessage{
			Chat: telegramChat{ID: chatID},
			From: telegramUser{ID: fromID},
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
