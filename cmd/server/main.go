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

	"github.com/11DingKing/asean-ai-launchpad/internal/clock"
	"github.com/11DingKing/asean-ai-launchpad/internal/config"
	"github.com/11DingKing/asean-ai-launchpad/internal/health"
	"github.com/11DingKing/asean-ai-launchpad/internal/httpapi"
	"github.com/11DingKing/asean-ai-launchpad/internal/idgen"
	"github.com/11DingKing/asean-ai-launchpad/internal/middleware"
	"github.com/11DingKing/asean-ai-launchpad/internal/service"
	"github.com/11DingKing/asean-ai-launchpad/internal/store"
	"github.com/11DingKing/asean-ai-launchpad/internal/worker"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(context.Background(), log); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(parent context.Context, log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stopSignals := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	database, err := store.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer database.Close()
	clk := clock.Real{}
	ids := idgen.Random{}
	services := service.New(database, clk, ids, cfg.SessionTTL)
	if cfg.BootstrapEmail != "" {
		user, err := services.BootstrapOperator(ctx, cfg.BootstrapEmail, cfg.BootstrapPassword)
		if err != nil {
			return err
		}
		log.Info("bootstrap operator ready", "user_id", user.ID, "email", user.Email)
	}
	checker := health.New(database)
	api := (&httpapi.API{Service: services, Health: checker}).Handler()
	handler := middleware.Stack{Log: log, IDs: ids}.Wrap(api)
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	workerCtx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()
	var workerDone <-chan error
	if cfg.WorkerEnabled {
		owner, err := ids.New("worker")
		if err != nil {
			return err
		}
		runner := &worker.Worker{Store: database, Tasks: services, Clock: clk, Owner: owner, Poll: cfg.WorkerPoll, Lease: cfg.WorkerLease, Log: log}
		done := make(chan error, 1)
		workerDone = done
		go func() { done <- runner.Run(workerCtx) }()
	}
	serverDone := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", cfg.HTTPAddr)
		serverDone <- server.ListenAndServe()
	}()
	select {
	case err := <-serverDone:
		cancelWorker()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case err := <-workerDone:
		if err != nil {
			return err
		}
	case <-ctx.Done():
	}
	checker.Stop()
	cancelWorker()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if workerDone == nil {
		return nil
	}
	select {
	case err := <-workerDone:
		return err
	case <-shutdownCtx.Done():
		return shutdownCtx.Err()
	}
}
