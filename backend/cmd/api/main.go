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

	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/auth"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/config"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/database"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/httpapi"
	"github.com/berivanelci-lgtm/ddo-case-berivan-elci/backend/internal/user"
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

	users := user.NewRepository(pool)

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
	mux.HandleFunc("GET /health", httpapi.HealthHandler(pool))
	mux.HandleFunc("POST /auth/login", httpapi.LoginHandler(authService))

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
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
