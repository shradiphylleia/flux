package api

import "net/http"

// Handler bodies are intentionally stubbed in the scaffold; each TODO
// describes the intended behaviour.

// createWebhook — POST /webhooks
//
// TODO: decode {url, events}; validate url is absolute http(s) and events is a
// non-empty list of non-empty strings; generate secret via signing.NewSecret;
// insert; return 201 with the secret (only time it is shown).
// Errors: 400 malformed JSON, 422 validation_failed with per-field messages.
func (s *Server) createWebhook(w http.ResponseWriter, r *http.Request) {
	notImplemented(w)
}

// listWebhooks — GET /webhooks
func (s *Server) listWebhooks(w http.ResponseWriter, r *http.Request) {
	notImplemented(w)
}

// getWebhook — GET /webhooks/{id}  (404 if unknown)
func (s *Server) getWebhook(w http.ResponseWriter, r *http.Request) {
	notImplemented(w)
}

// updateWebhook — PATCH /webhooks/{id}
//
// TODO: partial update of {enabled, url, events}. Disabling stops new
// fan-out; in-flight deliveries are checked by the worker before sending.
func (s *Server) updateWebhook(w http.ResponseWriter, r *http.Request) {
	notImplemented(w)
}

// publishEvent — POST /events
//
// TODO: validate {event_id, type, timestamp (RFC3339), data (JSON object)}.
// In ONE transaction:
//   - INSERT event ON CONFLICT (id) DO NOTHING
//     -> conflict with identical body: 200 (idempotent replay)
//     -> conflict with different body: 409
//   - INSERT INTO deliveries (...) SELECT ... FROM webhooks
//     WHERE enabled AND event_types @> ARRAY[$type]
//
// After commit, schedule each delivery ID in Redis (best effort; the
// reconciler covers failures). Return 202 Accepted with the delivery count.
func (s *Server) publishEvent(w http.ResponseWriter, r *http.Request) {
	notImplemented(w)
}

// getEventDeliveries — GET /deliveries/{eventID}
//
// TODO: return the event's aggregate status plus one entry per webhook
// (status, attempts, last_error, next_retry_at, timestamps). 404 if the event
// is unknown.
func (s *Server) getEventDeliveries(w http.ResponseWriter, r *http.Request) {
	notImplemented(w)
}

func notImplemented(w http.ResponseWriter) {
	writeError(w, http.StatusNotImplemented, "not_implemented", "endpoint not implemented yet")
}
