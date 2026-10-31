// Package signing implements HMAC-SHA256 request signing for outgoing webhooks.
//
// Header format (Stripe-style):
//
//	Webhook-Signature: t=<unix-seconds>,v1=<hex(hmac_sha256(secret, "<t>.<body>"))>
//
// Including the timestamp in the signed content lets receivers reject replays
// older than a tolerance window.
package signing

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const Header = "Webhook-Signature"

// NewSecret returns a random secret suitable for a webhook subscription.
func NewSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "whsec_" + hex.EncodeToString(b), nil
}

// Sign returns the header value for body signed at time t.
func Sign(secret string, t time.Time, body []byte) string {
	ts := strconv.FormatInt(t.Unix(), 10)
	return fmt.Sprintf("t=%s,v1=%s", ts, mac(secret, ts, body))
}

// Verify checks a header value against body. Receivers would run this; it is
// included here for tests and as reference for customers.
func Verify(secret, header string, body []byte, now time.Time, tolerance time.Duration) error {
	var ts, sig string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			ts = v
		case "v1":
			sig = v
		}
	}
	if ts == "" || sig == "" {
		return errors.New("malformed signature header")
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errors.New("malformed timestamp")
	}
	if d := now.Sub(time.Unix(sec, 0)); d > tolerance || d < -tolerance {
		return errors.New("timestamp outside tolerance")
	}
	if !hmac.Equal([]byte(sig), []byte(mac(secret, ts, body))) {
		return errors.New("signature mismatch")
	}
	return nil
}

func mac(secret, ts string, body []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(ts))
	h.Write([]byte("."))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}
