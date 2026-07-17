package store

import "testing"

func TestPrependBoundedHeadPreservesOrderAndBoundsCapacity(t *testing.T) {
	items := make([]int, 3, 8)
	copy(items, []int{1, 2, 3})

	items = prependBoundedHead(items, 0, 3)
	if len(items) != 3 || cap(items) != 3 {
		t.Fatalf("expected len/cap 3, got len=%d cap=%d", len(items), cap(items))
	}
	want := []int{0, 1, 2}
	for i, value := range want {
		if items[i] != value {
			t.Fatalf("unexpected item at %d: got %d want %d in %#v", i, items[i], value, items)
		}
	}

	items = prependBoundedHead(items[:2], -1, 3)
	want = []int{-1, 0, 1}
	if len(items) != 3 || cap(items) != 3 {
		t.Fatalf("expected len/cap 3 after append, got len=%d cap=%d", len(items), cap(items))
	}
	for i, value := range want {
		if items[i] != value {
			t.Fatalf("unexpected appended item at %d: got %d want %d in %#v", i, items[i], value, items)
		}
	}
}
