// Command gofipe serves a web UI and a BFF (Backend for Frontend) API for the
// Brazilian FIPE vehicle price table.
package main

import (
	"cmp"
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// appVersion is overridden at build time with -ldflags "-X main.appVersion=x.y.z".
var appVersion = "dev"

// assets embeds the frontend so the binary is self-contained.
//
//go:embed templates static
var assets embed.FS

// config holds runtime settings read from environment variables.
type config struct {
	Port        string // PORT (default 8080)
	FipeBaseURL string // FIPE_BASE_URL (default public FIPE v2 API)
	FipeToken   string // FIPE_API_TOKEN (optional subscription token)
	// ShutdownDelay (SHUTDOWN_DELAY, e.g. "5s") keeps serving after SIGTERM while
	// /health/ready reports 503, giving load balancers time to stop routing.
	ShutdownDelay time.Duration
}

func loadConfig() (config, error) {
	cfg := config{
		Port:        cmp.Or(os.Getenv("PORT"), "8080"),
		FipeBaseURL: cmp.Or(os.Getenv("FIPE_BASE_URL"), defaultFipeBaseURL),
		FipeToken:   os.Getenv("FIPE_API_TOKEN"),
	}
	if v := os.Getenv("SHUTDOWN_DELAY"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 {
			return cfg, fmt.Errorf("invalid SHUTDOWN_DELAY %q: use a duration such as 5s", v)
		}
		cfg.ShutdownDelay = d
	}
	return cfg, nil
}

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped with error", "error", err)
		os.Exit(1)
	}
}

// newServer builds the application with its dependencies.
func newServer(cfg config) (*server, error) {
	tmpl, err := template.ParseFS(assets, "templates/index.html")
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	static, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, fmt.Errorf("static assets: %w", err)
	}
	return &server{
		fipe:   newFipeClient(cfg.FipeBaseURL, cfg.FipeToken),
		cache:  newTTLCache(),
		prices: newPriceTracker(),
		tmpl:   tmpl,
		static: static,
	}, nil
}

// run starts the HTTP server and blocks until SIGINT/SIGTERM, then shuts down gracefully.
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := setupTelemetry(ctx, os.Stdout)
	if err != nil {
		return err
	}
	defer func() {
		// Flush pending spans and logs; use a fresh context because ctx is already canceled.
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTelemetry(flushCtx); err != nil {
			slog.Error("telemetry shutdown", "error", err)
		}
	}()

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	app, err := newServer(cfg)
	if err != nil {
		return err
	}
	go app.cache.runJanitor(ctx, 10*time.Minute)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           app.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Bind first so readiness only turns true once the port is accepting connections.
	ln, err := new(net.ListenConfig).Listen(ctx, "tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	slog.Info("server starting", "addr", srv.Addr, "version", appVersion,
		"fipe", cfg.FipeBaseURL, "telemetry", telemetryEnabled())

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	app.ready.Store(true)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	stop() // restore default signal handling: a second Ctrl+C exits immediately

	app.ready.Store(false)
	slog.Info("shutting down", "delay", cfg.ShutdownDelay.String())
	time.Sleep(cfg.ShutdownDelay) // let probes observe "not ready" before closing listeners
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
