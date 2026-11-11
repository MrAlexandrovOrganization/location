package geofence

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"time"
)

func (s *Store) RegisterMetrics() error {
	_, err := otel.Meter("location/geofence").Int64ObservableGauge("location.geo.evaluations.pending", metric.WithDescription("Durable location evaluations awaiting processing"), metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		var n int64
		if err := s.Pool.QueryRow(ctx, query("pending_count")).Scan(&n); err != nil {
			return err
		}
		o.Observe(n)
		return nil
	}))
	return err
}
