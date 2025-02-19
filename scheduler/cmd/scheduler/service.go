package main

import (
	"context"
	"log/slog"

	"github.com/3lvia/ice-scheduler/scheduler/config"
	"github.com/3lvia/ice-scheduler/scheduler/scheduler"
	"github.com/3lvia/libraries-go/pkg/elvia"
	"github.com/nats-io/nats.go"
)

type SchedulerService struct {
	*elvia.Service

	nc      *nats.Conn
	handler *scheduler.Scheduler
}

func NewSchedulerService(ctx context.Context, cfg *config.Config) (*SchedulerService, error) {
	opts := []elvia.ServiceOpt{
		elvia.WithEnvLoggerLevel(cfg.Env),
		elvia.WithAPI(cfg.ApiAddr),
	}

	svc, err := elvia.NewService(ctx, "ice", "scheduler", opts...)
	if err != nil {
		return nil, err
	}

	var secrets = &config.Secrets{}
	if cfg.VaultAddr != "" {
		slog.InfoContext(ctx, "vault addr set, loading secrets", "vault_addr", cfg.VaultAddr)

		vault, err := config.NewVault(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "failed to configure vault", "error", err)
			return nil, err
		}

		secrets, err = config.NewSecrets(ctx, vault)
		if err != nil {
			slog.ErrorContext(ctx, "failed to get secrets", "error", err)
			return nil, err
		}

		slog.InfoContext(ctx, "loaded secrets from vault")
	}

	slog.InfoContext(ctx, "connecting to nats server", "nats_addr", cfg.NatsAddr)

	nc, err := nats.Connect(cfg.NatsAddr,
		nats.Token(secrets.NatsToken),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			slog.ErrorContext(ctx, "disconnected from nats server", "error", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			slog.InfoContext(ctx, "reconnected to nats server")
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			slog.InfoContext(ctx, "connection to nats server closed")
		}),
		nats.ErrorHandler(func(nc *nats.Conn, sub *nats.Subscription, err error) {
			slog.ErrorContext(ctx, "nats error", "error", err)
		}),
	)
	if err != nil {
		slog.ErrorContext(ctx, "failed to connect to nats server", "error", err)
		return nil, err
	}

	slog.InfoContext(ctx, "connected to nats server")

	s, err := scheduler.New(ctx, nc)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create scheduler", "error", err)
		return nil, err
	}

	return &SchedulerService{
		svc,
		nc,
		s,
	}, nil
}

func (s *SchedulerService) Run(ctx context.Context) error {
	shutdown, err := s.handler.Start()
	if err != nil {
		return err
	}
	defer shutdown()

	return s.Service.Run(ctx)
}

func (s *SchedulerService) Stop(ctx context.Context) error {
	s.nc.Close()
	return s.Service.Stop(ctx)
}