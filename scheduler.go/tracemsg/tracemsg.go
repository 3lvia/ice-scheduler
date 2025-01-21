package tracemsg

import (
	"context"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func NewHeader(ctx context.Context) nats.Header {
	headers := make(nats.Header)
	propagator := otel.GetTextMapPropagator()
	propagator.Inject(ctx, propagation.HeaderCarrier(headers))

	return headers
}

func Extract(ctx context.Context, header nats.Header) context.Context {
	propagator := otel.GetTextMapPropagator()
	ctx = propagator.Extract(ctx, propagation.HeaderCarrier(header))
	return ctx
}

func Inject(ctx context.Context, header nats.Header) nats.Header {
	headers := make(nats.Header)
	propagator := otel.GetTextMapPropagator()
	propagator.Inject(ctx, propagation.HeaderCarrier(headers))

	for k, v := range headers {
		header[k] = v
	}

	return header
}