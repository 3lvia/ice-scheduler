package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_Installer(t *testing.T) {
	ctx := context.Background()

	addStoredMessage := func(store Store, message ScheduledMessage) {
		fp, err := fingerprint(&message)
		require.NoError(t, err)

		err = store.Put(ctx, message.Name, &StoredMessage{
			Fingerprint:      fp,
			ScheduledMessage: message,
			State:            NewState(message),
		})
		require.NoError(t, err)
	}

	minInterval := 2 * time.Second

	storedMessage1 := ScheduledMessage{
		Name:    "stored_message1",
		Subject: "stored.1",
		Rev:     0,
	}
	storedMessage2 := ScheduledMessage{
		Name:    "stored_message2",
		Subject: "stored.2",
		Rev:     0,
	}

	store := NewMemoryStore()
	addStoredMessage(store, storedMessage1)
	addStoredMessage(store, storedMessage2)

	installer := NewInstaller(store, minInterval)

	var tests = []struct {
		name    string
		msg     ScheduledMessage
		updated bool
		err     error
	}{
		{
			name: "valid message should be installed",
			msg: ScheduledMessage{
				Name:    "valid_name",
				Subject: "test.1",
			},
			err:     nil,
			updated: true,
		},
		{
			name: "empty name should return error",
			msg: ScheduledMessage{
				Name: "",
			},
			err:     ErrInvalidName,
			updated: false,
		},
		{
			name: "invalid name should return error",
			msg: ScheduledMessage{
				Name: "invalid.name",
			},
			err:     ErrInvalidName,
			updated: false,
		},
		{
			name: "empty subject should return error",
			msg: ScheduledMessage{
				Name:    "valid_name",
				Subject: "",
			},
			err:     ErrInvalidSubject,
			updated: false,
		},
		{
			name: "short repeat interval should return error",
			msg: ScheduledMessage{
				Name:    "valid_name",
				Subject: "test.1",
				RepeatPolicy: &RepeatPolicy{
					Interval: minInterval - 1*time.Second,
				},
			},
			err:     ErrInvalidInterval,
			updated: false,
		},
		{
			name: "existing message with new rev and fingerprint should be updated",
			msg: ScheduledMessage{
				Name:    storedMessage2.Name,
				Subject: "stored.updated",
				Rev:     1,
			},
			err:     nil,
			updated: true,
		},
		{
			name:    "existing message with same rev and fingerprint should not be updated",
			msg:     storedMessage1,
			err:     nil,
			updated: false,
		},
		{
			name: "existing message with same rev but different fingerprint should return error",
			msg: ScheduledMessage{
				Name:    storedMessage1.Name,
				Subject: "stored.2",
				Rev:     0,
			},
			err:     ErrFingerprintConflict,
			updated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			updated, err := installer.Install(ctx, &tt.msg)

			if updated != tt.updated {
				t.Errorf("expected %v, got %v", tt.updated, updated)
			}

			if tt.err == nil && err != nil {
				t.Errorf("expected nil, got %v", err)
			}

			if !errors.Is(err, tt.err) {
				t.Errorf("expected %v, got %v", tt.err, err)
			}
		})
	}
}