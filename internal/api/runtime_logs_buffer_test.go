package api

import "testing"

func TestRuntimeLogBufferAppendTrimsInPlace(t *testing.T) {
	buffer := newRuntimeLogBuffer(3)
	for i := 1; i <= 5; i++ {
		buffer.append(RuntimeLogEntry{Message: string(rune('a' + i - 1))})
	}

	entries, next := buffer.snapshot(10, 0)
	if next != 5 {
		t.Fatalf("next=%d, want 5", next)
	}
	if len(entries) != 3 {
		t.Fatalf("len(entries)=%d, want 3: %#v", len(entries), entries)
	}
	want := []string{"c", "d", "e"}
	for i, message := range want {
		if entries[i].Message != message {
			t.Fatalf("entry %d message=%q, want %q in %#v", i, entries[i].Message, message, entries)
		}
	}
}

func TestTrimRuntimeLogBufferEntriesCompactsOversizedCapacity(t *testing.T) {
	entries := make([]RuntimeLogEntry, 6, 32)
	for i := range entries {
		entries[i] = RuntimeLogEntry{ID: int64(i + 1), Attrs: map[string]string{"i": "x"}}
	}

	trimmed := trimRuntimeLogBufferEntries(entries, 3)
	if len(trimmed) != 3 || cap(trimmed) != 3 {
		t.Fatalf("expected len/cap 3, got len=%d cap=%d", len(trimmed), cap(trimmed))
	}
	for i, wantID := range []int64{4, 5, 6} {
		if trimmed[i].ID != wantID {
			t.Fatalf("trimmed[%d].ID=%d, want %d in %#v", i, trimmed[i].ID, wantID, trimmed)
		}
	}
}
