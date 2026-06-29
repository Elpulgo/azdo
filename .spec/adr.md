# Spec: GitHub Provider Parity

Status: Draft / proposal
Scope: Add GitHub as a second backend **alongside** Azure DevOps. Metrics view is **out of scope** and stays **Azure-only** for now (see [Caveats](#caveats)).

### Resolved decisions (drive the design below)

1. **Both providers merge into one list in v1.** A user already on Azure who
   adds GitHub sees PRs / work items / pipelines from *both* in the same
   views — not a provider switch. This is the whole point; a single-active
   provider would add no value for existing users.
2. **Work-item type & priority on GitHub come from a label convention** — no
   Projects v2 / GraphQL custom fields in v1.
3. **Tab/term names stay ("Work Items", "Pipelines") but are user-configurable**
   via the same `DisplayNames` pattern already in config — not hard-branched on
   provider.
4. **Metrics is Azure-only.** No GitHub story; it stays scoped to Azure data.

---

## 1. Goal

Today the app is hard-wired to Azure DevOps end to end: the concrete
`*azdevops.MultiClient` is constructed in `cmd/azdo-tui/main.go`, threaded
through `app.NewModel`, and handed to every view, which then calls
Azure-specific methods and renders Azure-specific strings. Azure concepts
(WIQL, `System.*` field names, `refs/heads/...`, vote integers, the
`dev.azure.com` URL shape) leak from the API layer up into the UI.

We want a user to point the same TUI at **either** Azure DevOps **or** GitHub
(eventually both at once) and get the same three working views — Pull
Requests, Work Items/Issues, Pipelines/Actions — with provider-appropriate
labels and icons.

The work splits into three big movements:

1. **Define a provider interface** that both backends implement, returning
   provider-neutral domain types.
2. **Lift Azure-specific syntax** (WIQL, field names, ref prefixes, URL
   building, state/type vocab) out of the UI and behind the interface.
3. **Tag the UI with a provider** so it can swap labels, icons, and
   terminology without branching everywhere.

---

## 2. Current architecture (what exists)

| Concern | Where | Notes |
|---|---|---|
| Client construction | `cmd/azdo-tui/main.go:196` `azdevops.NewMultiClient(org, projects, pat, displayNames)` | Single concrete type |
| App model holds client | `internal/app/app.go:66` `client *azdevops.MultiClient` | Concrete, not an interface |
| Views receive client | `app.go:397-400` `NewModelWithStyles(client, ...)` | Each view calls concrete methods |
| Polling abstraction | `internal/polling/poller.go:23` `PipelineClient` interface | **Already** a narrow interface — the pattern to copy |
| Domain types | `internal/azdevops/{workitems,git,types,comments}.go` | `System.Title` etc. as JSON struct tags |
| Auth | `internal/config/keyring.go` (service `azdo-tui`) + `AZDO_PAT` env | PAT only |
| Config | `internal/config/config.go` | `Organization`, `Projects[]`, `DisplayNames`, per-pane enable |
| Browser URLs | built **inline in the UI**, e.g. `workitems/detail.go:597`, `pullrequests/detail.go:643` | Hard-coded `dev.azure.com` format strings |

Key observation: the polling layer already proves the interface approach
works — `Poller` only knows `ListPipelineRuns(top int)`, satisfied by both
`*Client` and `*MultiClient`. We generalize that idea to the whole surface.

---

## 3. Provider interface (the central design)

Introduce `internal/provider` with neutral domain types and a `Provider`
interface. `azdevops` becomes one implementation; a new `github` package the
other. Views depend on `provider.Provider`, never on a concrete client.

Rough shape (illustrative, not final):

```go
type Provider interface {
    Kind() Kind // ProviderAzure | ProviderGitHub

    // Pull requests
    ListPullRequests(opts ListOpts) ([]PullRequest, error)
    ListMyPullRequests(opts ListOpts) ([]PullRequest, error)
    ListPullRequestsAsReviewer(opts ListOpts) ([]PullRequest, error)
    GetPRThreads(pr PRRef) ([]Thread, error)
    VotePullRequest(pr PRRef, vote Vote) error
    AddPRComment(pr PRRef, body string) (*Thread, error)
    // ... reply, resolve, code comment, iterations, file content

    // Work items / issues
    ListWorkItems(opts ListOpts) ([]WorkItem, error)
    ListMyWorkItems(opts ListOpts) ([]WorkItem, error)
    GetWorkItemComments(ref WorkItemRef) ([]Comment, error)
    AddWorkItemComment(ref WorkItemRef, body string) (*Comment, error)
    UpdateWorkItemState(ref WorkItemRef, state string) error
    AvailableStates(itemType string) ([]string, error)

    // Pipelines / actions
    ListPipelineRuns(opts ListOpts) ([]PipelineRun, error)
    GetTimeline(run RunRef) (*Timeline, error)
    ListLogs(run RunRef) ([]BuildLog, error)
    GetLogContent(ref LogRef) (string, error)

    // Cross-cutting
    WebURL(ref any) string // replaces inline dev.azure.com builders
}
```

Domain types (`WorkItem`, `PullRequest`, `PipelineRun`, `Thread`, etc.) move
to `internal/provider` and lose their Azure JSON tags. Each backend maps its
wire format → neutral type. The `ProjectName` / `ProjectDisplayName` fields
generalize to a **`Scope`** concept (Azure project *or* GitHub repo). Every
returned item also carries its **origin** (`Kind` + scope) so the merged UI can
icon/group rows by where they came from.

### 3.1 Merged providers (decision 1)

The app shows **both** backends in one list. The clean way: an **aggregating
provider** that implements the same `Provider` interface and fans out to N
real backends, then merges + sorts — exactly what `azdevops.MultiClient` does
today across *projects*. We generalize that fan-out from "projects" to
"backends":

```go
type CompositeProvider struct { backends []Provider } // satisfies Provider

func (c *CompositeProvider) ListPullRequests(o ListOpts) ([]PullRequest, error) {
    // goroutine per backend, merge, sort by date, wrap failures in PartialError
}
```

Consequences that shape the rest of the work:
- **`PartialError` becomes load-bearing.** If GitHub is down but Azure is up,
  the list still renders Azure rows + a non-fatal banner. The pattern already
  exists in `azdevops/errors.go` — promote it to `internal/provider`.
- **Stable cross-provider identity.** List selection, state persistence
  (`internal/state`), and "open detail" must key on `(Kind, scope, id)` — a
  bare int ID now collides across providers.
- **Sorting/paging is the composite's job.** Each backend has its own paging
  (`$top` vs `Link` header); the composite requests from each then merges by
  `ChangedDate` / `CreationDate`. A global "top N" is approximate.
- `app.Model.client` becomes `provider.Provider` and is usually a
  `CompositeProvider` — views don't know or care how many backends are behind it.

---

## 4. Feature parity — UI element inventory

Legend: **azdo** = the label this app shows today. Excludes the metrics view.
✅ direct equivalent · ⚠️ partial / needs mapping · ❌ no native equivalent.

### 4.1 Pull Requests view

| UI element | Azure DevOps term | azdo label | GitHub equivalent | Parity |
|---|---|---|---|---|
| Tab | Pull Requests | "Pull Requests" (`1`) | Pull requests | ✅ |
| List col: status | active / completed / abandoned / draft | `● Active` `✓ Merged` `○ Closed` `◐ Draft` | open / merged / closed / draft | ✅ |
| List col: title | Title | "Title" | Title | ✅ |
| List col: branches | `refs/heads/x → refs/heads/main` | "source → target" | head → base | ✅ (drop `refs/heads/`) |
| List col: author | Created By | "Author" | user (author) | ✅ |
| List col: repo | Repository | "Repo" | Repository | ✅ |
| List col: project | Team Project | "Project" (multi only) | ❌ no project layer → repo/owner | ⚠️ remap to scope |
| List col: reviews | Reviewer votes | `✓ N` `◐ N` `✗ N` … | Review states | ⚠️ see votes |
| Filter: my PRs | created by `@Me` | "My PRs" (`m`) | `author:@me` | ✅ |
| Filter: as reviewer | reviewer = me | "As reviewer" (`A`) | `review-requested:@me` | ✅ |
| Detail: header | `PR #ID: Title` | same | `#ID Title` | ✅ |
| Detail: reviewers + vote | vote −10..10 | `✓ Approved` `◐ Waiting` `~ Suggestions` `✗ Rejected` `○ No vote` | APPROVED / CHANGES_REQUESTED / COMMENTED / PENDING | ⚠️ no "approve w/ suggestions"; no −5/+5 granularity |
| Detail: vote action | set vote | "Vote" (`v`) | submit review | ⚠️ fewer options |
| Detail: changed files | iteration changes | `+ ~ - →` per file | files changed (add/modify/remove/rename) | ✅ |
| Detail: general comments | thread (no file context) | "General comments" | issue-style PR comment | ✅ |
| Diff: code comment | thread w/ `ThreadContext` | create comment (`c`) | review comment on line | ⚠️ Azure threads vs GitHub review model differ |
| Diff: reply to thread | comment reply | reply (`p`) | reply to review comment | ✅ |
| Diff: resolve thread | thread status active/fixed/closed | resolve (`x`) | resolve conversation | ⚠️ status vocab differs |
| Open in browser | PR overview URL | "Open" (`o`) | PR html_url | ✅ (different URL builder) |

### 4.2 Work Items view → GitHub Issues

| UI element | Azure DevOps term | azdo label | GitHub equivalent | Parity |
|---|---|---|---|---|
| Tab | Work Items | "Work Items" (`2`) | Issues | ⚠️ rename per provider |
| List col: type | Work Item Type | `Bug` `Task` `Story` `Feature` `Epic` `Issue` | ❌ no native type | ⚠️ derive from labels / issue type (beta) |
| List col: ID | Work Item ID | "ID" | Issue number | ✅ |
| List col: title | System.Title | "Title" | Title | ✅ |
| List col: state | New/Active/Resolved/Closed/Removed | colored state | open / closed (+ `state_reason`) | ⚠️ only 2 states natively |
| List col: priority | Microsoft.VSTS.Common.Priority | `P1`–`P4` | ❌ no priority field | ⚠️ label convention or Projects field |
| List col: assigned | System.AssignedTo | "Assigned" | assignee | ✅ |
| List col: project | Team Project | "Project" (multi only) | ❌ → repo | ⚠️ remap to scope |
| Filter: my items | `@Me` | "My items" (`m`) | `assignee:@me` | ✅ |
| Filter: by tag | System.Tags | "Tags" (`T`) | Labels | ✅ (tags ≈ labels) |
| Filter: by state | state picker | "State" (`s`) | open/closed filter | ⚠️ fewer states |
| Detail: header | `#ID: Title` | same | `#ID Title` | ✅ |
| Detail: meta line | `Type \| State \| P#` | same | label/state/(no prio) | ⚠️ |
| Detail: assigned to | AssignedTo | "Assigned To" / "Unassigned" | assignee(s) | ✅ (GitHub allows multiple) |
| Detail: iteration | System.IterationPath (`Sprint 1\Week 1`) | "Iteration" | ❌ → Milestone (flat, no path) | ⚠️ |
| Detail: tags | System.Tags | "Tags" | Labels | ✅ |
| Detail: last changed | System.ChangedDate | timestamp | updated_at | ✅ |
| Detail: description | Description / ReproSteps (Bug) | HTML-stripped body | body (Markdown) | ⚠️ Azure HTML vs GitHub Markdown |
| Detail: discussion | work item comments | "Discussion" | issue comments | ✅ |
| Action: change state | UpdateWorkItemState (WIQL states) | "Change state" (`w`) | open/close (+ reason) | ⚠️ |
| Action: comment | add comment | "Comment" (`c`) | add issue comment | ✅ |
| Open in browser | `_workitems/edit/ID` | "Open" (`o`) | issue html_url | ✅ |

### 4.3 Pipelines view → GitHub Actions

| UI element | Azure DevOps term | azdo label | GitHub equivalent | Parity |
|---|---|---|---|---|
| Tab | Pipelines | "Pipelines" (`3`) | Actions | ⚠️ rename per provider |
| List col: status | status + result | `● Running` `○ Queued` `✓ Success` `✗ Failed` `⊘ Cancel` `◐ Partial` | queued / in_progress / completed × conclusion | ⚠️ map status×conclusion; no "partial" |
| List col: pipeline | Pipeline (definition) | "Pipeline" | Workflow name | ✅ |
| List col: branch | Source Branch | "Branch" | head_branch | ✅ |
| List col: build | Build Number | "Build" | run_number | ✅ |
| List col: timestamp | Queue Time | "Timestamp" | created_at | ✅ |
| List col: duration | derived | "Duration" | updated−created | ✅ |
| List col: project | Team Project | "Project" (multi only) | ❌ → repo | ⚠️ remap to scope |
| Filter: by status | status picker | "Status" (`S`) | status filter | ⚠️ vocab differs |
| Detail: timeline | Timeline (Stage→Job→Task) | tree, `▶`/`▼`, `📄` | Jobs → Steps (2 levels, no stages) | ⚠️ shallower hierarchy |
| Detail: record state | succeeded/failed/skipped/… | status icons | success/failure/skipped/cancelled | ✅ |
| Log viewer | build log | log content | job/step logs | ⚠️ GitHub logs are per-job zip, different fetch |

### 4.4 Global / shared (not provider-specific, but touched)

| UI element | azdo today | GitHub change needed |
|---|---|---|
| Tab bar | PRs / Work Items / Pipelines / Metrics | Labels swap to Issues / Actions when provider = GitHub |
| Status bar | shows org + project | show owner + repo for GitHub |
| Setup wizard | Organization, Projects, interval, theme | add provider choice; Projects → repos for GitHub |
| PAT input | "Azure DevOps PAT" + scopes (Build/Code/Work Items) | GitHub token + scopes (repo, workflow, read:org) |
| Theme / help / error modals | provider-agnostic | no change |
| Icons | per state/type/vote/result | mostly reusable; type icons need GitHub mapping |

---

## 5. Azure-specific syntax to lift behind the interface

These are the leaks to seal. Each must move from UI/shared code into the
`azdevops` implementation so the neutral interface never exposes them:

- **WIQL queries** (`workitems.go`, `metrics.go`) — macros `@project`, `@Me`,
  `System.*`/`Microsoft.VSTS.*` field references. GitHub has **no WIQL**; it
  uses the issue **search syntax** (`is:issue assignee:@me state:open`) or
  GraphQL. Filtering must become a neutral `ListOpts`, each backend builds its
  own query.
- **Field reference names** as JSON tags (`System.Title`, `System.State`,
  `Microsoft.VSTS.Common.Priority`, `...TCM.ReproSteps`) — internal to mapping.
- **State vocabulary** (`New/Active/Resolved/Closed/Removed`) and the
  `StateIcon()` switch — neutralize to a state + category model.
- **Work item type strings** (`Bug/Task/User Story/...`) and Bug-specific
  ReproSteps handling.
- **Reviewer vote integers** (−10..10) — neutralize to a `Vote` enum.
- **Ref name prefixes** (`refs/heads/`, `refs/tags/`) — already shortened by
  `BranchShortName()`; keep neutral.
- **Browser URL builders** — inline `dev.azure.com` `fmt.Sprintf`s in
  `workitems/detail.go`, `pullrequests/detail.go`, `metrics/list.go`. Move to
  a provider `WebURL()`; UI just opens whatever string it gets.
- **API versioning / preview endpoints**, `connectionData` user-ID lookup —
  internal to `azdevops`.

---

## 6. Phases (big-pencil)

### Phase 0 — Extract neutral domain + provider interface
- Create `internal/provider` with neutral types and the `Provider` interface.
- Make `*azdevops.MultiClient` satisfy it (adapter or rename), **no behavior
  change**. App still only talks to Azure.
- Re-type `app.Model.client` and all `NewModelWithStyles` params to
  `provider.Provider`.
- **TDD:** interface conformance tests + existing tests stay green. This is the
  big mechanical refactor; do it first, ship it, verify nothing regressed.

### Phase 1 — Seal the leaks
- Move URL building into `WebURL()`. Replace inline `dev.azure.com` builders.
- Replace UI-side state/type/vote string switches with neutral enums +
  provider-supplied display metadata (icon + color + label).
- Convert WIQL/`@Me`/state filtering call sites to neutral `ListOpts`.
- Still Azure-only, but UI is now provider-blind. Verify visually + tests.

### Phase 2 — Per-row provider origin + configurable labels
- Each list row carries its origin `Kind` + scope; show a small provider glyph
  (e.g. Azure vs GitHub mark) per row so a merged list is legible. Work-item
  **type** icons get a GitHub mapping (derived from labels — see Phase 3).
- **Labels stay generic and user-configurable** (decision 3): keep "Work
  Items" / "Pipelines" as defaults, but let the user override display strings
  via the existing `DisplayNames` config pattern. Do **not** hard-branch tab
  names on `Kind()`.
- Status bar shows the active scopes (Azure projects + GitHub repos) together.

### Phase 3 — GitHub backend
- New `internal/github` package implementing `Provider` via REST (and GraphQL
  where REST is insufficient — issue search).
- Map: Issues→WorkItem, PRs→PullRequest, Actions runs→PipelineRun, reviews→votes.
- **Type & priority from a label convention** (decision 2): a configurable
  prefix scheme (e.g. `type:bug`, `priority:P1`) maps GitHub labels →
  neutral type/priority. Document the default convention; make the prefixes
  configurable. Unmatched labels stay plain tags.
- GitHub auth: token in keyring (new service key) + env fallback; scopes
  `repo`, `workflow`, `read:org`.
- **TDD:** mapping tests against recorded fixtures, mirroring `azdevops` tests.

### Phase 4 — Config, setup, wiring the composite
- Config gains an **optional GitHub section** (owner + repos, label-convention
  prefixes) **alongside** the existing Azure section — both can be present at
  once. Extend `DisplayNames` to cover tab/term overrides.
- Wizard gains optional GitHub steps; an Azure-only user can ignore them.
- `main.go` constructs an `azdevops` backend and/or a `github` backend from
  config, wraps them in a `CompositeProvider`, and hands that to `app.NewModel`.
- `azdo auth` learns a GitHub-token path (separate keyring key) without
  disturbing the existing Azure PAT flow.

---

## 7. Caveats — flag these

- **No WIQL on GitHub.** Custom/saved Azure queries have no equivalent. The
  neutral `ListOpts` can only cover what *both* sides support; advanced WIQL
  filters won't round-trip. Filtering parity is "good enough", not 1:1.
- **GitHub Issues are far simpler than Work Items.** No native *type*
  (Bug/Task/Story/Epic), *priority*, *area path*, *iteration path*, or *story
  points*. **Decision: type/priority come from a configurable label
  convention** (e.g. `type:bug`, `priority:P1`); milestone ≈ iteration (flat,
  no `\` path); area path and story points have no v1 representation. Issues
  whose labels don't follow the convention render with a neutral default type
  and no priority — that's expected, not a bug. Projects v2 custom fields
  (GraphQL-only, per-org) are explicitly **not** used in v1.
- **State model mismatch.** Azure has an open-ended workflow
  (New/Active/Resolved/Closed + custom). GitHub issues are **open/closed**
  (+ `state_reason`). The state picker collapses to two options for GitHub.
- **Reviewer votes are coarser on GitHub.** No "approve with suggestions"
  (+5) or "waiting for author" (−5). Map to APPROVED / CHANGES_REQUESTED /
  COMMENTED / PENDING — some icons go unused.
- **PR code-comment model differs.** Azure = threads with status; GitHub =
  review comments grouped in reviews. Resolve/reply semantics don't line up
  exactly; expect behavioral approximation.
- **Actions hierarchy is shallower.** Azure Timeline is Stage→Job→Task;
  GitHub is Jobs→Steps (no stages). The tree view works but renders 2 levels.
- **Actions logs differ.** GitHub returns logs as a per-run/per-job archive
  (zip), not a simple line-addressable log like Azure build logs — the log
  viewer's fetch path needs separate handling.
- **No "Project" layer in GitHub.** Azure org→project→repo vs GitHub
  owner→repo. The multi-**project** fan-out concept must generalize to
  multi-**repo** (or scope). The "Project" column becomes a repo/scope column.
- **Merged-list identity collision (from decision 1).** Item IDs are only
  unique *within* a provider — GitHub issue #42 and Azure WI 42 will coexist.
  Every selection, detail-open, and persisted-state key must use
  `(Kind, scope, id)`, not a bare int. This touches `internal/state` and each
  view's selection logic — call it out so it isn't missed.
- **Metrics stays Azure-only (decision 4).** It depends on work-item
  **state-transition history**, **story points**, and WIQL — none have clean
  GitHub equivalents. The metrics tab simply shows Azure data; when no Azure
  backend is configured, it's hidden. Not a parity target.
- **HTML vs Markdown bodies.** Azure returns HTML (currently stripped); GitHub
  returns Markdown. The detail renderer needs a per-provider body path.
- **Rate limits & pagination differ.** GitHub's REST rate limits and
  `Link`-header pagination differ from Azure's `$top`; the GitHub client needs
  its own paging, and the concurrent-fan-out pattern may need throttling.

---

## 8. Decisions (resolved)

The original open questions are now settled; the spec above reflects them:

1. **Merged list in v1** — both Azure + GitHub in one list via a
   `CompositeProvider` fan-out (§3.1). Not a single-active-provider switch.
2. **Label convention for type/priority** on GitHub — configurable prefixes,
   no Projects v2 / GraphQL (§Phase 3, §Caveats).
3. **Generic, user-configurable labels** — keep "Work Items"/"Pipelines",
   override via the existing `DisplayNames` config pattern; no hard branch on
   `Kind()` (§Phase 2).
4. **Metrics stays Azure-only** — no GitHub story; tab hides without an Azure
   backend (§Caveats).

### Remaining smaller calls (safe to defer to implementation)

- Exact default label-convention prefixes (`type:` / `priority:` vs something
  else) and whether they're case-sensitive.
- Whether GitHub Checks (vs Actions runs) should also feed the Pipelines view,
  or Actions-only for v1.
- Global "top N" semantics across a merged list (per-backend top N then merge
  is the proposed approximation).
