package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"
	geopb "location/gen/location/v1"
	"location/internal/config"
	"location/internal/geofence"
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

	if len(os.Args) == 2 && os.Args[1] == "migrate" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := migrate.Run(ctx, db, migrations.FS); err != nil {
			slog.Error("migrations", "error", err)
			os.Exit(1)
		}
		return
	}
	if err := migrate.Check(context.Background(), db, migrations.FS); err != nil {
		slog.Error("schema compatibility", "error", err)
		os.Exit(1)
	}

	repo := repository.NewPostgres(db)
	svc := service.New(repo, m, cfg.MaxSpeedMS)
	h := handler.New(svc)

	mux := http.NewServeMux()
	h.Register(mux)
	h.RegisterTelegram(mux)
	h.RegisterMobile(mux, cfg.MobileTokenHash)
	h.RegisterMeasurements(mux, cfg.MobileTokenHash)
	h.RegisterBatches(mux, cfg.MobileTokenHash)
	h.RegisterUpdates(mux, cfg.MobileTokenHash, cfg.MobileReleaseDir)
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if db.Ping(ctx) != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

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
	server := &http.Server{Addr: cfg.Addr, Handler: otelhttp.NewHandler(m.Middleware(mux), "location"),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	stop, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	geo := &geofence.Store{Pool: db}
	if err := geo.RegisterMetrics(); err != nil {
		slog.Error("geofence metrics unavailable")
	}
	geoDone := make(chan struct{})
	go func() {
		defer close(geoDone)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			ctx, done := context.WithTimeout(stop, 5*time.Second)
			if err := geo.Process(ctx, time.Now().UTC()); err != nil && stop.Err() == nil {
				slog.Error("geofence evaluation failed")
			}
			done()
			select {
			case <-stop.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	defer func() { cancel(); <-geoDone }()
	if len(cfg.GeoTokenHash) > 0 {
		listener, err := net.Listen("tcp", ":50056")
		if err != nil {
			slog.Error("geofence listener failed")
			return
		}
		geoServer := grpc.NewServer(grpc.MaxRecvMsgSize(16384), grpc.UnaryInterceptor(geofence.Auth(cfg.GeoTokenHash)), grpc.StatsHandler(otelgrpc.NewServerHandler()))
		geopb.RegisterGeofencesServiceServer(geoServer, &geofence.Server{Store: geo})
		go func() {
			if err := geoServer.Serve(listener); err != nil && stop.Err() == nil {
				slog.Error("geofence server stopped")
				cancel()
			}
		}()
		defer geoServer.Stop()
	}
	go func() {
		<-stop.Done()
		ctx, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if err := server.Shutdown(ctx); err != nil {
			slog.Error("http shutdown", "error", err)
			server.Close()
		}
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, err)
	}
}
