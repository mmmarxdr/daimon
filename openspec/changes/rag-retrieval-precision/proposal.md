# Proposal: RAG Retrieval Precision (Neighbor Expansion + Score Thresholds)

## 1. Why

Users report that the agent "hallucinates the rest" of retrieved fragments when citing knowledge-base content. The root cause is structural, not model-side: `SearchChunks` (internal/rag/sqlite_store.go:126-240) returns isolated top-K chunks with no notion of continuity. When chunk 5 of a document is the best BM25 match but the answer actually continues into chunk 6, the LLM sees a half-sentence and invents the tail.

A second, related, failure mode: there is **no score floor**. FTS5 BM25 always returns up to `ftsLimit` (50) and the top-K slice is taken unconditionally, so weak matches (including near-zero-relevance chunks that happened to share a single term with the query) get promoted into the prompt and pollute the context window — displacing stronger signal and giving the LLM plausible-looking noise to confabulate against.

Both problems live in the retrieval layer, not in the chunker or the context builder. Fixing them there is surgical: no migration, no re-index, no provider changes, no UI flow changes. The user's own Phase-D plan already owns chunk-size changes + re-embedding; this change intentionally stays upstream of that work.

## 2. Scope

### In Scope

- **Neighbor expansion** — after `SearchChunks` selects its top-K by score, fetch neighboring chunks (`idx ∈ [hit.idx - N, hit.idx + N]`) from the same `doc_id` and stitch them into the returned `[]SearchResult`. Default `N = 1`. De-duplicate: if chunk X appears both as a top-K hit and as a neighbor of chunk Y, it is returned once. Neighbors inherit a score flag (not re-ranked) so downstream context code can render them as continuations of the primary hit.
- **Score thresholds** — two new optional filters applied before neighbor expansion:
  - `MaxBM25Score` — upper bound on FTS5 `bm25()` score (FTS5 returns lower/more-negative = better; "max" means "reject if bm25 > threshold"). Zero means "no threshold".
  - `MinCosineScore` — minimum cosine similarity in [0,1]. Applied ONLY on the cosine-rerank path (when `queryVec` provided and ≥2 candidates have embeddings). Zero means "no threshold".
- **Config knobs** — three new fields under `internal/config/config.go:RAGConfig`, following the `rag.embedding` sub-struct pattern already shipped in v0.7.0:
  - `rag.retrieval.neighbor_radius` — int, default `0` (opt-in; set to 1 or higher to enable neighbor expansion)
  - `rag.retrieval.max_bm25_score` — float64, default `0` (disabled)
  - `rag.retrieval.min_cosine_score` — float64, default `0` (disabled)

  > **BM25 vs cosine orientation note** — these two knobs have intentionally asymmetric prefixes. SQLite FTS5 `bm25()` returns lower (more-negative) scores for better matches, so the threshold is a ceiling: reject a candidate if `bm25() > max_bm25_score`. Cosine similarity is "higher is better", so its threshold is a floor: reject if `cosine < min_cosine_score`. Setting either to `0` disables that threshold.
- **Runtime config plumbing** — `internal/rag/config.go:RAGConfig` mirrors the new fields so the rag package is self-sufficient. `ApplyRAGDefaults` keeps neighbor radius at `0` (thresholds stay at zero = off).
- **PUT /api/config allow-list** — `internal/web/handler_config.go:42` `patchRAG` MUST allow the new `retrieval` sub-tree. This is a regression risk explicitly flagged in prior work (the patchRAG allow-list silently drops unknown fields, so the toast says "saved" while the field never reaches disk).
- **Wiring into SearchChunks** — introduce a `SearchOptions` struct and update the `SearchChunks` signature:

  ```go
  type SearchOptions struct {
      Limit          int
      NeighborRadius int
      MaxBM25Score   float64  // zero = disabled
      MinCosineScore float64  // zero = disabled
  }

  func (s *Store) SearchChunks(ctx context.Context, query string, queryVec []float32, opts SearchOptions) ([]Chunk, error)
  ```

  Rationale: idiomatic Go once param count exceeds 3–4. Existing callers pass `SearchOptions{Limit: N}`; backward compat at the test-suite level. Future additions (reranker, HyDE) don't re-break the signature. Call sites to update: `internal/agent/loop.go:134`, `internal/agent/context.go`, and stubs in `internal/agent/context_test.go:289`.
- **Tests (strict TDD)** — table-driven tests covering every new code path, written BEFORE implementation per project Strict TDD Mode.

### Out of Scope (explicit)

These are deferred to future changes and MUST NOT be touched here:

- **Chunk size bump (512 → 1024/1536)** — deferred to a "Phase D" change that ships with a UI re-index button and per-chunk embedding-model tracking. `ChunkOptions` defaults (internal/rag/doc.go:59-63) stay at 512/64.
- **LLM reranker pass** — deferred. Adds latency + cost; evaluate AFTER this fix lands and we have telemetry.
- **HyDE** (hypothetical document embeddings) — deferred.
- **Token-based chunking** (vs rune-based via `snapBoundary`) — deferred.
- **External vector DB migration** (pgvector, qdrant, etc.) — out of scope; SQLite + cosine stays.
- **Per-chunk embedding-model tracking** — needed for re-index workflows; owned by Phase D.
- **UI changes** — Settings UI exposure of the new knobs is a follow-up. This change only guarantees the API accepts them; default values make the feature a no-op until a user opts in.

## 3. Capabilities

### Modified Capabilities

- `rag-retrieval` (implicit in `internal/rag`) — gains neighbor expansion and score-floor filtering. The `SearchChunks` contract changes from "top-K by score" to "top-K by score (filtered by thresholds), expanded with N-neighbor continuation".
- `config` — adds the `rag.retrieval` sub-tree.
- `web-api-config` — `PUT /api/config` accepts the new sub-tree via `patchRAG.Retrieval`.

### New Capabilities

None. This change is purely a precision upgrade on an existing pipeline.

## 4. Approach

### Architecture Decisions

1. **Neighbor expansion happens in the store, not in the agent loop.** The store already holds the DB handle and the chunk indexing; expanding neighbors there is one `SELECT ... WHERE doc_id = ? AND idx BETWEEN ? AND ?` per top-K hit (or a single batched `IN` query). Doing it in the agent loop would require a second round-trip across a store interface we don't need. The ordering in the returned slice reflects document order within each cluster, with primary hits marking cluster centers.

2. **Thresholds filter BEFORE neighbor expansion.** If a top-K hit is below threshold, we reject the hit AND its would-be neighbors — we don't want to leak a below-threshold chunk into the prompt just because it's adjacent to another below-threshold chunk. Only accepted hits grow neighbor halos.

3. **Neighbors are stored alongside primary hits in the same `[]SearchResult` slice.** No new result type, no flag field (yet) — callers treat expanded results as a single list. Rationale: the context builder (`internal/agent/context.go:buildRAGSection`) already renders `### docTitle (chunk N)` per result; adjacent chunk indices under the same doc title will naturally read as continuation in the LLM's prompt. If future work needs "cluster-level" rendering we add a `ClusterID` field then, not now.

4. **Default N=0, thresholds default OFF.** Conservative upgrade path: every existing deployment sees identical behavior on first upgrade. Neighbor expansion is opt-in — users enable it explicitly (e.g. `neighbor_radius: 1`) when they observe the continuity problem. This avoids a silent prompt-size increase for deployments that haven't opted in. Thresholds remain 0 (disabled) for the same reason.

5. **Rune-based neighbor lookup uses existing `idx` column.** Chunks are already indexed monotonically per document (`chunker.go:18-129` writes sequential indices). Adjacency is cheap and exact — no heuristics, no position-in-byte math.

6. **De-dup is by chunk `id`, not by content.** Two top-K hits 3 chunks apart, each with radius 1, produce halos `[c2,c3,c4]` and `[c6,c7,c8]` — no overlap, no dedup needed. Two hits 2 chunks apart produce `[c2,c3,c4]` and `[c4,c5,c6]` — c4 appears twice and is emitted once (de-dup'd, kept in the primary's cluster).

7. **Score field for neighbors.** Since neighbors don't have their own query-relative score, the `SearchResult.Score` for a neighbor is set to the primary hit's score (so sorting stays stable). If a chunk IS itself a top-K hit AND a neighbor of another, the hit's own score wins.

### Retrieval Flow (end-to-end)

```
agent/loop.go:130-145
    reads SearchOptions from cfg.RAG.Retrieval (Limit, NeighborRadius, MaxBM25Score, MinCosineScore)
    passes to ragStore.SearchChunks(ctx, query, queryVec, opts SearchOptions)

rag/sqlite_store.go:SearchChunks
    1. FTS5 query → candidates (unchanged)
    2. Optional cosine rerank when queryVec + embeddings (unchanged)
    3. NEW: drop candidates above MaxBM25Score / below MinCosineScore
    4. NEW: take top-K surviving candidates as "primary hits"
    5. NEW: if NeighborRadius > 0, expand each primary with adjacent chunks
       (single SELECT per doc_id using idx BETWEEN primary.idx-N AND primary.idx+N,
        excluding chunk IDs already in the result set)
    6. NEW: merge primaries + neighbors, preserve per-cluster order (idx ASC),
       return the combined []SearchResult

agent/context.go:buildRAGSection
    unchanged — renders every SearchResult as "### docTitle (chunk N)\n<content>"
    the extra neighbors slot naturally into document order and appear as continuations
```

### Layer Impact

| Layer | Impact |
|-------|--------|
| `internal/config/config.go` | Add `RAGRetrievalConf` sub-struct + field on `RAGConfig`; extend `ApplyDefaults`. |
| `internal/rag/config.go` | Mirror fields on rag-package `RAGConfig`; extend `ApplyRAGDefaults`. |
| `internal/rag/sqlite_store.go` | `SearchChunks` honors thresholds; new helper `expandNeighbors` (or inline). |
| `internal/rag/doc.go` | No changes to `SearchResult`; optionally add `RetrievalOptions` struct if thread-through wins. |
| `internal/agent/loop.go` | Pass retrieval options from agent config into `SearchChunks`. |
| `internal/web/handler_config.go` | Add `Retrieval *config.RAGRetrievalConf` to `patchRAG`; merge on PUT. |
| `internal/rag/sqlite_store_test.go` (or new file) | Table-driven tests per Test Plan below. |

## 5. Files To Change

| File | Lines | Change |
|------|-------|--------|
| `internal/config/config.go` | ~523-533 | Add `RAGRetrievalConf` struct (`NeighborRadius int`, `MaxBM25Score float64`, `MinCosineScore float64`); add `Retrieval RAGRetrievalConf` field to `RAGConfig`; `ApplyDefaults` leaves `NeighborRadius` at `0` (opt-in). |
| `internal/rag/config.go` | 4-34 | Mirror `RAGRetrievalConf` fields on the rag-package `RAGConfig`; extend `ApplyRAGDefaults` (all zero = disabled). |
| `internal/rag/sqlite_store.go` | 126-240 | Replace `SearchChunks` variadic/positional params with `SearchOptions` struct; filter by `MaxBM25Score` / `MinCosineScore`; call neighbor expansion when `NeighborRadius > 0`. |
| `internal/rag/sqlite_store.go` | new helper | Add `expandNeighbors(ctx, primaries, radius) ([]SearchResult, error)` — one batched SQL to fetch adjacent chunks per doc, de-dup'd. |
| `internal/rag/doc.go` | 43-48 | Introduce `type SearchOptions struct { Limit int; NeighborRadius int; MaxBM25Score float64; MinCosineScore float64 }`. |
| `internal/agent/loop.go` | 134 | Build `SearchOptions` from `a.config.RAG.Retrieval`; pass to `SearchChunks`. |
| `internal/agent/context.go` | existing call site | Update `SearchChunks` call to use `SearchOptions{Limit: N}`. |
| `internal/web/handler_config.go` | 42-44 | Extend `patchRAG` with `Retrieval *config.RAGRetrievalConf`. |
| `internal/web/handler_config.go` | merge block | Merge `patch.RAG.Retrieval` into `merged.RAG.Retrieval` on PUT, preserving unspecified fields. |
| `internal/rag/sqlite_store_test.go` | new cases | Table-driven cases per Test Plan. |
| `internal/agent/context_test.go` | 289 | Update `SearchChunks` stubs to match `SearchOptions` signature. |

## 6. Behavior Changes

**Before:** a chat message arrives. Agent loop embeds the query (if embedding is enabled), calls `ragStore.SearchChunks(ctx, query, queryVec, 5)`. Store runs FTS5, takes the top 50 by BM25, optionally reranks by cosine, returns the top 5 unfiltered. `buildRAGSection` stitches those 5 into `## Relevant Documents:` with headers `### {doc} (chunk N)`. LLM sees fragments with no continuity and no relevance floor.

**After:** agent loop reads `a.config.RAG.Retrieval` → `{NeighborRadius: 0, MaxBM25Score: 0, MinCosineScore: 0}` by default (or whatever the user configured). Passes `SearchOptions` to `SearchChunks`. Store runs FTS5 and optional cosine rerank as before. **New step 1:** candidates above `MaxBM25Score` are dropped from the BM25 path; candidates below `MinCosineScore` are dropped from the cosine path (both are no-ops when set to 0). **New step 2:** surviving candidates are trimmed to top-K as "primary hits". **New step 3:** if `NeighborRadius > 0` (user opted in), the store issues one batched `SELECT ... WHERE doc_id = ? AND idx BETWEEN ? AND ?` per primary hit (or a single `UNION` query across hits), de-dup'd by chunk ID, scored with the primary's score, inserted into the result list in document order (idx ASC). Final list returned to the agent loop.

`buildRAGSection` is unchanged. The prompt now contains continuous passages (`### Manual (chunk 3)`, `### Manual (chunk 4)`, `### Manual (chunk 5)` instead of just chunk 4) and does not contain below-threshold noise. The LLM no longer has to invent the missing sentence because it is now present.

## 7. Test Plan (Strict TDD — tests land BEFORE implementation)

All tests table-driven per golang-pro rules. Target file: `internal/rag/sqlite_store_test.go` (extend existing) plus a small seam in `internal/rag/neighbor_test.go` if helper extraction justifies it.

**Fixtures:** seed a small in-memory SQLite (existing test helper) with a 10-chunk document and a 5-chunk document, both with embeddings.

| # | Case | Setup | Assert |
|---|------|-------|--------|
| T1 | Neighbor expansion happy path | 1 primary hit at idx=4, radius=1 | Results contain chunks idx=3,4,5 in doc order; length = 3. |
| T2 | Neighbor radius 0 is a no-op | 1 primary hit at idx=4, radius=0 | Results contain only idx=4; identical to pre-change behavior. |
| T3 | Neighbor clamp at document edges | Primary hit at idx=0, radius=2 | Results contain idx=0,1,2 (no idx=-1 or -2); no error. |
| T4 | Neighbor clamp at document end | Primary hit at idx=9 (last), radius=2 | Results contain idx=7,8,9; no idx=10. |
| T5 | Neighbor dedup with overlapping top-K | Two primaries at idx=2 and idx=4, radius=2 | idx=3 appears once (not twice); results ordered 0,1,2,3,4,5,6. |
| T6 | BM25 threshold drops weak chunks | MaxBM25Score set below the worst candidate's bm25 (i.e. stricter ceiling); radius=1 | Above-threshold primaries are dropped AND their neighbors are not added. |
| T7 | Cosine threshold drops weak chunks | queryVec provided, embeddings on 3 chunks, MinCosineScore=0.9; only 1 chunk above threshold | Result length = 1 (plus its neighbors if radius>0). |
| T8 | No-embeddings path unaffected by cosine threshold | queryVec=nil OR fewer than 2 chunks embedded; MinCosineScore=0.9 | Threshold ignored; BM25 path returns normally. |
| T9 | Empty FTS5 result stays empty | Query matches nothing | Returns `nil, nil` — neighbor expansion never runs; thresholds never run. |
| T10 | Both thresholds enabled together | MinBM25Score + MinCosineScore both > 0 | Only chunks passing BOTH thresholds (on the chosen ranking path) survive. Neighbors still inherit survivor status. |
| T11 | Cross-document neighbors don't leak | Primary A at (docX, idx=4), Primary B at (docY, idx=4), radius=1 | Neighbors of A come only from docX; neighbors of B only from docY. |
| T12 | De-dup against primary hits | Primary at idx=3, another primary at idx=5, radius=1 | idx=4 is fetched as a neighbor exactly once; not duplicated across both primaries' halos. |
| T13 | Score inheritance on neighbors | Primary with score=0.8 at idx=2, neighbor idx=3 | Neighbor's `SearchResult.Score == 0.8` (not zero, not recomputed). |
| T14 | Config default wiring | `ApplyRAGDefaults` on zero-valued `RAGConfig` | `Retrieval.NeighborRadius == 0`; thresholds stay at `0`. |
| T15 | PUT /api/config accepts retrieval sub-tree | HTTP test: PUT body `{"rag":{"retrieval":{"neighbor_radius":2,"min_cosine_score":0.7}}}` | GET returns the same values; stored config file contains them. |
| T16 | PUT /api/config preserves unspecified retrieval fields | Start with `{neighbor_radius:3, max_bm25_score:1.5}`; PUT only `{min_cosine_score:0.8}` | All three fields end up set to `{3, 1.5, 0.8}` (no reset-to-zero regression). |

`go vet ./...` and `golangci-lint run` must pass. Race detector (`go test -race`) on the RAG package is mandatory before commit. Coverage target ≥ 80% per golang-pro rules.

## 8. Rollout

### Backward Compatibility (defaults-as-no-op goal)

- `MaxBM25Score = 0` and `MinCosineScore = 0` → no chunks filtered. Existing deployments see identical filtering behavior.
- `NeighborRadius` defaults to **0** — no behavior change on first upgrade. Users enable explicitly (e.g. `neighbor_radius: 1`) when they observe the continuity problem. Conservative, opt-in.
- The change does NOT require re-indexing, DB migration, or re-embedding. Existing `document_chunks` rows are immediately usable — the feature operates on top of them.
- `patchRAG` is additive: adding `Retrieval` does not break existing PUTs that only send `embedding`.

### Phased Rollout

This is a small, surgical change — one phase is sufficient.

1. **Phase 1 (this change) — land retrieval precision.** Tests first (Test Plan §7), then implementation, then PUT /api/config plumbing, then manual smoke with a seeded knowledge base. Tree stays green throughout.

A future **Phase D** (NOT in this change) will cover chunk-size bump + re-index UI + per-chunk embedding-model tracking.

### Rollback Plan

- **Config-level rollback (cheap):** set `rag.retrieval.neighbor_radius: 0` (already the default) and both thresholds to `0`. The retrieval path behaves exactly like pre-change code. No restart-safe state is touched; the knob reverts behavior immediately.
- **Full revert (last resort):** `git revert` of the change leaves no schema debt — this change adds no tables, no columns, no migrations. Config files that mention `rag.retrieval.*` will be tolerated by the old code (unknown keys are ignored by yaml decoding under the current schema; verify this during apply).

### Release Notes (draft snippet)

> RAG retrieval gains optional neighbor-chunk expansion: set `rag.retrieval.neighbor_radius: 1` (or higher) to stitch adjacent chunks around each hit so answers preserve continuity across chunk boundaries. New optional thresholds (`rag.retrieval.max_bm25_score`, `rag.retrieval.min_cosine_score`) let you drop weak matches before they reach the model. All knobs default to `0` (disabled) — no behavior change on upgrade; opt in explicitly.

## 9. Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Default `NeighborRadius=0` means existing users must opt in to get the continuity fix | Low | Low | Acceptable tradeoff: no silent behavior change on upgrade. Release notes and config comment explain how to enable. |
| Extra SQL round-trip per search adds latency | Low | Low | Neighbor fetch is a single batched query per search (not per hit). SQLite is local — cost is microseconds. Benchmark during apply; if >5% search latency bump, switch to a `UNION ALL` / `IN` pattern. |
| BM25 threshold semantics confuse users (lower/negative = better) | Medium | Low | Knob is named `max_bm25_score` (ceiling, not floor) and the config comment includes an explicit asymmetry paragraph explaining the BM25/cosine inversion. T6 locks the orientation in tests. |
| Dedup logic has an off-by-one at document boundaries | Medium | Medium | T3, T4, T11 explicitly cover edge cases. |
| `patchRAG` regression — new field silently dropped on PUT (prior session's flagged risk) | High if forgotten | Medium | T15 and T16 lock the round-trip. Without those tests, history says this WILL regress. |
| Smart Retrieval test suite (internal/agent/context_test.go:289) breaks due to signature change | Medium | Low | Apply phase updates stubs; if a back-compat wrapper on `SearchChunks` is cheaper, prefer it. |
| Future Phase D chunk-size bump interacts awkwardly with neighbor radius | Low | Low | Phase D owns the re-index + re-tune; neighbor radius will be re-evaluated there. Document the coupling in Phase D's future proposal. |

## 10. Success Criteria

- [ ] `SearchChunks` accepts `SearchOptions`; neighbor expansion is opt-in via `NeighborRadius > 0`; default `NeighborRadius: 0` preserves pre-change behavior exactly.
- [ ] Score thresholds drop below-threshold chunks AND their would-be neighbors when set > 0.
- [ ] `PUT /api/config` accepts `{"rag":{"retrieval":{...}}}` and the values persist across restart (not just in-memory).
- [ ] Every Test Plan case (T1–T16) passes.
- [ ] `go vet ./...`, `golangci-lint run`, and `go test -race ./internal/rag/... ./internal/agent/... ./internal/web/...` all green.
- [ ] Coverage ≥ 80% in `internal/rag` (per golang-pro rules).
- [ ] No DB schema change, no migration, no re-index required.
- [ ] Manual smoke: seed a 20-chunk document with a known answer spanning chunks 7-8, ask a question whose answer is on chunk 8's opening sentence; with `neighbor_radius=1` (opted in), the answer appears coherent; with `neighbor_radius=0` (default), the old truncation behavior reproduces.

## 11. Dependencies

- No new Go modules.
- No new external services.
- Uses existing `internal/rag/sqlite_store.go` SQLite handle and schema.
- Builds on the `rag.embedding` config pattern shipped in v0.7.0.
