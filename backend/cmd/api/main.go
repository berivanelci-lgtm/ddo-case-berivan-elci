package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/audit"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/auth"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/catalogstore"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/config"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/database"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/genealogy"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/genealogystore"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/httpapi"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/idempotency"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/metrics"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/ncrstore"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/spc"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/spcstore"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/userstore"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/workorder"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/workorderstore"
)

const (
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 10 * time.Second
	seedTimeout       = 30 * time.Second
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	if err := run(); err != nil {
		slog.Error("uygulama durdu", "error", err)
		os.Exit(1)
	}
}

// run, uygulamayı başlatır ve kapanış sinyaline kadar çalıştırır.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	tokens, err := auth.NewTokenManager(cfg.JWTSecret, cfg.AccessTTL, cfg.RefreshTTL)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := database.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	users := userstore.New(pool)

	seedCtx, cancelSeed := context.WithTimeout(ctx, seedTimeout)
	created, err := user.SeedDemoUsers(seedCtx, users, auth.HashPassword, cfg.DemoPassword)
	cancelSeed()
	if err != nil {
		return err
	}
	slog.Info("demo hesaplar hazır", "eklenen", created)

	authService, err := auth.NewService(users, tokens)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	registry := metrics.New()
	mux.Handle("GET /metrics", registry.Handler())

	mux.HandleFunc("GET /health", httpapi.HealthHandler(pool))
	mux.HandleFunc("POST /auth/login", httpapi.LoginHandler(authService))
	authenticate := httpapi.Authenticate(tokens)
	mux.Handle("GET /me", authenticate(httpapi.MeHandler()))

	spcStore := spcstore.New(pool)
	httpapi.NewChartsAPI(spc.NewService(spcStore), spcStore).Register(mux, authenticate)

	httpapi.NewCatalogAPI(catalogstore.New(pool)).Register(mux, authenticate)

	orderStore := workorderstore.New(pool)
	httpapi.NewWorkOrdersAPI(workorder.NewService(orderStore), orderStore).
		WithIdempotency(idempotency.NewStore(pool)).
		Register(mux, authenticate)
	httpapi.NewAuditAPI(audit.NewReader(pool)).Register(mux, authenticate)
	httpapi.NewTraceabilityAPI(genealogy.NewService(genealogystore.New(pool))).Register(mux, authenticate)
	httpapi.NewNCRsAPI(ncrstore.New(pool)).Register(mux, authenticate)
	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: registry.Middleware(mux), ReadHeaderTimeout: readHeaderTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("API dinliyor", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		slog.Info("kapanış sinyali alındı")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	return server.Shutdown(shutdownCtx)
}
