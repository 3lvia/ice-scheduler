package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"

	"github.com/nats-io/nats.go/jetstream"
)

type StoredMessage struct {
	Fingerprint      [32]byte         `json:"fingerprint"`
	ScheduledMessage ScheduledMessage `json:"scheduled_message"`
}

type Store struct {
	kv jetstream.KeyValue
}

func NewStore(ctx context.Context, js jetstream.JetStream) (*Store, error) {
	kv, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "scheduled_messages",
		Description: "Installed scheduled messages",
		Storage:     jetstream.FileStorage,
	})
	if err != nil {
		return nil, err
	}

	return &Store{
		kv: kv,
	}, nil
}

func (s *Store) Fingerprint(msg *ScheduledMessage) ([32]byte, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return [32]byte{}, errors.Join(ErrFailedToFingerprint, err)
	}
	return sha256.Sum256(data), nil
}

func (s *Store) Get(ctx context.Context, key string) (*StoredMessage, uint64, error) {
	kve, err := s.kv.Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil, 0, ErrKeyNotFound
	}

	if err != nil {
		return nil, 0, err
	}

	var sm StoredMessage
	err = json.Unmarshal(kve.Value(), &sm)
	if err != nil {
		return nil, 0, err
	}

	return &sm, kve.Revision(), nil
}

func (s *Store) PutWithFingerprint(ctx context.Context, key string, sm *ScheduledMessage) error {
	fp, err := s.Fingerprint(sm)
	if err != nil {
		return err
	}

	err = s.Put(ctx, key, &StoredMessage{
		Fingerprint:      fp,
		ScheduledMessage: *sm,
	})
	return err
}

func (s *Store) Put(ctx context.Context, key string, sm *StoredMessage) error {
	d, err := json.Marshal(sm)
	if err != nil {
		return err
	}

	_, err = s.kv.Put(ctx, key, d)
	return err
}

func (s *Store) UpdateWithFingerprint(ctx context.Context, key string, sm *ScheduledMessage, rev uint64) (uint64, error) {
	fp, err := s.Fingerprint(sm)
	if err != nil {
		return 0, err
	}

	return s.Update(ctx, key, &StoredMessage{
		Fingerprint:      fp,
		ScheduledMessage: *sm,
	}, rev)
}

func (s *Store) Update(ctx context.Context, key string, sm *StoredMessage, rev uint64) (uint64, error) {
	d, err := json.Marshal(sm)
	if err != nil {
		return 0, err
	}

	return s.kv.Update(ctx, key, d, rev)
}

func (s *Store) Delete(ctx context.Context, key string) error {
	return s.kv.Delete(ctx, key)
}

func (s *Store) Purge(ctx context.Context, key string) error {
	return s.kv.Purge(ctx, key)
}