// Package delivery runs the asynchronous delivery pipeline: claim due
// deliveries, POST them to the subscriber, record the attempt, and schedule a
// retry or mark the delivery terminal.
package delivery

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shradiphylleia/rival/internal/config"
	"github.com/shradiphylleia/rival/internal/queue"
)

type Worker struct {
	cfg     config.Config
	db      *pgxpool.Pool
	q       *queue.Queue
	log     *slog.Logger
	client  *http.Client
	backoff Backoff
}

func NewWorker(cfg config.Config, db *pgxpool.Pool, q *queue.Queue, log *slog.Logger) *Worker {
	return &Worker{
		cfg: cfg,
		db:  db,
		q:   q,
		log: log,
		client: &http.Client{
			Timeout: cfg.DeliveryTimeout,
			// Never follow redirects: a 3xx is treated as a failed delivery.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		backoff: Backoff{Base: cfg.BackoffBase, Max: cfg.BackoffMax},
	}
}

// Run polls for due deliveries and fans them out to a bounded pool of
// goroutines until ctx is cancelled. Each delivery is handled independently,
// so a slow or failing endpoint only occupies one slot for at most
// DeliveryTimeout.
func (w *Worker) Run(ctx context.Context) error {
	jobs := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < w.cfg.WorkerConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				w.process(ctx, id)
			}
		}()
	}

	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()
	defer func() { close(jobs); wg.Wait() }()

	// TODO: run a reconciler on a slower ticker that re-schedules deliveries
	// from Postgres whose next_attempt_at <= now() and lease has expired
	// (covers IDs lost from Redis).

	for {
		select {
		case <-ctx.Done():
			w.log.Info("worker shutting down")
			return nil
		case <-ticker.C:
			ids, err := w.q.PopDue(ctx, time.Now(), w.cfg.WorkerConcurrency)
			if err != nil {
				w.log.Error("pop due deliveries", "err", err)
				continue
			}
			for _, id := range ids {
				select {
				case jobs <- id:
				case <-ctx.Done():
					return nil
				}
			}
		}
	}
}

// process performs one delivery attempt.
//
// TODO: implement
//  1. Claim a lease: UPDATE deliveries SET status='in_progress',
//     locked_until=now()+lease WHERE id=$1 AND status IN ('pending','in_progress')
//     AND (locked_until IS NULL OR locked_until < now()) RETURNING ...
//     Zero rows => another worker owns it or it is terminal; skip.
//  2. Load event + webhook; skip (mark failed) if the webhook was disabled.
//  3. Build body, sign with signing.Sign, POST with headers
//     Webhook-Id (delivery id), Webhook-Event-Id, Webhook-Signature.
//  4. Insert delivery_attempts row; 2xx => succeeded, otherwise
//     attempt_count++ and either schedule retry (w.backoff.Next) or mark failed.
//  5. On retry, w.q.Schedule(id, next_attempt_at).
func (w *Worker) process(ctx context.Context, deliveryID string) {
	w.log.Debug("process delivery (not implemented)", "delivery_id", deliveryID)
}
