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

func TestWithNATS(t *testing.T) {
	ctx := context.Background()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	nc, _ := bootstrapNATS(t, ctx)

	scheduler, err := New(ctx, nc)
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
			Name:    "test_1",
			Subject: "test.1",
			At:      expectedTime,
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
			Name:    "test_2",
			Subject: "test.2",
			At:      expectedTime,
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
			Name:    "test_3",
			Subject: "test.3",
			At:      time.Now(),
			Payload: []byte("hello"),
			RepeatPolicy: &RepeatPolicy{
				Times:    expectedResult - 1,
				Interval: 5 * time.Second,
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

	t.Run("Scheduled message with same name should update the existing message", func(t *testing.T) {
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
			Name:    "test_4",
			Subject: "test.4",
			At:      expectedTime,
			Payload: []byte("hello"),
		}

		err = scheduler.installSchedule(ctx, &schedulerMsg)
		require.NoError(t, err)

		// schedulerMsgBytes, err := json.Marshal(schedulerMsg)
		// require.NoError(t, err)
		//
		// _, err = js.Publish(ctx, "scheduled.test.4", schedulerMsgBytes)
		// require.NoError(t, err)

		// Update the message
		schedulerMsg.At = time.Now().Add(3 * time.Second)
		schedulerMsg.Payload = []byte("world")
		schedulerMsg.Rev = 1
		// schedulerMsgBytes, err = json.Marshal(schedulerMsg)
		// require.NoError(t, err)

		// _, err = js.Publish(ctx, "scheduled.test.4", schedulerMsgBytes)
		err = scheduler.installSchedule(ctx, &schedulerMsg)
		require.NoError(t, err)

		select {
		case msg := <-resultChan:
			require.Equal(t, "world", string(msg.Data))
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for message")
		}
	})

	// t.Run("Update message with same revision should fail", func(t *testing.T) {
	// 	t.Parallel()
	//
	// 	resultChan := make(chan *nats.Msg, 1)
	// 	sub, err := nc.Subscribe("test.5", func(msg *nats.Msg) {
	// 		resultChan <- msg
	// 	})
	// 	require.NoError(t, err)
	// 	t.Cleanup(func() {
	// 		_ = sub.Unsubscribe()
	// 	})
	//
	// 	expectedTime := time.Now().Add(2 * time.Second)
	// 	schedulerMsg := ScheduledMessage{
	// 		Name:    "test_5",
	// 		At:      expectedTime,
	// 		Payload: []byte("hello"),
	// 	}
	//
	// 	schedulerMsgBytes, err := json.Marshal(schedulerMsg)
	// 	require.NoError(t, err)
	//
	// 	_, err = js.Publish(ctx, "scheduled.test.5", schedulerMsgBytes)
	// 	require.NoError(t, err)
	//
	// 	// Update the message
	// 	schedulerMsg.Payload = []byte("world")
	// 	schedulerMsg.Rev = 0
	// 	schedulerMsgBytes, err = json.Marshal(schedulerMsg)
	// 	require.NoError(t, err)
	//
	// 	_, err = js.Publish(ctx, "scheduled.test.5", schedulerMsgBytes)
	//
	// 	select {
	// 	case msg := <-resultChan:
	// 		require.Equal(t, "hello", string(msg.Data))
	// 	case <-time.After(5 * time.Second):
	// 		t.Fatal("timed out waiting for message")
	// 	}
	// })
}