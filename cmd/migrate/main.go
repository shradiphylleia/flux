// Command migrate applies pending SQL migrations and exits.
package main

import (
	"context"
	"os"

	"github.com/shradiphylleia/rival/internal/config"
	"github.com/shradiphylleia/rival/internal/db"
	"github.com/shradiphylleia/rival/internal/logging"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log := logging.New(cfg.LogLevel, "migrate")
	ctx := context.Background()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("connect postgres", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool, log); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}
	log.Info("migrations up to date")
}
