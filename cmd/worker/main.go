// Command worker consumes due deliveries and POSTs them to subscribers.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/shradiphylleia/rival/internal/config"
	"github.com/shradiphylleia/rival/internal/db"
	"github.com/shradiphylleia/rival/internal/delivery"
	"github.com/shradiphylleia/rival/internal/logging"
	"github.com/shradiphylleia/rival/internal/queue"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log := logging.New(cfg.LogLevel, "worker")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("connect postgres", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	rdb, err := queue.Connect(ctx, cfg.RedisURL)
	if err != nil {
		log.Error("connect redis", "err", err)
		os.Exit(1)
	}
	defer rdb.Close()

	log.Info("worker started", "concurrency", cfg.WorkerConcurrency)
	if err := delivery.NewWorker(cfg, pool, queue.New(rdb), log).Run(ctx); err != nil {
		log.Error("worker", "err", err)
		os.Exit(1)
	}
}
