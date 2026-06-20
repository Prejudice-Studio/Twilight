package api

import "testing"

func TestRuntimeLogBufferSnapshotCursorOrdering(t *testing.T) {
	buffer := newRuntimeLogBuffer(10)
	for i := 1; i <= 5; i++ {
		buffer.append(RuntimeLogEntry{Message: "entry"})
	}

	entries, next := buffer.snapshot(2, 0)
	if len(entries) != 2 || entries[0].ID != 4 || entries[1].ID != 5 || next != 5 {
		t.Fatalf("unexpected latest snapshot next=%d entries=%#v", next, entries)
	}

	entries, next = buffer.snapshot(2, 2)
	if len(entries) != 2 || entries[0].ID != 3 || entries[1].ID != 4 || next != 4 {
		t.Fatalf("unexpected cursor snapshot next=%d entries=%#v", next, entries)
	}

	entries, next = buffer.snapshot(2, 5)
	if len(entries) != 0 || next != 5 {
		t.Fatalf("unexpected empty cursor snapshot next=%d entries=%#v", next, entries)
	}
}
