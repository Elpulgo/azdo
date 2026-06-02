package metrics

import (
	"sync"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	coremetrics "github.com/Elpulgo/azdo/internal/metrics"
	tea "github.com/charmbracelet/bubbletea"
)

// backfillWindowDays is the lookback the one-shot backfill synthesizes, which
// must match the snapshot file retention so the appended rows aren't pruned on
// the very same call.
const backfillWindowDays = 90

// backfillConcurrency caps parallel /updates fetches during the one-shot
// backfill. Mirrors the gap-fallback cap (4) — the same external API, same
// bounded-parallel discipline.
const backfillConcurrency = 4

// backfillDoneMsg signals the result of the one-shot backfill attempt.
// Mutually exclusive flags: `err` on failure; `alreadyDone` when the marker
// was already present; `saved` / `skipped` on a real run.
type backfillDoneMsg struct {
	total       int
	saved       int
	skipped     int
	alreadyDone bool
	err         error
}

// backfillFetcher is the dependency-injection seam for unit tests. The
// production wrapper calls client.ClientFor(project).WorkItemUpdates(id).
type backfillFetcher func(project string, id int) ([]azdevops.WorkItemStateTransition, error)

// buildBackfillRows fans /updates fetches across `items` with bounded
// concurrency and synthesizes per-day snapshot rows back to (now - 90 days).
// Returns the union of all synthesized rows plus a count of items whose
// fetch failed (other items still produce rows — partial success is fine).
//
// Pure-ish: HTTP injection via `fetch`, no file I/O. Caller persists.
func buildBackfillRows(
	items []azdevops.WorkItem,
	fetch backfillFetcher,
	now time.Time,
	concurrency int,
) ([]coremetrics.Snapshot, int) {
	if len(items) == 0 {
		return nil, 0
	}
	if concurrency < 1 {
		concurrency = 1
	}

	type result struct {
		rows    []coremetrics.Snapshot
		skipped bool
	}
	results := make([]result, len(items))

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	since := now.AddDate(0, 0, -backfillWindowDays)

	for i := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, wi azdevops.WorkItem) {
			defer wg.Done()
			defer func() { <-sem }()

			txns, err := fetch(wi.ProjectName, wi.ID)
			if err != nil {
				results[i] = result{skipped: true}
				return
			}
			template := snapshotTemplate(wi)
			rows := coremetrics.SynthesizeGapRows(
				coremetrics.FromAzDevTransitions(txns),
				since, now, template,
			)
			results[i] = result{rows: rows}
		}(i, items[i])
	}
	wg.Wait()

	var all []coremetrics.Snapshot
	skipped := 0
	for _, r := range results {
		if r.skipped {
			skipped++
			continue
		}
		all = append(all, r.rows...)
	}
	return all, skipped
}

// snapshotTemplate builds the per-item static fields used as the row template
// passed to SynthesizeGapRows. TS, State, StateSince, Source are overwritten
// per-row by the synthesizer.
func snapshotTemplate(wi azdevops.WorkItem) coremetrics.Snapshot {
	return coremetrics.Snapshot{
		ID:         wi.ID,
		Project:    wi.ProjectName,
		AssignedTo: wi.AssignedToName(),
		Points:     wi.EffectivePoints(),
		Tags:       wi.TagList(),
		Iteration:  wi.Fields.IterationPath,
		Source:     coremetrics.SourceUpdates,
	}
}

// runBackfillCmd is the tea.Cmd entry point for the one-shot backfill. Checks
// the marker (cheap stat) and short-circuits if already done; otherwise
// fetches candidate items via MetricsWorkItems(now-90d), fans /updates fetches
// out with bounded concurrency, appends the synthesized rows, and writes the
// marker. Errors leave the marker untouched so the next launch retries.
func runBackfillCmd(client *azdevops.MultiClient, now time.Time) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return backfillDoneMsg{}
		}
		markerPath, err := coremetrics.DefaultBackfillMarkerPath()
		if err != nil {
			return backfillDoneMsg{err: err}
		}
		done, err := coremetrics.BackfillAlreadyDone(markerPath)
		if err != nil {
			return backfillDoneMsg{err: err}
		}
		if done {
			return backfillDoneMsg{alreadyDone: true}
		}

		since := now.AddDate(0, 0, -backfillWindowDays)
		items, err := client.MetricsWorkItems(since)
		if err != nil {
			return backfillDoneMsg{err: err}
		}

		fetch := func(project string, id int) ([]azdevops.WorkItemStateTransition, error) {
			cli := client.ClientFor(project)
			if cli == nil {
				return nil, errBackfillNoClient
			}
			return cli.WorkItemUpdates(id)
		}

		rows, skipped := buildBackfillRows(items, fetch, now, backfillConcurrency)

		if len(rows) > 0 {
			path, err := coremetrics.DefaultSnapshotPath()
			if err != nil {
				return backfillDoneMsg{err: err}
			}
			if err := coremetrics.EnsureSnapshotDir(path); err != nil {
				return backfillDoneMsg{err: err}
			}
			if err := coremetrics.AppendSnapshots(path, rows, backfillWindowDays*24*time.Hour, now); err != nil {
				return backfillDoneMsg{err: err}
			}
		}

		if err := coremetrics.MarkBackfillDone(markerPath); err != nil {
			return backfillDoneMsg{total: len(items), saved: len(rows), skipped: skipped, err: err}
		}
		return backfillDoneMsg{total: len(items), saved: len(rows), skipped: skipped}
	}
}

// errBackfillNoClient is the sentinel a fetch closure returns when the
// MultiClient has no client for the requested project (a misconfiguration).
// Treated as a per-item skip, not a fatal failure.
var errBackfillNoClient = backfillError("no client for project")

type backfillError string

func (e backfillError) Error() string { return string(e) }
