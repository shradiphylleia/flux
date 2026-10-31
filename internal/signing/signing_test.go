package signing

import (
	"testing"
	"time"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	body := []byte(`{"event_id":"evt_001"}`)
	h := Sign("whsec_test", now, body)

	if err := Verify("whsec_test", h, body, now, 5*time.Minute); err != nil {
		t.Fatalf("expected valid signature, got %v", err)
	}
}

func TestVerifyRejects(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	body := []byte(`{"a":1}`)
	h := Sign("whsec_test", now, body)

	cases := map[string]struct {
		secret, header string
		body           []byte
		at             time.Time
	}{
		"wrong secret":  {"whsec_other", h, body, now},
		"tampered body": {"whsec_test", h, []byte(`{"a":2}`), now},
		"stale":         {"whsec_test", h, body, now.Add(10 * time.Minute)},
		"malformed":     {"whsec_test", "garbage", body, now},
		"missing v1":    {"whsec_test", "t=1800000000", body, now},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Verify(c.secret, c.header, c.body, c.at, 5*time.Minute); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
