# Notifications Pane — Phase 1: GitHub

**Ticket:** N/A
**Branch:** feat/notifications-github
**Author:** Oscar Larsson
**Created:** 2026-07-29

## Goal

A default-on Notifications tab — first in the tab bar — that renders the GitHub
user inbox as a provider-neutral attention feed with read/done triage.

## Constraints

- UI has **one entrypoint**: the pane consumes `[]provider.Notification` and never
  knows which backend produced a row (same rule as the PR/work-item/pipeline lists).
- `GET /notifications` is **user-level**; the existing `github.Client` is per-`owner/repo`.
  Notifications cannot reuse the `scope`-routed fan-out (learned convention 2 does not apply).
- Azure DevOps has no inbox API. Phase 1 ships with Azure not implementing the capability.
- No new external dependencies; glob matching via `path.Match`.
- Config keys are lowercased by viper — all keys lowercase snake_case (convention 9).
- `Config.Save()` must never drop existing config. It already round-trips the file through
  `ReadInConfig()` before setting values, so unmanaged keys (`metrics`, `terms`, `github`,
  `notifications`) survive — but **no test pins this**, so task 17 adds one. Comments are
  lost by viper and that is accepted; values are not negotiable.
- **Tests must never write the real config.** `Save()` falls back to `GetPath()` when
  `configPath` is empty, so a `Config{...}` literal in a test rewrites
  `~/.config/azdo-tui/config.yaml`. Always go through `LoadFrom(<t.TempDir() path>)`.

## Scope

**In scope:**
- `provider.NotificationSource` optional capability interface + neutral `Notification` type
- User-scoped GitHub client: list, mark read, mark done, conditional requests
- Composite fan-out over backends that implement the capability
- `notifications` config block (filters, precedence) + `disabled_panes: notifications`
- New first tab, unread footer badge, help-modal section, polling integration
- Docs: ADR, README, Architecture.md, config.yaml.example, FAQ, help modal

**Out of scope:**
- Azure DevOps synthetic feed → phase 2 (`20260729-notif-p2-azdo.md`)
- Unsubscribe-from-thread, marking whole repos read, subscription-rule management
- Jump-to-tab deep-linking (browser `o` only in v1); desktop notifications
- Cross-machine sync; local read/done state (phase 1 is fully server-side)

## Approach

Model notifications as an **optional capability**, not a `Provider` method — mirroring how
Metrics stayed off the interface. `CompositeProvider` type-asserts each backend for
`NotificationSource`, fans out to those that match, merges and sorts newest-first, and
exposes `HasNotifications()` so the app can hide the tab when nothing supports it. GitHub
gets a user-scoped client alongside the per-repo ones, honouring `Last-Modified`/
`If-Modified-Since` and `X-Poll-Interval` so polling stays off the rate limit. Read and
done are server-side (`PATCH`/`DELETE /notifications/threads/{id}`), so phase 1 needs no
local state file. Filtering is a pure function over the merged feed, config-driven.

## Config shape

A `notifications:` block with keys `only_configured_repos`, `exclude_repos`
(glob), `include_repos` (glob), `exclude_reasons`, `unread_only`, `participating_only`,
`since_days`, `max_items`, `poll_interval`. Every default is the widest behaviour — the
whole inbox, nothing filtered, pane on. Task 9 owns defaults and validation; task 17
documents each key in `config.yaml.example`.

Filter precedence, most to least specific: `only_configured_repos` → `include_repos` →
`exclude_repos` → `exclude_reasons` → `unread_only`.

`exclude_reasons` filters on GitHub's `reason` field — why the row reached your inbox
(`mention`, `review_requested`, `subscribed`, `ci_activity`, …). It exists because the repo
filters are all-or-nothing per repo, and the repos you work in hardest emit both your
`review_requested` rows and the bulk of your `subscribed` noise: blacklisting the repo would
cost you the review requests. Reason is the axis that correlates with "needs me".

## Decisions

| # | Question | Decision | Rationale |
|---|----------|----------|-----------|
| 1 | Extend `Provider` or add an optional interface? | Optional `NotificationSource`, discovered by type assertion | No empty Azure stubs, no `ErrUnsupported` runtime path, no typed-nil traps (convention 15). Metrics set the precedent |
| 2 | Show the whole inbox or only configured repos? | Whole inbox by default, narrowable by config | An inbox filtered to 3 repos is not a triage pane. Escape hatches cover the noise |
| 3 | Rows from unconfigured repos? | Shown; `o` opens browser, no tab jump | No client exists for those scopes; browser is the honest fallback |
| 4 | Triage actions in v1? | Read + done only | Both are server-side one-call endpoints. Unsubscribe is destructive and hard to undo from a TUI |
| 5 | Local read/done state? | None in phase 1 | GitHub owns the state. Phase 2 introduces it for Azure — do not build it early |
| 6 | Tab position? | First, before Pull Requests | It is the "what needs me now" pane, so it is the natural landing tab |
| 7 | Merged feed or per-provider sections? | One merged feed, sorted newest-first, provider glyph per row | Matches the merged-list decision from the GitHub-parity work |
| 8 | Polling cadence? | `X-Poll-Interval` as floor, else `notifications.poll_interval`, else global | Conditional requests returning 304 do not count against the rate limit |
| 9 | `state.yaml` version bump? | No bump; add `TabID "notifications"` | Additive — old files simply restore a different tab |
| 10 | `participating_only` and `exclude_reasons` overlap — keep both? | Both, and they compose: `participating_only` narrows server-side, `exclude_reasons` trims the result client-side | They are not redundant. `participating=true` is one coarse server-side bundle (roughly "everything except `subscribed`") and is cheaper — fewer pages fetched. `exclude_reasons` is per-reason and exact but only after the full fetch. Neither subsumes the other; do not drop one |
| 11 | When is the tab hidden? | On **capability**, never on emptiness. Hidden only when no configured backend implements `NotificationSource` | An empty inbox is a *result* ("you're clear"), not a reason to remove navigation. Azure-only configs hide the tab in phase 1 and it reappears by itself in phase 2 once Azure implements the interface — no app-layer change needed |
| 12 | Fetch unread-only or everything? | Always `all=true`; `unread_only` filters client-side | Default `GET /notifications` returns unread only, so marking a row read would make it vanish next poll and leave `unread_only: false` with nothing to un-filter |
| 13 | Is read reversible? | No — `u` is **mark read**, one-way, not a toggle | GitHub exposes mark-read (`PATCH`) but no mark-unread endpoint. Encode one-way; the loop has network but no token, so it can confirm the route exists and is auth-gated, not observe behaviour. Flag for manual confirmation, do not claim it verified |
| 14 | Notification identity? | Provider-qualified from day one (kind + scope + native id), never a bare id | Phase 2's Azure keys are derived strings that would collide with GitHub thread ids. This is the same trap that forced the `state.yaml` v1→v2 migration; retrofitting identity is expensive |
| 15 | `Read`/`Done` on the neutral type even though phase 1 has no local state? | Yes, both fields exist in phase 1 | GitHub populates them from the server, Azure from local state in phase 2. Keeping them means phase 2 changes only *who fills them*. Do not "simplify" them away while GitHub is the only source |
| 16 | One disable mechanism or two? | Only `disabled_panes: notifications`; no `notifications.enabled` key | Every default-on pane already disables via `disabled_panes`; `metrics.enabled` exists because metrics is opt-in. Two knobs for one behaviour invites the state where they disagree |
| 17 | Missing `notifications` token scope? | Dedicated in-**view** error state naming the required scope, plus an in-view action to disable the pane by writing `disabled_panes` via `Config.Save()` | A 403 here is a configuration problem, not a transient failure — retry-and-toast would nag forever. The user gets exactly two exits: fix the token, or turn the pane off. The disable action is offered **only** from this error state; when the pane works, `disabled_panes` in the config file is the documented route and needs no TUI affordance |

| 18 | Neutral reason vocabulary — settled here, **not** left to the implementer | `Unknown`, `ReviewRequested`, `Mentioned`, `Assigned`, `Authored`, `Commented`, `StateChanged`, `CIActivity`, `SecurityAlert`, `ApprovalRequested`, `Subscribed`, `Other`. Collapses: `team_mention`→Mentioned, `security_advisory_credit`→SecurityAlert, `manual`/`invitation`/`member_feature_requested`→Other. **Any unrecognised reason maps to `Other` and is never dropped** | Tasks 5, 10 and 11 all consume this enum, so an implementer guess would force three reworks. Collapsing is by triage action: nobody triages a team mention differently from a direct one. Dropping unknown reasons would silently hide notifications, which is the one failure a triage pane cannot have |
| 19 | `exclude_reasons` keys — GitHub strings or neutral names? | Neutral enum names, lowercase snake_case (`subscribed`, `ci_activity`, `security_alert`) | The config must survive phase 2 unchanged. Neutral names happen to read the same as GitHub's for the common cases, so nothing looks odd |
| 20 | Composite partial failure? | One backend erroring keeps the other backends' rows and surfaces a per-backend error; it never empties the feed | A blank attention pane is indistinguishable from "you're clear" — the worst possible failure mode here. Resolves a phase-2 open question early |
| 21 | Badge counts unread before or after filters? | After. Hidden entirely at zero | Counting rows the user's own config deliberately hides would nag about things they chose not to see |
| 22 | Is notifications-only a valid config? | Yes — the "at least one pane" guard must accept it | It is a genuinely useful standalone pane. The current guard only checks PR/work-items/pipelines and would reject it |

## Tasks

- [ ] 1. ADR `docs/adr/0001-notifications-capability-interface.md` — decisions 1, 2, 5. → done: file exists, ≤30 lines, `Status: Accepted`, has Context/Decision/Alternatives/Consequences
- [ ] 2. `provider`: `Notification` type (provider-qualified identity + `Read`/`Done` per decisions 14, 15), `NotificationReason` enum (exactly decision 18's values), `NotifOpts`, `NotificationSource` (blocked by: 1). → done: `go build ./...` clean, `gofmt -l` empty, enum values match decision 18 one-for-one
- [ ] 3. `ui/display`: reason → glyph + label + style map (blocked by: 2). → done: every enum value returns non-empty glyph, label and named style; an unrecognised value renders as `Other`, never empty; table test covers all values plus one unrecognised input; asserts glyph *and* style, not label substrings (convention 6)
- [ ] 4. `github`: user-scoped client — `GET /notifications` with `all=true` (decision 12), pagination, `If-Modified-Since`, `X-Poll-Interval` (blocked by: 2). → done: `httptest` tests assert `all=true` in the query, `Link rel=next` followed, `If-Modified-Since` sent when a cached timestamp exists, 304 returns the cached slice unchanged, `X-Poll-Interval` parsed
- [ ] 5. `github`: wire → neutral mapping, reason mapping, `subject.url` → web URL resolution (blocked by: 4). → done: table test maps every reason string in decision 18 plus an invented unknown → `Other`; `subject.url` resolves for pull/issue/release/commit and falls back to the repo URL otherwise; no panic on absent optional fields
- [ ] 6. `github`: mark read (`PATCH /notifications/threads/{id}`) + mark done (`DELETE`) (blocked by: 4). → done: tests assert method and path per call; ids `<= 0` rejected (convention 11) with a negative-input row; one-way read documented in the doc comment, not claimed as API-verified (decision 13)
- [ ] 7. `github`: implement `NotificationSource` on `Adapter` (blocked by: 5,6). → done: compile-time `var _ provider.NotificationSource = (*Adapter)(nil)`; conformance test following `adapter_conformance_test.go`
- [ ] 8. `provider`: composite fan-out, merge/sort, `HasNotifications()` (blocked by: 2). → done: fans out only to backends implementing the interface; `HasNotifications()` false with zero capable backends and true with ≥1; merged output sorted newest-first; per decision 20 a failing backend still returns the others' rows and never empties the feed — test that case explicitly
- [ ] 9. `config`: `notifications` block with decision-19 key names, defaults, validation, `validDisabledPanes` entry, guard accepts notifications-only (decision 22) (blocked by: 2). → done: block loads with documented defaults; keys resolve lowercased (convention 9); `disabled_panes: notifications` validates; a config with only notifications enabled passes `Validate()`
- [ ] 10. Config-driven filter as a pure function (blocked by: 8,9). → done: table tests cover each knob alone, the full precedence chain, the `participating_only` + `exclude_reasons` compose case (decision 10), and that an unrecognised reason is only filtered when `other` is listed explicitly
- [ ] 11. `ui/notifications`: `listview` pane — dynamic repo column, unread emphasis, `f` reason filter (blocked by: 3,10). → done: renders through `View()` after a `WindowSizeMsg` without panic (convention 8); column count equals row-cell count in both single- and multi-repo cases (convention 7); unread rows assert a named style, not a substring (convention 6); the filter-collapse path and cursor survival are asserted (convention 14)
- [ ] 12. `app`: register the tab first, remap number keys, `enabledTabs`, `state.TabID` (decisions 6, 9, 11) (blocked by: 11). → done: notifications is `enabledTabs[0]`; number keys map to the new order; `TabID "notifications"` round-trips through `state.yaml`; the tab is absent when no backend implements the capability, and present-but-empty when one does
- [ ] 13. `app`: the three render states — empty inbox, capability-unsupported, token-scope error (decisions 11, 17) (blocked by: 12). → done: three distinct renders, each asserted by its own test; the empty state reads as "you're clear", never as an error
- [ ] 14. `app`: `u` mark-read (one-way, decision 13) / `d` mark-done (blocked by: 13). → done: `u` issues one mark-read and updates the row optimistically, rolling back on API failure; `d` removes the row and restores it on failure; a poll that returns stale `unread` inside the debounce window does not flicker the row back
- [ ] 15. Polling integration honouring decision 8 (blocked by: 12). → done: cadence is `max(X-Poll-Interval, configured)`; a 304 response leaves the existing list intact rather than clearing it
- [ ] 16. Unread-count footer badge (decision 21) (blocked by: 12). → done: count is unread *after* config filters; the badge is hidden entirely at zero; visible from every tab
- [ ] 17. Help-modal section + `RemoveSection` wiring when the pane is disabled (blocked by: 12). → done: section lists `u`/`d`/`o`/`f`; disabling the pane removes it; the tabs binding line reflects the new order
- [ ] 18. `config`: regression test pinning `Save()` preservation — seed a file containing `metrics:` and `notifications:`, change only the theme, assert both blocks survive with every value intact (blocked by: 9). → done: the new test fails if `ReadInConfig()` is removed from `Save()` (verify by deleting it locally, watching the test fail, restoring it); fixtures go through `LoadFrom(<t.TempDir() path>)`, never a bare `Config` literal (convention 17)
- [ ] 19. Token-scope error state (decision 17) (blocked by: 13,18). → done: a 403 missing-scope response renders in-view naming the `notifications` scope and how to add it; 401-expired and generic failures render differently; the disable action writes `disabled_panes` via `Config.Save()` behind a confirm, on a key that is not `d` or `u`; its test uses a temp-path config (convention 17)
- [ ] 20. Docs: README (required `notifications` scope, upgrade note for existing tokens, how to disable), Architecture.md, config.yaml.example, FAQ (blocked by: 17,19). → done: every config key from decision 19 is documented; the required token scope and the upgrade path for existing tokens are stated
- [ ] 21. Fold phase-1 answers into `20260729-notif-p2-azdo.md` "Inputs from Phase 1" (blocked by: 20). → done: no `TBD` remains in that section

## Unknowns

**No live API observation is possible here.** The environment reaches `api.github.com` but has
no token, so a route's existence and auth-gating can be confirmed while real payloads cannot.
Tasks 4–6 and 15 encode the *documented* shapes and unit-test against fixtures; anything below
marked _(manual)_ needs a human with a real token before merge. Do not report these verified.

- `subject.url` → browser URL for every subject type (PR, issue, release, discussion, commit,
  check suite). Task 5 implements the documented shapes and falls back to the repo URL for
  anything unrecognised; the fallback is the correctness guarantee, not the mapping. _(manual)_
- Whether `X-Poll-Interval` is in practice slow enough to feel stale against the configured
  interval. Ship the `max()` rule from decision 8 and observe later. _(manual)_
- Whether the merged feed needs pagination/virtualisation above `max_items: 100`.
- Read state may be eventually consistent: a poll landing right after a `PATCH` could still
  return `unread`, flickering the optimistic update back. Task 14 holds the local intent until
  the server agrees rather than trusting the first poll that contradicts it.
