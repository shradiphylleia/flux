package delivery

import (
	"testing"
	"time"
)

func TestBackoffCeilingGrowsAndCaps(t *testing.T) {
	b := Backoff{Base: time.Second, Max: 30 * time.Second, Rand: func() float64 { return 1 }}

	want := []time.Duration{1, 2, 4, 8, 16, 30, 30}
	for i, w := range want {
		if got := b.Next(i + 1); got != w*time.Second {
			t.Errorf("attempt %d: got %v want %v", i+1, got, w*time.Second)
		}
	}
}

func TestBackoffJitterWithinBounds(t *testing.T) {
	b := Backoff{Base: time.Second, Max: time.Minute}
	for i := 0; i < 1000; i++ {
		if d := b.Next(4); d < 0 || d > 8*time.Second {
			t.Fatalf("delay %v out of [0, 8s]", d)
		}
	}
}
