package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestClient(t *testing.T) {
	ctx := context.Background()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	nc, _ := bootstrapNATS(t, ctx)
	require.NotEmpty(t, nc)

	name := "scheduler_test"

	scheduler, err := New(nc)
	require.NoError(t, err)

	t.Run("Should be able to install and uninstall", func(t *testing.T) {
		t.Parallel()

		si, err := nc.Subscribe("scheduler.install", func(msg *nats.Msg) {
			var scheduledMsg ScheduledMessage
			err := json.Unmarshal(msg.Data, &scheduledMsg)
			require.NoError(t, err)

			require.Equal(t, name, scheduledMsg.Name)

			header := nats.Header{}
			header.Add("status", "ok")
			err = msg.RespondMsg(&nats.Msg{
				Header: header,
			})
			require.NoError(t, err)
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = si.Unsubscribe()
		})

		su, err := nc.Subscribe(fmt.Sprintf("scheduler.uninstall.%s", name), func(msg *nats.Msg) {
			header := nats.Header{}
			header.Add("status", "ok")
			err = msg.RespondMsg(&nats.Msg{
				Header: header,
			})
			require.NoError(t, err)
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = su.Unsubscribe()
		})

		err = scheduler.Install(ctx, name, time.Now(), nil)
		require.NoError(t, err)

		err = scheduler.Uninstall(ctx, name)
		require.NoError(t, err)
	})
}