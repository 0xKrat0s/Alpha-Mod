# ARTEX Fork — Two-Track Work Plan

Repo: `0xKrat0s/Alpha-Mod` (fork of `BaaRai/ARTEX`, upstream remote configured)
Base: `main` @ `3cb0638` (= upstream latest)

- **Track A — Optimisation** (engine internals: `server/`, `db/`, `traffic/`, `agent/`, `Dockerfile`)
- **Track B — Skills** (everything under `skills/`, plus small `web/` registration lines)

Shared file contract: Track A never edits `skills/`; Track B never edits Go in `server/`, `db/`, `traffic/`, `agent/`.
Only potentially shared file: `web/src/...` skill registry lists — **Track B owns it**; Track A must not touch `web/` skill registration (if a perf change needs it, coordinate in the PR).

---

## 0. One-time setup

### Friend (clone)
```bash
git clone https://github.com/0xKrat0s/Alpha-Mod.git
cd Alpha-Mod
git remote add upstream https://github.com/BaaRai/ARTEX
git fetch upstream --prune
git config user.name "HisName"
git config user.email "his@email"
```

### Owner (already done — verify)
```bash
cd ~/tmp/Forked-tmp/Alpha-Mod
git remote -v                # origin = Alpha-Mod, upstream = BaaRai/ARTEX
git switch main && git pull
```

### GitHub (owner, web UI)
- Settings → Collaborators → invite friend (Write access)
- Settings → Branches → Add rule for `main`:
  - ✅ Require a pull request before merging
  - ✅ Require 1 approval
  - (no force pushes, no deletions)

---

## 1. Ground rules (both)

1. **Never commit to `main` directly.** Branch → push → Pull Request → other person reviews → merge.
2. **One branch = one task.** Branch names below are pre-agreed so PRs don't collide.
3. **Sync your branch often:**
   ```bash
   git fetch origin && git merge origin/main
   ```
   Daily while the branch is active. Small merges > big conflicts.
4. **`go.mod` / `go.sum`:** only Track A bumps dependencies. Track B never edits them.
5. Commits: `opt:` / `skill:` / `fix:` / `security:` prefixes.

---

## 2. Track A — Optimisation (owner)

### A0. Baseline first — branch `opt/baseline-bench`

No optimising before numbers exist.

```bash
git switch main && git pull
git switch -c opt/baseline-bench
```

1. Check `docker-compose.bench.yml` and `docs/` for the bench harness usage:
   ```bash
   docker compose -f docker-compose.bench.yml up --build   # verify flags in README first
   ```
2. If no harness exists, build a minimal one:
   - Fixed lab target (e.g. a local vulnerable app: DVWA/Juice Shop container)
   - 1 fixed task run with fixed LLM profile → record: wall time, tokens per finding,
     tokens per intent, DB row counts, UI task-page load time
3. Save results to `docs/benchmarks/baseline-<date>.md`. Commit + PR.

**Deliverable:** a repeatable bench + a baseline doc. Every later opt PR must show before/after on this bench.

### A1. LLM context/cost — branch `opt/llm-context`
Highest-leverage area. Tokens are the engine's bottleneck.

- Measure context assembly: how much graph/traffic/asset context is packed per planner
  round and per worker turn; how much is actually used.
- Tool-result truncation: replace hard size caps with smarter cuts (keep head+tail, strip
  boilerplate, summarize repeat blocks).
- Compaction tuning: when it triggers, what it keeps/drops.
- Metric: **tokens per finding**, **tokens per completed intent** (query `llm_usage` table).
- Key files: `agent/` (planner, worker, compaction), `db/` llm usage records.

### A2. DB hot paths — branch `opt/db-hotpaths`
- Turn on Postgres query logging (`log_min_duration_statement = 100`) in the dev compose,
  run a bench task, collect the slow query list.
- Focus: coverage-graph reads, task list, `llm_usage` aggregates, asset-graph traversal
  (recursive queries), finding lists.
- Add indexes / rewrite N+1s / add `EXPLAIN ANALYZE` before/after in the PR description.
- Upstream already merged `perf/task-query-indexes` — don't duplicate; audit what's covered.

### A3. Traffic store — branch `opt/traffic-store`
- 338 MB of captures already. Add: compressed bodies at rest (zstd), lazy body loading in
  the UI, FTS index over request/response text.
- Key files: `traffic/`, `db/` traffic tables, `archivestore/`.

### A4. MITM proxy streaming — branch `opt/proxy-streaming`
- Audit buffering: bodies held fully in memory (replay allows 8 MiB). Bound memory,
  stream large bodies to disk/blob store, keep SSE/WebSocket pass-through intact.
- Key files: `traffic/traffic.go`, `intercept/`.

### A5. Image slimming — branch `opt/image-slim`
- `docker image ls artex:local` before/after. Merge apt layers, drop build-only residue,
  reorder Dockerfile layers for cache hits (build time matters too).
- Careful: pinned digests + checksum verification are deliberate (security) — keep them.

### Suggested order
A0 → A2 (easy wins, builds bench discipline) → A1 (biggest payoff) → A3 → A4 → A5.

---

## 3. Track B — Skills (friend)

Skill format: copy the shape of `skills/api-recon/` — `SKILL.md` (playbook the model
follows) + `scripts/` (deterministic helpers) + `reference.md` if needed.
The Docker image already ships: nmap, impacket, hydra, hashcat, john, sqlmap, nuclei,
httpx, subfinder, katana, ffuf, dalfox, bbot, Playwright, ripgrep, curl, netcat.

### B1. `skill/internal-recon` — branch `skill/internal-recon`
Post-foothold network/AD recon. **Biggest gap in the framework** — nothing covers this phase today.

- SKILL.md playbook: when to run it (after first access), scope checks first, noise discipline
- scripts: `nmap_deep.sh` (service-version sweeps with pacing), `smb_enum.sh` (enum4linux-ng /
  impacket lookupsid), `secretsdump_run.sh` (destructive — document as approval-required),
  cred-file harvester (unattend.xml, history files, configs)
- Every script must check scope before firing; destructive steps documented as
  approval-required in SKILL.md (the platform's intercept gate will ask — lean into it)

### B2. `skill/exploit-suggest` — branch `skill/exploit-suggest`
- Input: findings/assets from the graph (versions, techs, CVEs)
- scripts: match against nuclei template tags (baked at `/opt/nuclei-templates`),
  optional offline exploit-db feed; output ranked "next actions" the planner can dispatch
- This makes findings actionable — closes the loop for the engine

### B3. `skill/auth-attacks` — branch `skill/auth-attacks`
- JWT confusion (alg none / HS-RS swap, kid injection), session handling flaws, IDOR playbook
  (role matrix, object-id iteration), OAuth flow misuse
- scripts: `jwt_probe.py`, `idor_matrix.py` (paced, per noise profile)

### B4. `skill/cred-vault` — branch `skill/cred-vault`
- Organize looted creds: provenance (which host/finding), where they validated, reuse map
- Output: spray candidates with discipline rules (lockout-aware pacing)

### B5. `skill/cloud-metadata` — branch `skill/cloud-metadata`
- SSRF playbook: IMDSv1/v2, metadata endpoints, redirect chains, DNS rebinding probes
- scripts: `metadata_matrix.py` (safe probe set)

### B6. `skill/report-mapping` — branch `skill/report-mapping`
- Annotate findings: ATT&CK technique IDs, OWASP category, CVSS v3.1 vector + score
- scripts: `cvss_calc.py`, ATT&CK lookup table (vendored JSON)

### Suggested order
B1 → B2 (completeness pair) → B3 → B4 → B5 → B6.

Each PR includes: the skill, a `docs/` example run against a lab target, and (when relevant)
entries for the drostbench-style flag corpus so skill behavior is testable.

---

## 4. Daily routine (both)

```bash
# start of day
git switch main && git pull origin main && git fetch upstream
git log --oneline main..upstream/main        # did BaaRai move? if yes:
git merge upstream/main && git push origin main

# work on your branch
git switch <your-branch>
git merge origin/main                        # pull main into your branch (daily!)
# ...code, commit...
git push -u origin <your-branch>             # first push; afterwards: git push
```

## 5. PR flow (the "combine" step)

1. Push branch → GitHub → "Compare & pull request"
2. Title: `opt: ...` or `skill: ...`; description: what/why + bench numbers or example run
3. The OTHER person reviews and merges (never your own PR while the other is active)
4. Merge strategy: **merge commit** (default) — keep history; rebase only tiny fixes
5. Delete the branch after merge (button on GitHub); both run `git fetch --prune origin`

## 6. Merging BaaRai's upstream work mid-branch

```bash
git fetch upstream
git log --oneline HEAD..upstream/main        # what's new
git merge upstream/main                      # resolve conflicts in YOUR branch, then:
git push
```
Conflict rules: upstream wins on files you didn't touch; your change wins on your files;
both-touched = talk it through, don't guess.

## 7. Definition of done (every task)

- [ ] Bench numbers or example run attached to the PR
- [ ] No edits outside your track's directories
- [ ] `go build ./...` + `go vet ./...` clean (Track A) / scripts pass `shellcheck` + dry-run (Track B)
- [ ] Other person reviewed + merged
- [ ] Branch deleted, both sides pruned
