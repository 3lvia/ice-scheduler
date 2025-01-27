package runtime

import (
	"log/slog"
	"os"

	slogmulti "github.com/samber/slog-multi"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/log/global"
)

func NewLogger(env Env) *slog.Logger {
	var handlers []slog.Handler

	if env == Development {
		handlers = append(handlers, slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
	}

	// Disable OpenTelemetry logging if the environment variable is set
	if os.Getenv("OTEL_SDK_DISABLED") != "true" {
		handlers = append(handlers, otelslog.NewHandler("scheduler", otelslog.WithLoggerProvider(global.GetLoggerProvider())))
	}

	logger := slog.New(slogmulti.Fanout(handlers...))
	slog.SetDefault(logger)

	return logger
}