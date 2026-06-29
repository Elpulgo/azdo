# Phase 3 — GitHub Backend

**Ticket:** N/A
**Branch:** feature/gh-phase-3
**Author:** Oscar Larsson
**Created:** 2026-06-29

## Goal

A complete `internal/github` package implements `provider.Provider` — Issues→Work
Items, PRs→Pull Requests, Actions→Pipelines — with full mapping tests and a
conformance gate, compiled and reviewable but **not yet wired** into the running
app (config/wizard/composite are Phase 4). Azure-only users see zero change.

## Constraints

- **No app wiring.** Nothing in `cmd/`, `internal/config`, or `internal/app` changes
  behavior. The package is dormant until Phase 4 — the only repo-wide additions are the
  `KindGitHub` enum value + its `display` glyph/label/style case, which stay inert for
  Azure-only users.
- Builds on Phases 0–2: neutral types, semantic enums, `ListOpts`, the `display` glyph
  maps, and `Terms` already exist and are reused, not redefined.
- **stdlib only** (`net/http`, `encoding/json`) — no new module deps. The minimal GraphQL
  path is hand-built query strings over the same HTTP client.
- Identity is stamped `(KindGitHub, owner/repo, number)` **only** at the mapping boundary,
  never in views (Phase 0 invariant).
- TDD: mapping/enum/label/URL tests first, against **inline JSON fixtures** mirroring the
  `azdevops` tests. Network code stays thin; HTTP paths get integration tests run
  **manually** (sandbox has no network, blocks `/tmp`: use worktree-local
  `GOCACHE`/`TMPDIR`, `CGO_ENABLED=0`).

## Scope

**In scope:**
- New `internal/github`: per-repo `Client` (stdlib net/http), a `MultiClient` fan-out across
  repos, and an `Adapter` satisfying `provider.Provider`; wire types, mappers, the
  label-convention parser, the token source, and `WebURL` builders.
- `KindGitHub` enum value + its `display` glyph/label/style case.
- Full unit coverage of mapping; manual integration tests for the HTTP/GraphQL paths.

**Out of scope (Phase 4 or later):**
- Config fields, setup wizard, `azdo auth` GitHub path, `main.go` construction, and the
  `CompositeProvider` merge — all Phase 4.
- Metrics (Azure-only by ADR decision 4).
- Projects v2 / custom fields; GitHub **Checks** (Actions-only this phase).
- Any view/UI rendering change beyond the inert `KindGitHub` display case.

## Approach

Mirror the `azdevops` layering. A per-repo `Client` over `net/http` does fetch + JSON
decode; `mapping.go` / `mapper_enums.go` translate wire→neutral and stamp
`(KindGitHub, owner/repo, number)`. A `MultiClient` fans out across repos
(goroutine-per-repo, merge+sort by date, `provider.PartialError`) and an `Adapter` exposes
`provider.Provider`. Type/priority come from a configurable label-convention parser;
`UpdateThreadStatus` (resolve conversation) is the one spot with no REST equivalent, handled
by a minimal hand-built GraphQL `resolveReviewThread` mutation. Everything is unit-tested
against inline JSON fixtures; HTTP paths are integration-tested manually.

## Decisions

| # | Question | Decision | Rationale |
|---|----------|----------|-----------|
| 1 | HTTP client? | Hand-rolled `Client` over stdlib `net/http` + `encoding/json`, mirroring `azdevops.Client` | stdlib-first, zero new dep tree, full control over mapping |
| 2 | REST vs GraphQL? | REST for everything except a minimal GraphQL `resolveReviewThread` (+ `/search/issues` for mine-filters) | GitHub has no REST resolve-conversation; keep GraphQL to one mutation |
| 3 | Phase boundary? | Ship `internal/github` + tests + conformance, **unwired**; no config/main.go/composite | Keeps the diff reviewable; Phase 4 owns wiring |
| 4 | Type/priority source? | Configurable label-convention parser; defaults `type:` / `priority:` (case-insensitive); unmatched labels → `Tags` | ADR decision 2; no Projects v2 / GraphQL custom fields |
| 5 | PR iterations on GitHub? | One synthetic iteration = the whole PR; changes from `GET /pulls/{n}/files` | GitHub has no per-push iteration; the diff view still gets its file list |
| 6 | CI source? | Actions **runs** only (not Checks); `GetBuildLogContent` uses the per-job plaintext log endpoint | Actions maps cleanly to `PipelineRun`; Checks deferred; per-run zip avoided |
| 7 | Multi-repo fan-out? | `github.MultiClient` mirroring `azdevops.MultiClient` (goroutine/merge/sort/`PartialError`); `Scope` = `owner/repo` | Same proven shape; the "project" fan-out generalizes to repos |

## Tasks

- [x] 1. Add `KindGitHub` to the `provider` `Kind` enum; fill the GitHub case in `display.KindGlyph`/`KindLabel`/`KindStyle` (replace the Phase 2 placeholders); table-test all three.
- [x] 2. Scaffold `internal/github`: a per-repo `Client` (owner, repo, base URL, token, `*http.Client`) with shared request + JSON-decode + error helpers reusing `provider.PartialError`; add a token source (keyring + `GITHUB_TOKEN` env fallback). (blocked by: none)
- [x] 3. Define GitHub wire types (issue, label, user, pull, review, reviewComment, run, job, step) and neutral enum mappers — state(+`state_reason`)→`StateCategory`, review→`VoteKind`, run status×conclusion→`RunStatus`; table-test each. (blocked by: 1)
- [x] 4. Label-convention parser: prefixes (default `type:` / `priority:`, case-insensitive, injectable) → `ItemType` + `Priority`; unmatched labels → `Tags`; table-test incl. the no-match defaults. (blocked by: 3)
- [x] 5. Issue→`WorkItem` and issue-comment→`WorkItemComment` mappers, stamping `(KindGitHub, owner/repo, number)`; inline-JSON fixture tests asserting the identity invariant + fields. (blocked by: 4)
- [x] 6. PR→`PullRequest` (reviewers from reviews→votes), review-comments→`Thread`/`Comment`, files→`IterationChange` mappers; fixture tests. (blocked by: 3)
- [x] 7. Actions run→`PipelineRun` and jobs/steps→`Timeline` mappers; fixture tests. (blocked by: 3)
- [x] 8. Work-item `Client` methods: `ListWorkItems`/`ListMyWorkItems` (`/issues` + `/search/issues` for mine), `GetWorkItemTypeStates`→open/closed, `UpdateWorkItemState` (PATCH state+reason), comments get/add. (blocked by: 5)
- [x] 9. PR `Client` methods: list/my/as-reviewer (search), `GetPRThreads`, `VotePullRequest`→submit review, `GetFileContent`, add code/general comment, reply; `UpdateThreadStatus` via the minimal GraphQL `resolveReviewThread`. (blocked by: 6)
- [x] 10. Pipeline `Client` methods: `ListPipelineRuns`, `GetBuildTimeline` (jobs+steps), `GetBuildLogContent` (per-job plaintext log). (blocked by: 7)
- [ ] 11. `WebURL` builders (`WorkItemURL`/`PRURL`/`PRThreadWebURL`/`PipelineURL`) from `html_url` shapes; table-test the exact URLs. (blocked by: 1)
- [ ] 12. `github.MultiClient` fan-out across repos (goroutine/merge/sort/`PartialError`) + `Adapter` satisfying `provider.Provider`; `//go:build adapter` conformance test mirroring `azdevops`; `CGO_ENABLED=0 go test/vet ./...` green; integration tests written, run manually. (blocked by: 8,9,10,11)

## Review feedback: Task 4

- **RESOLVED (hardening, follow-up commit).** Reviewer raised one 🔴 (latent) + one
  live 🟡; both fixed in the parser:
  - **Empty-prefix footgun (🔴):** `Parse` now guards `prefix != ""` before matching, so a
    zero-value `LabelConvention` (a Phase-4 config that leaves a prefix blank) routes every
    label to `Tags` instead of greedily consuming the first two. Pinned by
    `TestLabelConventionEmptyPrefixMatchesNothing` + per-prefix variant.
  - **Silent drop of recognised-but-unparseable labels (🟡, live under defaults):**
    `priority:high`, `type:chore`, bare `type:` etc. now fall through to `Tags` instead of
    vanishing. Rule is now uniform: **a prefixed label is consumed only if it yields a usable
    value** (recognised type, or priority 1–4); otherwise it stays a visible tag and a later
    well-formed label with the same prefix can still win. `mapItemType` returns `(type, ok)`;
    priority match gated on `!= 0`.
  - 🟢 notes (ASCII slice-offset on non-ASCII custom prefixes; leading-whitespace label names)
    logged as Phase-4 considerations — not reachable with ASCII defaults.

## Review feedback: Task 6

- **RESOLVED (fix commit `01d3781`).** `MapReviewThreads` rewritten to two passes:
  pass 1 registers every root (`InReplyToID==nil`) in first-seen order; pass 2
  attaches replies and only then creates defensive orphan threads — so a reply
  preceding its root no longer collides with the real root or drops the reply.
  New test `TestMapReviewThreads_ReplyBeforeRoot` pins it (one thread, root first,
  reply with ParentCommentID 1); the absent-root orphan test still passes. The
  🟢 tie-break comment was folded into `MapReviewers`. 73 tests green, vet clean.

- 🔴 **(must-fix) Thread grouping is not order-independent.** `MapReviewThreads`'
  single-pass loop assumes a reply never precedes its root. For input
  `[reply(id=2,→1), root(id=1)]` the reply hits the defensive orphan branch
  (`threadMap[1]` created with the reply as root), then the real root overwrites
  `threadMap[1]` AND re-appends `1` to `threadOrder` → two threads with identical
  Identity "1" and the reply silently dropped. Latent under GitHub's default
  root-first ordering, but the code claims to be defensive/order-independent and
  isn't. **Fix:** make grouping genuinely order-independent — two passes (register
  all roots first, then attach replies), or in the root branch adopt an existing
  orphan entry instead of overwriting + re-appending to `threadOrder`. Add a test
  `[reply(id=2,→1), root(id=1)]` asserting exactly one thread, identity "1", two
  comments in correct parent order.
- 🟢 **(fold in) MapReviewers equal-timestamp tie-break** is order-dependent
  (`.After` → first-seen wins on ties). Add a one-line comment documenting
  "equal SubmittedAt → first-seen wins (GitHub returns chronological order)".

## Validation: Task 8

- **COMPLETE** (commit `36631ee`). All six per-repo `Client` methods exist in
  `internal/github/workitems.go` with the required shapes (no scope param, wire
  types returned except the synthesized states):
  - `ListWorkItems(top int, opts provider.ListOpts) ([]Issue, error)` — GET `/issues`,
    filters PRs via `Issue.PullRequest != nil`, caps top at 100, maps states.
  - `ListMyWorkItems(top int, opts provider.ListOpts) ([]Issue, error)` — GET
    `/search/issues` with `is:issue assignee:@me`, unwraps `issueSearchResponse`.
  - `GetWorkItemTypeStates(_ string) ([]provider.WorkItemTypeState, error)` — static
    open/closed, no HTTP.
  - `UpdateWorkItemState(number int, state string) error` — PATCH `state`+`state_reason`
    via new `doJSON` helper; rejects unknown states.
  - `GetWorkItemComments(number int) ([]IssueComment, error)` — GET issue comments.
  - `AddWorkItemComment(number int, text string) (IssueComment, error)` — POST comment.
- `types.go` adds `PullRequest *json.RawMessage` to `Issue`; `client.go` adds `doJSON`.
- Unit tests in `workitems_test.go` cover all six methods (18 `Test*` funcs).
- Integration test `workitems_integration_test.go` carries `//go:build integration` and
  is excluded from the default run.
- Test result (worktree-local cache/tmp, CGO off):
  - `go test ./internal/github/...` → `ok ... 0.026s`
  - `go vet ./internal/github/...` → clean (no output)

## Validation: Task 9

- **COMPLETE** (commit `bd22d50`). All 11 per-repo `Client` methods exist in
  `internal/github/pullrequests.go` with the required no-scope/no-repositoryID
  shape, returning wire types (`PullRequest`/`ReviewComment`/`PRFile`/`IssueComment`)
  except `GetFileContent`→`string` and the mutation methods:
  - `ListPullRequests(top, opts) ([]PullRequest, error)` — GET `/pulls`.
  - `ListMyPullRequests(top, opts)` and `ListPullRequestsAsReviewer(top, opts)` →
    GET `/search/issues` with `is:pr` + `author:@me` / `review-requested:@me`.
  - `GetPRThreads(number) ([]ReviewComment, error)` — flat list, grouping deferred.
  - `GetPRFiles(number) ([]PRFile, error)`.
  - `VotePullRequest(number, vote) error` — POST `/reviews`, vote→APPROVE/REQUEST_CHANGES/COMMENT.
  - `GetFileContent(filePath, branch) (string, error)` — base64 newline-strip + decode.
  - `AddPRCodeComment(number, filePath, line, content) (ReviewComment, error)` — fetches head SHA then POSTs.
  - `AddPRComment(number, content) (IssueComment, error)` — issue-comment endpoint.
  - `ReplyToThread(number, rootCommentID, content) (ReviewComment, error)` — `in_reply_to`.
  - `UpdateThreadStatus(number, rootCommentID, status) error` — minimal GraphQL
    `resolveReviewThread`/`unresolveReviewThread`, matching by first-comment `databaseId`.
- `types.go` adds `SHA` to `PullRequestBranch` (for code-comment `commit_id`).
- Unit tests in `pullrequests_test.go` cover all 11 methods (26 `Test*` funcs),
  including 6 GraphQL `UpdateThreadStatus` tests (resolve/unresolve/no-match/unrecognized/vars).
- Integration test `pullrequests_integration_test.go` carries `//go:build integration`
  and is excluded from the default run.
- Test result (worktree-local cache/tmp, CGO off):
  - `go test -count=1 ./internal/github/...` → `ok ... 0.038s`
  - `go vet ./internal/github/...` → clean; `gofmt -l internal/github/` → no output.

## Unknowns

- **RESOLVED (task 9).** Resolving a thread: matched by first-comment `databaseId` via a
  GraphQL `reviewThreads(first:100)` query; on no-match a descriptive error is returned
  (not a silent no-op). See `UpdateThreadStatus` in `internal/github/pullrequests.go`.
- **RESOLVED (task 9).** Search-PR fidelity: `/search/issues` items are issue-shaped;
  `Draft`, `Head`, and `Base` are left zero; `merged_at` is captured from the nested
  `pull_request` sub-object. Partial map is accepted for list views; N+1 fetch avoided.
- `/search/issues` rate limits (30/min) and `Link`-header pagination vs Azure `$top`;
  per-repo paging is approximated, global top-N merged like the Azure fan-out.
- Exact keyring user key for the GitHub token (proposed `github-token` under service
  `azdo-tui`).
- Whether `state_reason` (`completed` / `not_planned`) maps to distinct categories or
  both fold into `StateCategoryClosedDone`.
