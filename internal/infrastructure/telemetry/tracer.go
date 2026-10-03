package telemetry

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace" // Outputs spans to stdout/terminal instead of remote collector
	"go.opentelemetry.io/otel/sdk/resource"                 // Identifies service resource attributes
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func InitTracer() (func(context.Context) error, error) {
	if os.Getenv("ENABLE_TRACING") != "true" {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint(), // Human-readable formatted JSON trace output
	)
	if err != nil {
		return nil, fmt.Errorf("creating stdout exporter: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("workhub-service"),
			semconv.ServiceVersion("1.0.0"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter), // Batch spans asynchronously
		sdktrace.WithResource(res),     // Associate provider with service resource metadata
	)
	otel.SetTracerProvider(tp) // Register provider as the global tracer provider

	return tp.Shutdown, nil
}

// Tracer returns the global tracer for the application.
func Tracer() trace.Tracer {
	return otel.Tracer("workhub-service")
}
