package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"location/internal/config"
	"location/internal/handler"
	"location/internal/logx"
	"location/internal/metrics"
	"location/internal/migrate"
	"location/internal/repository"
	"location/internal/service"
	"location/internal/tracing"
	"location/migrations"
)

func main() {
	logx.Setup("location")

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}

	metricsHandler, metricsShutdown, err := metrics.Init()
	if err != nil {
		slog.Error("metrics init", "error", err)
		os.Exit(1)
	}
	defer metricsShutdown()

	m, err := metrics.New()
	if err != nil {
		slog.Error("metrics new", "error", err)
		os.Exit(1)
	}

	db, err := pgxpool.New(context.Background(), cfg.PGURL)
	if err != nil {
		slog.Error("postgres", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(context.Background()); err != nil {
		slog.Error("postgres ping", "error", err)
		os.Exit(1)
	}

	if err := migrate.Run(context.Background(), db, migrations.FS); err != nil {
		slog.Error("migrations", "error", err)
		os.Exit(1)
	}

	repo := repository.NewPostgres(db)
	svc := service.New(repo, m, cfg.MaxSpeedMS)
	h := handler.New(svc)

	mux := http.NewServeMux()
	h.Register(mux)

	tracingShutdown, err := tracing.Init(context.Background())
	if err != nil {
		slog.Error("tracing init", "error", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := tracingShutdown(shutdownCtx); err != nil {
			slog.ErrorContext(shutdownCtx, "tracing shutdown", "error", err)
		}
	}()

	go func() {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", metricsHandler)
		slog.Info("starting metrics server", "port", cfg.MetricsPort)
		if err := http.ListenAndServe(":"+cfg.MetricsPort, metricsMux); err != nil {
			slog.Error("metrics server stopped", "error", err)
		}
	}()

	slog.Info("starting", "addr", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, otelhttp.NewHandler(m.Middleware(mux), "location")); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
