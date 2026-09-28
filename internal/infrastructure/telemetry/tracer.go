package telemetry

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace" // أي بدل إرسالها إلى Jaeger أو OTLP collector، ستظهر في terminal/log output.
	"go.opentelemetry.io/otel/sdk/resource"                 // من هو التطبيق الذي أنتج هذه telemetry؟
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
		stdouttrace.WithPrettyPrint(), // يجعل output مقروءًا للإنسان بدل شكل compact.
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
		sdktrace.WithBatcher(exporter), // يتم تجميع spans ثم إرسالها على دفعات:
		sdktrace.WithResource(res),     // كل traces/spans التي ينتجها هذا provider مرتبطة بهذه الـ resource information.
	)
	otel.SetTracerProvider(tp) //  الآن جعلنا هذا الـ provider هو الـ global provider.

	return tp.Shutdown, nil //
}

// Tracer returns the global tracer for the application.
func Tracer() trace.Tracer {
	return otel.Tracer("workhub-service")
}
