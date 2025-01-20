package client

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	scheduler2 "elvia.io/scheduler/internal/scheduler"
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

	scheduler, err := scheduler2.NewScheduler(ctx, nc)
	require.NoError(t, err)

	cleanup, err := scheduler.Start()
	require.NoError(t, err)
	t.Cleanup(cleanup)

	client, err := NewClient(nc)
	require.NoError(t, err)

	name := "scheduler_test"

	err = client.Install(ctx, name, time.Now(), nil)
	require.NoError(t, err)

	err = client.Uninstall(ctx, name)
	require.NoError(t, err)
}