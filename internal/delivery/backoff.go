package delivery

import (
	"math/rand/v2"
	"time"
)

// Backoff computes the delay before the next attempt using exponential
// backoff with "full jitter": a random duration in [0, min(max, base*2^(n-1))].
// Jitter spreads retries out so a recovering endpoint is not hit by a
// thundering herd.
type Backoff struct {
	Base time.Duration
	Max  time.Duration
	// Rand returns a float in [0,1). Overridable for deterministic tests.
	Rand func() float64
}

// Next returns the delay after the given attempt number (1-based).
func (b Backoff) Next(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	ceiling := b.Base
	for i := 1; i < attempt && ceiling < b.Max; i++ {
		ceiling *= 2
	}
	if ceiling > b.Max {
		ceiling = b.Max
	}
	r := b.Rand
	if r == nil {
		r = rand.Float64
	}
	return time.Duration(r() * float64(ceiling))
}
