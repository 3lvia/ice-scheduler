package runtime

import (
	"log/slog"
	"os"

	slogmulti "github.com/samber/slog-multi"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/log/global"
)

func NewLogger(env Env) *slog.Logger {
	var handler slog.Handler
	handler = otelslog.NewHandler("scheduler", otelslog.WithLoggerProvider(global.GetLoggerProvider()))

	if env == Development {
		handler = slogmulti.Fanout(handler, slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logger
}