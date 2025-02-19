package main

import (
	"context"
	"log/slog"

	"github.com/3lvia/ice-scheduler/scheduler/config"
)

func main() {
	cfg, err := config.New()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	svc, err := NewSchedulerService(ctx, cfg)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create scheduler service", "error", err)
		panic(err)
	}

	if err := svc.Run(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to run scheduler service", "error", err)
		panic(err)
	}

	if err := svc.Stop(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to stop scheduler service", "error", err)
		panic(err)
	}
}