// Package domain holds the core types shared across the API, store, and worker.
package domain

import (
	"encoding/json"
	"time"
)

type Webhook struct {
	ID         string    `json:"id"`
	URL        string    `json:"url"`
	Secret     string    `json:"secret,omitempty"` // only exposed on create
	EventTypes []string  `json:"events"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Event is what producers publish. Payload is kept as raw JSON and delivered
// verbatim.
//
// TODO(design): design.md mentions mapping the payload onto an abstract
// interface with runtime variables resolved at delivery time (e.g. per-webhook
// templating). That would live in a payload-rendering step in the worker;
// out of scope for the core flow.
type Event struct {
	ID         string          `json:"event_id"`
	Type       string          `json:"type"`
	OccurredAt time.Time       `json:"timestamp"`
	Payload    json.RawMessage `json:"data"`
	ReceivedAt time.Time       `json:"received_at"`
}

type DeliveryStatus string

const (
	StatusPending    DeliveryStatus = "pending"
	StatusInProgress DeliveryStatus = "in_progress"
	StatusSucceeded  DeliveryStatus = "succeeded"
	StatusFailed     DeliveryStatus = "failed" // terminal: max attempts exhausted
)

type Delivery struct {
	ID             string         `json:"id"`
	EventID        string         `json:"event_id"`
	WebhookID      string         `json:"webhook_id"`
	Status         DeliveryStatus `json:"status"`
	AttemptCount   int            `json:"attempts"`
	MaxAttempts    int            `json:"max_attempts"`
	NextAttemptAt  *time.Time     `json:"next_retry_at"`
	LastError      *string        `json:"last_error"`
	LastStatusCode *int           `json:"last_status_code"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	CompletedAt    *time.Time     `json:"completed_at"`
}

type DeliveryAttempt struct {
	AttemptNumber int       `json:"attempt"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
	StatusCode    *int      `json:"status_code"`
	Error         *string   `json:"error"`
}
