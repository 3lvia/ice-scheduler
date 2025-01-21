package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ice-scheduler/scheduler/api"
	"github.com/ice-scheduler/scheduler/config"
	"github.com/ice-scheduler/scheduler/internal/observability"
	"github.com/ice-scheduler/scheduler/internal/runtime"
	"github.com/ice-scheduler/scheduler/scheduler"
	"github.com/nats-io/nats.go"
)

func main() {
	cfg, err := config.NewConfig()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	obsSync, err := observability.Configure(ctx, cfg.Env)
	if err != nil {
		panic(err)
	}
	defer obsSync(ctx)

	_ = runtime.NewLogger(cfg.Env)

	slog.InfoContext(ctx, "Starting scheduler", "env", cfg.Env)

	nc, err := nats.Connect(cfg.NatsConn)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to connect to nats server", "error", err)
		panic(err)
	}
	defer nc.Close()

	slog.InfoContext(ctx, "Connected to nats server")

	handler, err := scheduler.New(ctx, nc)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create scheduler", "error", err)
		panic(err)
	}
	shutdown, err := handler.Start()
	defer shutdown()

	apiShutdown, apiErrChan := api.Serve(cfg.ApiAddr, cfg.Env)

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-apiErrChan:
		if err != nil && errors.Is(err, http.ErrServerClosed) {
			slog.InfoContext(ctx, "API server stopped")
		} else {
			slog.ErrorContext(ctx, "API server stopped unexpectedly", "error", err)
		}
	case <-done:
		slog.InfoContext(ctx, "Shutting down scheduler")

		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		apiShutdown(ctx)
	}

	slog.InfoContext(ctx, "Scheduler stopped")
}