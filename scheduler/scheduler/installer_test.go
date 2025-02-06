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

	minInterval := 2 * time.Second

	storedMessage := ScheduledMessage{
		Name:    "stored_message",
		Subject: "stored.1",
		Rev:     0,
	}
	fp, err := fingerprint(&storedMessage)
	require.NoError(t, err)

	store := NewMemoryStore()
	err = store.Put(ctx, "stored_message", &StoredMessage{
		Fingerprint:      fp,
		ScheduledMessage: storedMessage,
		State:            NewState(storedMessage),
	})
	require.NoError(t, err)

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
				Name:    "stored_message",
				Subject: "stored.updated",
				Rev:     1,
			},
			err:     nil,
			updated: true,
		},
		{
			name:    "existing message with same rev and fingerprint should not be updated",
			msg:     storedMessage,
			err:     nil,
			updated: false,
		},
		{
			name: "existing message with same rev but different fingerprint should return error",
			msg: ScheduledMessage{
				Name:    "stored_message",
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