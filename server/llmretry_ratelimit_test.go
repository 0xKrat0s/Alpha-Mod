package server

import (
	"errors"
	"testing"
	"time"
)

// A rate-limit rejection (HTTP 429 / rate_limit in any provider phrasing) is
// detected wherever it appears in the wrapped transport error.
func TestIsRateLimitError(t *testing.T) {
	yes := []string{
		`anthropic: status 429: {"error":{"type":"rate_limit_error","code":"1302"}}`,
		"openai: 429 Too Many Requests",
		"provider said: Rate Limit exceeded, slow down",
		"relay error: RATE_LIMIT hit",
	}
	for _, s := range yes {
		if !isRateLimitError(errors.New(s)) {
			t.Fatalf("isRateLimitError(%q) = false, want true", s)
		}
	}
	no := []string{
		"transport: connection reset by peer",
		"status 500: internal",
		"status 400: bad request",
		"context deadline exceeded",
		`status 402: {"error":{"type":"quota_exhausted"}}`, // quota, not pacing — handled by failover
	}
	if isRateLimitError(nil) {
		t.Fatal("isRateLimitError(nil) = true, want false")
	}
	for _, s := range no {
		if isRateLimitError(errOrNil(s)) {
			t.Fatalf("isRateLimitError(%q) = true, want false", s)
		}
	}
}

func errOrNil(s string) error {
	if s == "" {
		return nil
	}
	return errors.New(s)
}

// Rate limits extend the default same-provider budget (2 → 3 attempts); an
// operator-configured count is respected exactly.
func TestRetryAttemptsFor(t *testing.T) {
	rl := errors.New("anthropic: status 429 rate_limit_error")
	other := errors.New("connection reset by peer")

	if got := retryAttemptsFor(sameProviderStreamRetries, rl); got != rateLimitStreamRetries {
		t.Fatalf("default policy + rate limit: attempts=%d, want %d", got, rateLimitStreamRetries)
	}
	if got := retryAttemptsFor(sameProviderStreamRetries, other); got != sameProviderStreamRetries {
		t.Fatalf("default policy + transport error: attempts=%d, want %d", got, sameProviderStreamRetries)
	}
	if got := retryAttemptsFor(5, rl); got != 5 {
		t.Fatalf("configured 5 + rate limit: attempts=%d, want 5 (operator config wins)", got)
	}
	if got := retryAttemptsFor(-1, rl); got != -1 {
		t.Fatalf("disabled layer (-1) + rate limit: attempts=%d, want -1 (off stays off)", got)
	}
}

// The rate-limit ladder backs off exponentially with a 30s cap; the default
// ladder stays untouched for non-rate-limit failures.
func TestBackoffFor(t *testing.T) {
	rl := errors.New("status 429 rate limit")
	other := errors.New("eof")

	for attempt, wantMin := range []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second} {
		got := backoffFor(rl, attempt, sameProviderRetryBackoff)
		if got < wantMin || got > wantMin+wantMin/4 {
			t.Fatalf("rate limit backoff(attempt %d) = %v, want [%v, %v]", attempt, got, wantMin, wantMin+wantMin/4)
		}
	}
	if got := backoffFor(rl, 10, sameProviderRetryBackoff); got > 30*time.Second+30*time.Second/4 {
		t.Fatalf("rate limit backoff cap: %v exceeds 30s ladder + jitter", got)
	}
	if got := backoffFor(other, 0, sameProviderRetryBackoff); got != 500*time.Millisecond {
		t.Fatalf("non-rate-limit backoff(0) = %v, want the 500ms default ladder", got)
	}
}
