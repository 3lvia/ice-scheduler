package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type State struct {
	At    time.Time `json:"at"`
	Times int       `json:"times"`
}

func NewState(msg ScheduledMessage) State {
	state := State{}
	if msg.At != nil {
		state.At = *msg.At
	} else {
		state.At = time.Now()
	}

	if msg.RepeatPolicy != nil {
		state.Times = msg.RepeatPolicy.Times
	}

	return state
}

type StoredMessage struct {
	Fingerprint      [32]byte         `json:"fingerprint"`
	ScheduledMessage ScheduledMessage `json:"scheduled_message"`
	State            State            `json:"state"`
}

type Store interface {
	Get(ctx context.Context, key string) (*StoredMessage, uint64, error)
	Put(ctx context.Context, key string, sm *StoredMessage) error
	Update(ctx context.Context, key string, sm *StoredMessage, rev uint64) (uint64, error)
	Delete(ctx context.Context, key string) error
	Purge(ctx context.Context, key string) error
}

type JetstreamStore struct {
	kv jetstream.KeyValue
}

func NewJetstreamStore(ctx context.Context, nc *nats.Conn) (*JetstreamStore, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}

	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "scheduled_messages",
		Description: "Installed scheduled messages",
		Storage:     jetstream.FileStorage,
	})
	if err != nil {
		return nil, err
	}

	return &JetstreamStore{
		kv: kv,
	}, nil
}

func (s *JetstreamStore) Get(ctx context.Context, key string) (*StoredMessage, uint64, error) {
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

func (s *JetstreamStore) Put(ctx context.Context, key string, sm *StoredMessage) error {
	d, err := json.Marshal(sm)
	if err != nil {
		return err
	}

	_, err = s.kv.Put(ctx, key, d)
	return err
}

func (s *JetstreamStore) Update(ctx context.Context, key string, sm *StoredMessage, rev uint64) (uint64, error) {
	d, err := json.Marshal(sm)
	if err != nil {
		return 0, err
	}

	return s.kv.Update(ctx, key, d, rev)
}

func (s *JetstreamStore) Delete(ctx context.Context, key string) error {
	return s.kv.Delete(ctx, key)
}

func (s *JetstreamStore) Purge(ctx context.Context, key string) error {
	return s.kv.Purge(ctx, key)
}