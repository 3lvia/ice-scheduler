package observability

import (
	"context"
	"errors"
	"os"

	"github.com/3lvia/ice-scheduler/scheduler/internal/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const TraceNamespace = "ice"
const TraceServiceName = TraceNamespace + "." + "scheduler"
const TracerName = "scheduler"

func Configure(ctx context.Context, env runtime.Env) (shutdown func(context.Context) error, err error) {
	// Disabling of the OpenTelemetry SDK
	// Not implemented yet: https://github.com/open-telemetry/opentelemetry-go/issues/3559
	if os.Getenv("OTEL_SDK_DISABLED") == "true" {
		return
	}

	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(TraceServiceName),
			// semconv.ServiceVersion("v1.0.0"),
			semconv.DeploymentEnvironment(string(env)),
		))
	if err != nil {
		return
	}

	var shutdownFuncs []func(context.Context) error

	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	prop := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	otel.SetTextMapPropagator(prop)

	tracerProvider, err := newTraceProvider(ctx, r, env)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	loggerProvider, err := newLoggerProvider(ctx, r, env)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
	global.SetLoggerProvider(loggerProvider)

	metricsProvider, err := newMetricsProvider(r)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, metricsProvider.Shutdown)
	otel.SetMeterProvider(metricsProvider)

	return
}