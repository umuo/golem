package main

import "sync"

const defaultRouteLimit = 10_000

// routeStore remembers the Golem conversation required by MessageService.Revoke.
// OneBot delete_msg only carries message_id, while Golem also needs receiver.
type routeStore struct {
	mu    sync.Mutex
	limit int
	items map[string]string
	order []string
}

func newRouteStore(limit int) *routeStore {
	if limit <= 0 {
		limit = defaultRouteLimit
	}
	return &routeStore{limit: limit, items: make(map[string]string, limit)}
}

func (s *routeStore) put(messageID, receiver string) {
	if messageID == "" || messageID == "0" || receiver == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.items[messageID]; exists {
		s.items[messageID] = receiver
		return
	}
	s.items[messageID] = receiver
	s.order = append(s.order, messageID)
	if len(s.order) <= s.limit {
		return
	}
	oldest := s.order[0]
	s.order = s.order[1:]
	delete(s.items, oldest)
}

func (s *routeStore) get(messageID string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	receiver, ok := s.items[messageID]
	return receiver, ok
}
