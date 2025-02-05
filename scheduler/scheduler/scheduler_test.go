package scheduler

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	testnats "github.com/testcontainers/testcontainers-go/modules/nats"
)

func bootstrapNATS(t *testing.T, ctx context.Context) (*nats.Conn, jetstream.JetStream) {
	natsContainer, err := testnats.Run(ctx, "nats:latest")
	require.NoError(t, err)
	t.Cleanup(func() {
		if err = natsContainer.Terminate(ctx); err != nil {
			t.Fatal("failed to terminate the test container", err)
		}
	})

	// wait a second for the server to start, otherwise the connection will sometimes fail
	time.Sleep(1 * time.Second)

	uri, err := natsContainer.ConnectionString(ctx)
	require.NoError(t, err)

	nc, err := nats.Connect(uri)
	require.NoError(t, err)
	t.Cleanup(func() {
		nc.Close()
	})

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	return nc, js
}

func Test_Scheduler(t *testing.T) {
	ctx := context.Background()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	nc, _ := bootstrapNATS(t, ctx)

	store := NewMemoryStore()

	scheduler, err := New(ctx, nc, WithMinInterval(1*time.Second), WithStore(store))
	require.NoError(t, err)

	cleanup, err := scheduler.Start()
	require.NoError(t, err)
	t.Cleanup(cleanup)

	t.Run("Future At time should be delayed before sending", func(t *testing.T) {
		t.Parallel()

		resultChan := make(chan *nats.Msg, 1)
		sub, err := nc.Subscribe("test.1", func(msg *nats.Msg) {
			resultChan <- msg
		})

		require.NoError(t, err)
		t.Cleanup(func() {
			_ = sub.Unsubscribe()
		})

		expectedTime := time.Now().Add(2 * time.Second)
		schedulerMsg := ScheduledMessage{
			Name:    "delayed_message",
			Subject: "test.1",
			At:      &expectedTime,
			Payload: []byte("hello"),
		}

		err = scheduler.installSchedule(ctx, &schedulerMsg)
		require.NoError(t, err)

		select {
		case msg := <-resultChan:
			require.WithinDuration(t, expectedTime, time.Now(), 250*time.Millisecond)
			require.Equal(t, "hello", string(msg.Data))
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for message")
		}
	})

	t.Run("Past At time should be sent immediately", func(t *testing.T) {
		t.Parallel()

		resultChan := make(chan *nats.Msg, 1)
		sub, err := nc.Subscribe("test.2", func(msg *nats.Msg) {
			resultChan <- msg
		})

		require.NoError(t, err)
		t.Cleanup(func() {
			_ = sub.Unsubscribe()
		})

		expectedTime := time.Now()
		schedulerMsg := ScheduledMessage{
			Name:    "send_immediately",
			Subject: "test.2",
			At:      &expectedTime,
			Payload: []byte("hello"),
		}

		err = scheduler.installSchedule(ctx, &schedulerMsg)
		require.NoError(t, err)

		select {
		case msg := <-resultChan:
			require.WithinDuration(t, expectedTime, time.Now(), 250*time.Millisecond)
			require.Equal(t, "hello", string(msg.Data))
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for message")
		}
	})

	t.Run("Scheduled message with repeat policy should be sent multiple times", func(t *testing.T) {
		t.Parallel()

		resultChan := make(chan *nats.Msg, 3)
		sub, err := nc.Subscribe("test.3", func(msg *nats.Msg) {
			resultChan <- msg
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = sub.Unsubscribe()
		})

		const expectedResult = 3

		schedulerMsg := ScheduledMessage{
			Name:    "resheduled_message",
			Subject: "test.3",
			At:      nil,
			Payload: []byte("hello"),
			RepeatPolicy: &RepeatPolicy{
				Times:    expectedResult - 1,
				Interval: 2 * time.Second,
			},
		}

		err = scheduler.installSchedule(ctx, &schedulerMsg)
		require.NoError(t, err)

		for i := 0; i < expectedResult; i++ {
			select {
			case msg := <-resultChan:
				require.Equal(t, "hello", string(msg.Data))
			case <-time.After(10 * time.Second):
				t.Fatal("timed out waiting for message")
			}
		}
	})

	t.Run("Scheduled message with updated rev should update the existing message", func(t *testing.T) {
		t.Parallel()

		resultChan := make(chan *nats.Msg, 1)
		sub, err := nc.Subscribe("test.4", func(msg *nats.Msg) {
			resultChan <- msg
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = sub.Unsubscribe()
		})

		expectedTime := time.Now().Add(2 * time.Second)
		schedulerMsg := ScheduledMessage{
			Name:    "updated_rev",
			Subject: "test.4",
			At:      &expectedTime,
			Payload: []byte("hello"),
		}

		err = scheduler.installSchedule(ctx, &schedulerMsg)
		require.NoError(t, err)

		// Update the message
		schedulerMsg.Payload = []byte("world")
		schedulerMsg.Rev = 1

		err = scheduler.installSchedule(ctx, &schedulerMsg)
		require.NoError(t, err)

		select {
		case msg := <-resultChan:
			require.Equal(t, "world", string(msg.Data))
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for message")
		}
	})

	t.Run("Update message with same revision should fail", func(t *testing.T) {
		t.Parallel()

		resultChan := make(chan *nats.Msg, 1)
		sub, err := nc.Subscribe("test.5", func(msg *nats.Msg) {
			resultChan <- msg
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = sub.Unsubscribe()
		})

		expectedTime := time.Now().Add(2 * time.Second)
		schedulerMsg := ScheduledMessage{
			Name:    "failing_rev",
			Subject: "test.5",
			At:      &expectedTime,
			Payload: []byte("hello"),
		}

		err = scheduler.installSchedule(ctx, &schedulerMsg)
		require.NoError(t, err)

		// Update the message
		schedulerMsg.Payload = []byte("world")
		schedulerMsg.Rev = 0

		err = scheduler.installSchedule(ctx, &schedulerMsg)
		require.Error(t, err)

		select {
		case msg := <-resultChan:
			require.Equal(t, "hello", string(msg.Data))
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for message")
		}
	})

	t.Run("Old messages with repeat interval should trigger only once", func(t *testing.T) {
		t.Parallel()

		// The message is scheduled to be sent every 4 seconds
		// The message was scheduled 1 minute and 2 seconds ago
		// The scheduler corrects for old At times and sends the after (At-startTime+Interval) time
		// The message should be sent after 2 seconds
		// Then again after 4 seconds
		expectedReceived := 2

		offset := 2 * time.Second
		oldAt := time.Now().Add(-1 * time.Minute).Add(-offset)
		oldMessage := ScheduledMessage{
			Name:    "old_message",
			Subject: "old.1",
			Rev:     0,
			At:      &oldAt,
			RepeatPolicy: &RepeatPolicy{
				Times:    Infinite,
				Interval: 4 * time.Second,
			},
		}
		fp, err := fingerprint(&oldMessage)
		require.NoError(t, err)

		err = store.Put(ctx, oldMessage.Name, &StoredMessage{
			Fingerprint:      fp,
			ScheduledMessage: oldMessage,
			State:            NewState(oldMessage),
		})
		require.NoError(t, err)

		numReceived := 0
		sub, err := nc.Subscribe("old.1", func(msg *nats.Msg) {
			numReceived++
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = sub.Unsubscribe()
		})

		err = scheduler.publishToScheduleStream(ctx, &oldMessage)
		require.NoError(t, err)

		select {
		case <-time.After(oldMessage.RepeatPolicy.Interval * 2):
			require.Equal(t, expectedReceived, numReceived)
		}
	})
}