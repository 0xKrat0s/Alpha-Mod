# opt/context-slim — phase 1: compaction engagement (operator setting)

## Problem (from the 2026-11-26 audit)

Worker lane averaged 32,780 input tokens/call (max 75K); late worker runs grew
to ~52K with no compaction. Root cause: the Z.AI profile had
`context_window_k = 0`, so ARTEX sized compaction for the 200K default.
Norma's thresholds (compaction/compaction.go):

- micro-compact (clear old compactable tool results) at 60% of effective window
- auto-compact (summarize) at effective window − 13K buffer
- effective window = ContextWindow − min(maxOutput, 20K) reserved

At 200K: micro at 108K, auto at 167K — never reached at observed 52K peaks.

## Change applied (live instance, 2026-11-26)

`UPDATE llm_profiles SET context_window_k = 64 WHERE id = 1` (Z.AI / glm-5.3-flash).

Effective window 44K → micro-compact at 26.4K, auto-compact at 31K.
Fresh worker turns start ~18K, so the summarizer does not run on turn one;
mid-run growth past ~26K now clears old tool results first (non-destructive)
and summarizes past ~31K. Expected: worker avg input 32.8K → low-to-mid 20s,
late-run 52K peaks capped around ~35K.

## How to measure (same task shape as baseline)

After one comparable task run:

```sql
SELECT worker, count(*) calls, round(avg(input_tokens)) avg_in,
       max(input_tokens) max_in,
       round(avg(input_tokens - cache_read)) avg_uncached
FROM llm_usage WHERE ts > now() - interval '1 day' AND worker = 'worker'
GROUP BY worker;
```

Compare against baseline (audit table): avg 32,780 / max 75,497.
Also watch: tokens per finding (quality must not degrade), task wall time
(compaction adds summarizer calls), and `llm_context.tool_result_bytes`
slope across message buckets (should flatten).

## Tuning

- Peaks still > 35K and quality fine → drop to 56 (micro at 21.6K, auto at 23K).
- Summarizer firing too often / losing context → raise to 80 (micro at 36K,
  auto at 47K).

## Phase 2 backlog (code, not config)

1. Intent-scoped tool subsets: 38 tool schemas = 42 KB on every call
   (~20% of prompt). Workers should carry the tools their intent needs.
2. `graph_overview` prefetch trim: first user message is 24 KB; fresh worker
   first call is the largest uncached spend.
3. Cache-write visibility (Norma provider parser) — verify whether the relay
   reports cache writes and whether the parser drops them.
