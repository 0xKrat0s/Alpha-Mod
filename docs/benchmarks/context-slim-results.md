# opt/context-slim — measured results (2026-11-27)

Live A/B: the 2026-11-26 baseline tasks vs a full Juice Shop task run on the
optimized build (compaction 64K window + tool RAG on + slim worker overview +
429-aware retry). 230 LLM calls, 186 worker calls.

## Worker lane (the primary burn target)

| metric | baseline | measured | delta |
|---|---|---|---|
| avg input tokens | 32,780 | **25,011** | −24% |
| max input tokens | 75,497 | **33,613** | −56% |
| tool_result bytes/call | 49 KB | **6 KB** | −88% |
| user message bytes/call | 66 KB | **22 KB** | −67% |
| assistant bytes/call | 41 KB | **13 KB** | −68% |
| uncached input/call | ~18K first call | **1,923 avg** | 94% cache hit |

## Other verification

- **Attribution fix**: 0 NULL-meta rows since deploy (baseline: 323 per 23 h).
  Error rows now self-identify (`conv:reporter`, `worker`).
- **429 retry**: the task path ran clean; the one error burst (15 calls,
  4.4 s, 0 tokens) hit the *conversation* lane, which does not use the task
  retry policy — see backlog.

## Backlog (derived from this run)

1. **Conversation-lane retry parity**: ChatAgent/conversation runs bypass the
   task_llm retry policy; give them the same 429-aware ladder.
2. **Tool schema dieting**: tool RAG prunes little because the keep-set covers
   nearly the whole catalog — tools_bytes stayed ~41 KB. The heavy schemas
   (ingest_bbot, traffic_*, graph tools) are all always-kept; shortening
   descriptions / lazy-loading heavy tools is the next byte win (~10-15 KB).
3. **cache_write visibility** (Norma provider parser) — unchanged.
