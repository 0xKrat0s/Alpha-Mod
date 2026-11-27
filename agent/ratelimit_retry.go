package agent

import (
	"context"
	"iter"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/Autumn-27/norma/llm"
)

// Rate-limit retry parity for the chat/conversation lane.
//
// The task path (server/task_llm.go) replays rate-limited same-provider calls
// with a 2s/4s/8s jittered ladder before failing over; the chat family (chat
// page, conversation agents, finding retests — everything through
// ChatAgent.Chat) had only the SDK's fast connect ladder (0.5s-8s, 3 tries),
// which a real rate-limit window outlasts: the 2026-11-27 burst logged 15
// conv:reporter failures at 0 input tokens in two minutes, then recovered.
// This provider wraps the chat provider chain with the same pacing.

const (
	// rateLimitChatRetries is the same-provider attempt cap for rate limits on
	// the chat lane. One more than the task lane's default: a chat turn has no
	// planner failover behind it, so waiting is the only recovery.
	rateLimitChatRetries = 3
)

// chatRateLimitBackoff is exponential (2s, 4s, 8s) capped at 30s, plus up
// to 25% jitter so concurrent agents backing off after the same 429 do not
// fire in lockstep. Variable so tests can pin it.
var chatRateLimitBackoff = func(attempt int) time.Duration {
	d := min(2*time.Second*(1<<attempt), 30*time.Second)
	return d + time.Duration(rand.Int64N(int64(d)/4)+1)
}

// isChatRateLimitError reports whether the provider rejected the call for
// pacing (HTTP 429 / rate_limit). Mirrors server.isRateLimitError; the
// strings are provider-transport phrasing, stable across both lanes.
func isChatRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "429") ||
		strings.Contains(s, "rate limit") ||
		strings.Contains(s, "rate_limit")
}

// RateLimitRetryProvider wraps a provider and replays rate-limited failures
// with a paced ladder. Only pre-commit failures are replayed: once any event
// reached the caller, replaying could duplicate delivered output, so the error
// surfaces untouched. Non-rate-limit errors are never retried here — the SDK's
// own connect ladder and the caller's failover own those.
type RateLimitRetryProvider struct {
	inner      llm.Provider
	maxRetries int
	backoff    func(attempt int) time.Duration
}

// NewRateLimitRetryProvider wraps p with chat-lane rate-limit retries.
func NewRateLimitRetryProvider(p llm.Provider) *RateLimitRetryProvider {
	return &RateLimitRetryProvider{inner: p, maxRetries: rateLimitChatRetries, backoff: chatRateLimitBackoff}
}

func (p *RateLimitRetryProvider) sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (p *RateLimitRetryProvider) Stream(ctx context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		for attempt := 0; ; attempt++ {
			committed := false
			var streamErr error
			for ev, err := range p.inner.Stream(ctx, req) {
				if err != nil {
					streamErr = err
					break
				}
				committed = true
				if !yield(ev, nil) {
					return
				}
			}
			if streamErr == nil || committed || ctx.Err() != nil ||
				attempt >= p.maxRetries || !isChatRateLimitError(streamErr) {
				if streamErr != nil {
					yield(llm.StreamEvent{}, streamErr)
				}
				return
			}
			if !p.sleep(ctx, p.backoff(attempt)) {
				yield(llm.StreamEvent{}, ctx.Err())
				return
			}
		}
	}
}

func (p *RateLimitRetryProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	for attempt := 0; ; attempt++ {
		msg, sr, usage, err := p.inner.Complete(ctx, req)
		// Complete fails as a whole — no partial output — so any rate-limit
		// failure can be replayed verbatim.
		if err == nil || ctx.Err() != nil || attempt >= p.maxRetries || !isChatRateLimitError(err) {
			return msg, sr, usage, err
		}
		if !p.sleep(ctx, p.backoff(attempt)) {
			return llm.Message{}, "", usage, ctx.Err()
		}
	}
}

// compile-time guard: the wrapper stays usable wherever a raw provider is.
var _ llm.Provider = (*RateLimitRetryProvider)(nil)
