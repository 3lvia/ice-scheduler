package scheduler

import (
	"context"
)

type MemoryStore struct {
	messages map[string]*StoredMessage
	revs     map[string]uint64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		messages: make(map[string]*StoredMessage),
		revs:     make(map[string]uint64),
	}
}

func (s *MemoryStore) Get(ctx context.Context, key string) (*StoredMessage, uint64, error) {
	sm, ok := s.messages[key]
	if !ok {
		return nil, 0, ErrKeyNotFound
	}

	rev, ok := s.revs[key]
	if !ok {
		return nil, 0, ErrKeyNotFound
	}
	return sm, rev, nil
}

func (s *MemoryStore) Put(ctx context.Context, key string, sm *StoredMessage) error {
	if _, ok := s.revs[key]; ok {
		s.revs[key]++
	} else {
		s.revs[key] = 0
	}

	s.messages[key] = sm
	return nil
}

func (s *MemoryStore) Update(ctx context.Context, key string, sm *StoredMessage, rev uint64) (uint64, error) {
	if _, ok := s.messages[key]; !ok {
		return 0, ErrKeyNotFound
	}

	curRev := s.revs[key]
	if curRev > rev {
		return 0, ErrRevisionConflict
	}

	curRev++

	s.messages[key] = sm
	s.revs[key] = curRev

	return curRev, nil
}

func (s *MemoryStore) Delete(ctx context.Context, key string) error {
	return s.Purge(ctx, key)
}

func (s *MemoryStore) Purge(ctx context.Context, key string) error {
	delete(s.messages, key)
	delete(s.revs, key)
	return nil
}