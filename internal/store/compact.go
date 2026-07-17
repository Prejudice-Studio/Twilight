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

func prependBoundedHead[T any](items []T, item T, max int) []T {
	if max <= 0 {
		return nil
	}
	if len(items) >= max {
		if cap(items) > max {
			out := make([]T, max)
			copy(out, items[:max])
			items = out
		} else {
			items = items[:max]
		}
		copy(items[1:], items[:max-1])
		items[0] = item
		return items
	}
	if cap(items) > max {
		out := make([]T, len(items), max)
		copy(out, items)
		items = out
	}
	items = append(items, item)
	copy(items[1:], items[:len(items)-1])
	items[0] = item
	return items
}
