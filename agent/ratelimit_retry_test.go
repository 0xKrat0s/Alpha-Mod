package agent

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/Autumn-27/norma/llm"
)

// countingProv replays a scripted error list before succeeding, and counts
// calls. It emits one text event (committing the stream) on success.
type countingProv struct {
	streamErrs []error
	compErrs   []error
	calls      int
}

func (p *countingProv) Stream(ctx context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		p.calls++
		if p.calls <= len(p.streamErrs) {
			yield(llm.StreamEvent{}, p.streamErrs[p.calls-1])
			return
		}
		yield(llm.StreamEvent{Type: llm.SETextDelta, Text: "ok"}, nil)
	}
}

func (p *countingProv) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	p.calls++
	if p.calls <= len(p.compErrs) {
		return llm.Message{}, "", llm.Usage{}, p.compErrs[p.calls-1]
	}
	return llm.Message{}, "stop", llm.Usage{InputTokens: 10}, nil
}

func pinned() *RateLimitRetryProvider {
	p := NewRateLimitRetryProvider(nil)
	p.backoff = func(int) time.Duration { return 0 }
	return p
}

var rlErr = errors.New("anthropic: status 429 rate_limit_error")
var transportErr = errors.New("connection reset by peer")

// A rate-limited pre-commit stream is replayed on the paced ladder until it
// succeeds; the caller never sees the transient errors.
func TestChatRateLimitStreamRetriesThenSucceeds(t *testing.T) {
	inner := &countingProv{streamErrs: []error{rlErr, rlErr}}
	p := pinned()
	p.inner = inner
	var events []string
	var gotErr error
	for ev, err := range p.Stream(context.Background(), llm.CompletionRequest{}) {
		if err != nil {
			gotErr = err
			continue
		}
		events = append(events, ev.Text)
	}
	if gotErr != nil {
		t.Fatalf("unexpected error: %v", gotErr)
	}
	if inner.calls != 3 || len(events) != 1 || events[0] != "ok" {
		t.Fatalf("calls=%d events=%v, want 3 attempts and one ok event", inner.calls, events)
	}
}

// The attempt cap is real: a persistently rate-limited stream surfaces the
// error after maxRetries+1 attempts.
func TestChatRateLimitStreamCapsAttempts(t *testing.T) {
	inner := &countingProv{streamErrs: []error{rlErr, rlErr, rlErr, rlErr, rlErr}}
	p := pinned()
	p.inner = inner
	var gotErr error
	for _, err := range p.Stream(context.Background(), llm.CompletionRequest{}) {
		if err != nil {
			gotErr = err
		}
	}
	if gotErr == nil || !isChatRateLimitError(gotErr) {
		t.Fatalf("err=%v, want the rate-limit error", gotErr)
	}
	if inner.calls != rateLimitChatRetries+1 {
		t.Fatalf("calls=%d, want %d (1 initial + %d retries)", inner.calls, rateLimitChatRetries+1, rateLimitChatRetries)
	}
}

// A non-rate-limit failure is surfaced untouched: the SDK connect ladder and
// the caller own those.
func TestChatNonRateLimitNotRetried(t *testing.T) {
	inner := &countingProv{streamErrs: []error{transportErr, transportErr}}
	p := pinned()
	p.inner = inner
	var gotErr error
	for _, err := range p.Stream(context.Background(), llm.CompletionRequest{}) {
		if err != nil {
			gotErr = err
		}
	}
	if gotErr != transportErr || inner.calls != 1 {
		t.Fatalf("err=%v calls=%d, want the raw error after exactly 1 call", gotErr, inner.calls)
	}
}

// Once any event reached the caller the stream is committed: a later rate
// limit must surface, not replay (replaying would duplicate output).
func TestChatCommittedStreamNeverReplayed(t *testing.T) {
	inner := &countingProv{}
	committedProv := &committedThenRateLimit{inner: inner}
	p := pinned()
	p.inner = committedProv
	var sawText bool
	var gotErr error
	for ev, err := range p.Stream(context.Background(), llm.CompletionRequest{}) {
		if ev.Text == "half" {
			sawText = true
		}
		if err != nil {
			gotErr = err
		}
	}
	if !sawText || gotErr != rlErr || inner.calls != 1 {
		t.Fatalf("sawText=%v err=%v calls=%d, want committed error surfaced after 1 call", sawText, gotErr, inner.calls)
	}
}

// committedThenRateLimit emits one committing text event, then a rate-limit
// error on the same attempt.
type committedThenRateLimit struct {
	inner *countingProv
}

func (c *committedThenRateLimit) Stream(ctx context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		c.inner.calls++
		yield(llm.StreamEvent{Type: llm.SETextDelta, Text: "half"}, nil)
		yield(llm.StreamEvent{}, rlErr)
	}
}

func (c *committedThenRateLimit) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	return c.inner.Complete(ctx, req)
}

// Complete fails as a whole, so rate limits are replayed there too.
func TestChatRateLimitCompleteRetries(t *testing.T) {
	inner := &countingProv{compErrs: []error{rlErr}}
	p := pinned()
	p.inner = inner
	msg, _, usage, err := p.Complete(context.Background(), llm.CompletionRequest{})
	if err != nil || usage.InputTokens != 10 || inner.calls != 2 {
		t.Fatalf("err=%v usage=%+v calls=%d, want success on attempt 2", err, usage, inner.calls)
	}
	_ = msg
}

func TestChatIsRateLimitError(t *testing.T) {
	if !isChatRateLimitError(errors.New("429 too many")) || !isChatRateLimitError(errors.New("Rate LIMIT")) {
		t.Fatal("rate-limit phrasings must match")
	}
	if isChatRateLimitError(nil) || isChatRateLimitError(errors.New("500 boom")) {
		t.Fatal("non-pacing errors must not match")
	}
}
