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
	cfg, err := config.New()
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

	slog.InfoContext(ctx, "starting scheduler", "env", cfg.Env)

	var secrets = &config.Secrets{}
	if cfg.VaultAddr != "" {
		slog.InfoContext(ctx, "loading secrets from vault", "vault_addr", cfg.VaultAddr)

		vault, err := config.NewVault(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "failed to configure vault", "error", err)
			panic(err)
		}

		secrets, err = config.NewSecrets(ctx, vault)
		if err != nil {
			slog.ErrorContext(ctx, "failed to get secrets", "error", err)
			panic(err)
		}

		slog.InfoContext(ctx, "loaded secrets from vault")
	}

	nc, err := nats.Connect(cfg.NatsAddr, nats.Token(secrets.NatsToken))
	if err != nil {
		slog.ErrorContext(ctx, "failed to connect to nats server", "error", err)
		panic(err)
	}
	defer nc.Close()

	slog.InfoContext(ctx, "connected to nats server")

	handler, err := scheduler.New(ctx, nc)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create scheduler", "error", err)
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
		slog.InfoContext(ctx, "shutting down scheduler")

		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		apiShutdown(ctx)
	}

	slog.InfoContext(ctx, "scheduler stopped")
}