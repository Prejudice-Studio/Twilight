package store

func compactHead[T any](items []T, max int) []T {
	if max <= 0 || len(items) <= max {
		return items
	}
	out := make([]T, max)
	copy(out, items[:max])
	return out
}

func compactTail[T any](items []T, max int) []T {
	if max <= 0 || len(items) <= max {
		return items
	}
	out := make([]T, max)
	copy(out, items[len(items)-max:])
	return out
}
