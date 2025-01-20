package observability

import (
	"context"

	"elvia.io/scheduler/internal/runtime"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
)

func newTraceProvider(ctx context.Context, r *resource.Resource, env runtime.Env) (*trace.TracerProvider, error) {
	traceExporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, err
	}

	opts := []trace.TracerProviderOption{
		trace.WithResource(r),
	}

	switch {
	case env == runtime.Development:
		opts = append(opts, trace.WithSyncer(traceExporter))
	default:
		opts = append(opts, trace.WithBatcher(traceExporter))
	}

	traceProvider := trace.NewTracerProvider(opts...)
	return traceProvider, nil
}