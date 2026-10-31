// Package config loads runtime settings from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr    string
	DatabaseURL string
	RedisURL    string
	LogLevel    string

	WorkerConcurrency int
	DeliveryTimeout   time.Duration
	MaxAttempts       int
	BackoffBase       time.Duration
	BackoffMax        time.Duration
	LeaseDuration     time.Duration
	PollInterval      time.Duration
}

func Load() (Config, error) {
	c := Config{
		HTTPAddr:    getenv("HTTP_ADDR", ":8080"),
		DatabaseURL: getenv("DATABASE_URL", "postgres://webhooks:webhooks@localhost:5433/webhooks?sslmode=disable"),
		RedisURL:    getenv("REDIS_URL", "redis://localhost:6379/0"),
		LogLevel:    getenv("LOG_LEVEL", "info"),
	}

	var err error
	if c.WorkerConcurrency, err = getint("WORKER_CONCURRENCY", 10); err != nil {
		return c, err
	}
	if c.MaxAttempts, err = getint("MAX_ATTEMPTS", 8); err != nil {
		return c, err
	}
	if c.DeliveryTimeout, err = getdur("DELIVERY_TIMEOUT", 10*time.Second); err != nil {
		return c, err
	}
	if c.BackoffBase, err = getdur("BACKOFF_BASE", 5*time.Second); err != nil {
		return c, err
	}
	if c.BackoffMax, err = getdur("BACKOFF_MAX", 1*time.Hour); err != nil {
		return c, err
	}
	if c.LeaseDuration, err = getdur("LEASE_DURATION", 60*time.Second); err != nil {
		return c, err
	}
	if c.PollInterval, err = getdur("POLL_INTERVAL", 500*time.Millisecond); err != nil {
		return c, err
	}
	return c, nil
}

func getenv(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return def
}

func getint(k string, def int) (int, error) {
	v := os.Getenv(k)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", k, err)
	}
	return n, nil
}

func getdur(k string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(k)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", k, err)
	}
	return d, nil
}
