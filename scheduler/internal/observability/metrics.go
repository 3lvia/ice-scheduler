package observability

import (
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

func newMetricsProvider(r *resource.Resource) (*metric.MeterProvider, error) {
	prom, err := prometheus.New()
	if err != nil {
		return nil, err
	}

	provider := metric.NewMeterProvider(
		metric.WithResource(r),
		metric.WithReader(prom),
	)

	return provider, nil
}