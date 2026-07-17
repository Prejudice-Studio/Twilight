package store

import (
	"container/heap"
	"sort"
)

type TicketPage struct {
	Tickets []Ticket
	Total   int
}

type ticketIDMinHeap []Ticket

func (h ticketIDMinHeap) Len() int           { return len(h) }
func (h ticketIDMinHeap) Less(i, j int) bool { return h[i].ID < h[j].ID }
func (h ticketIDMinHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *ticketIDMinHeap) Push(x any) {
	*h = append(*h, x.(Ticket))
}

func (h *ticketIDMinHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

func (s *Store) ListTicketsPage(filter TicketFilter, page, perPage int) TicketPage {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	offset := (page - 1) * perPage
	window := offset + perPage
	if window < perPage {
		return TicketPage{}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	var top ticketIDMinHeap
	total := 0
	heapReady := false
	for _, t := range s.state.Tickets {
		if !ticketMatchesFilter(t, filter) {
			continue
		}
		total++
		if len(top) < window {
			top = append(top, t)
			if len(top) == window {
				heap.Init(&top)
				heapReady = true
			}
			continue
		}
		if !heapReady {
			heap.Init(&top)
			heapReady = true
		}
		if len(top) > 0 && t.ID > top[0].ID {
			top[0] = t
			heap.Fix(&top, 0)
		}
	}
	if total == 0 || offset >= len(top) {
		return TicketPage{Total: total}
	}
	sort.Slice(top, func(i, j int) bool { return top[i].ID > top[j].ID })
	end := offset + perPage
	if end > len(top) {
		end = len(top)
	}
	tickets := make([]Ticket, end-offset)
	copy(tickets, top[offset:end])
	return TicketPage{Tickets: tickets, Total: total}
}
