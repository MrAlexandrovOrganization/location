package metrics

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelprometheus "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

type Metrics struct {
	savesTotal    metric.Int64Counter
	filteredTotal metric.Int64Counter
	httpRequests  metric.Int64Counter
	httpDuration  metric.Float64Histogram
}

// Init sets up the OTel → Prometheus pipeline and returns the /metrics HTTP handler.
func Init() (http.Handler, func(), error) {
	exporter, err := otelprometheus.New()
	if err != nil {
		return nil, nil, err
	}
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	otel.SetMeterProvider(provider)
	shutdown := func() { _ = provider.Shutdown(context.Background()) }
	return promhttp.Handler(), shutdown, nil
}

// New creates a Metrics instance wired to the global MeterProvider.
// Must be called after Init().
func New() (*Metrics, error) {
	meter := otel.GetMeterProvider().Meter("location")

	savesTotal, err := meter.Int64Counter("location.saves.total",
		metric.WithDescription("Total location points saved"),
	)
	if err != nil {
		return nil, err
	}

	filteredTotal, err := meter.Int64Counter("location.filtered.total",
		metric.WithDescription("Points rejected by velocity filter"),
	)
	if err != nil {
		return nil, err
	}

	httpRequests, err := meter.Int64Counter("location.http.requests.total",
		metric.WithDescription("Total HTTP requests by method, path and status"),
	)
	if err != nil {
		return nil, err
	}

	httpDuration, err := meter.Float64Histogram("location.http.request.duration.seconds",
		metric.WithDescription("HTTP request duration"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	return &Metrics{
		savesTotal:    savesTotal,
		filteredTotal: filteredTotal,
		httpRequests:  httpRequests,
		httpDuration:  httpDuration,
	}, nil
}

// RecordFiltered increments the velocity-filter rejection counter.
func (m *Metrics) RecordFiltered(ctx context.Context) {
	m.filteredTotal.Add(ctx, 1)
}

// RecordSave increments the save counter.
func (m *Metrics) RecordSave(ctx context.Context, live bool, hidden bool) {
	kind := "once"
	if live {
		kind = "live"
	}
	m.savesTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("kind", kind),
		attribute.Bool("hidden", hidden),
	))
}

// Middleware wraps an http.Handler and records request count and duration.
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		dur := time.Since(start).Seconds()

		attrs := metric.WithAttributes(
			attribute.String("method", r.Method),
			attribute.String("path", r.URL.Path),
			attribute.String("status", strconv.Itoa(rw.status)),
		)
		m.httpRequests.Add(r.Context(), 1, attrs)
		m.httpDuration.Record(r.Context(), dur, metric.WithAttributes(
			attribute.String("method", r.Method),
			attribute.String("path", r.URL.Path),
		))
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
