package observability

import (
	"context"
	"os"
	"strings"
	"sync"

	"elvia.io/scheduler/internal/runtime"
	"go.opentelemetry.io/contrib/processors/minsev"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

const key = "OTEL_LOG_LEVEL"

var getSeverity = sync.OnceValue(func() log.Severity {
	conv := map[string]log.Severity{
		"":      log.SeverityInfo, // Default to SeverityInfo for unset.
		"debug": log.SeverityDebug,
		"info":  log.SeverityInfo,
		"warn":  log.SeverityWarn,
		"error": log.SeverityError,
	}
	// log.SeverityUndefined for unknown values.
	return conv[strings.ToLower(os.Getenv(key))]
})

type EnvSeverity struct{}

func (EnvSeverity) Severity() log.Severity { return getSeverity() }

func newLoggerProvider(ctx context.Context, r *resource.Resource, env runtime.Env) (*sdklog.LoggerProvider, error) {
	logExporter, err := otlploggrpc.New(ctx)
	if err != nil {
		return nil, err
	}

	opts := []sdklog.LoggerProviderOption{
		sdklog.WithResource(r),
	}

	switch {
	case env == runtime.Development:
		opts = append(opts, sdklog.WithProcessor(minsev.NewLogProcessor(sdklog.NewSimpleProcessor(logExporter), EnvSeverity{})))
	default:
		opts = append(opts, sdklog.WithProcessor(minsev.NewLogProcessor(sdklog.NewBatchProcessor(logExporter), EnvSeverity{})))
	}

	loggerProvider := sdklog.NewLoggerProvider(opts...)
	return loggerProvider, nil
}