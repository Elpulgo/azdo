package metrics

import (
	"sync"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	coremetrics "github.com/Elpulgo/azdo/internal/metrics"
	tea "github.com/charmbracelet/bubbletea"
)

// snapshotSavedMsg arrives after the daily snapshot-write attempt completes.
// Mutually exclusive flags describe what happened: success (`saved`),
// already-saved-today (`alreadyToday`), or failure (`err`).
type snapshotSavedMsg struct {
	saved        int
	skipped      int
	alreadyToday bool
	err          error
}

// snapshotsLoadedMsg carries the snapshot file's contents plus the user's
// persisted sprint selection. Fired on Init and after every successful
// snapshot write so Trends reflects the freshest data.
type snapshotsLoadedMsg struct {
	snaps    []coremetrics.Snapshot
	selected []string
	err      error
}

// loadSnapshotsCmd reads the snapshot JSONL and the persisted sprint
// selection. Tolerates missing files (returns empty results).
func loadSnapshotsCmd() tea.Cmd {
	return func() tea.Msg {
		snapPath, err := coremetrics.DefaultSnapshotPath()
		if err != nil {
			return snapshotsLoadedMsg{err: err}
		}
		snaps, err := coremetrics.ReadSnapshots(snapPath)
		if err != nil {
			return snapshotsLoadedMsg{err: err}
		}
		selPath, err := coremetrics.DefaultSelectionPath()
		if err != nil {
			return snapshotsLoadedMsg{snaps: snaps}
		}
		selected, _ := coremetrics.LoadSelection(selPath) // missing/bad → ignore
		return snapshotsLoadedMsg{snaps: snaps, selected: selected}
	}
}

// saveSelectionCmd persists the chosen sprint tags to disk. Fire-and-forget;
// errors are swallowed because the in-memory selection is the source of truth
// for the current session anyway.
func saveSelectionCmd(sprints []string) tea.Cmd {
	chosen := append([]string(nil), sprints...)
	return func() tea.Msg {
		path, err := coremetrics.DefaultSelectionPath()
		if err != nil {
			return nil
		}
		_ = coremetrics.SaveSelection(path, chosen)
		return nil
	}
}

// gapFallbackConcurrency caps the per-item /updates calls fired during a
// gap-fallback sweep. The path is rare (only when an item skipped a state
// between snapshots), but bounding parallelism keeps a bad day predictable.
const gapFallbackConcurrency = 4

// saveSnapshotCmd persists today's snapshot rows for `items`, runs gap-fallback
// via /updates for items whose state jumped or moved backward, prunes the file
// to 90 days, and returns a snapshotSavedMsg with counts.
//
// Pure-ish orchestration: all snapshot file I/O, all /updates fetches. Safe to
// call as a tea.Cmd from the metrics view's Update; the goroutine returns the
// message back into the bubbletea loop on completion.
func saveSnapshotCmd(client *azdevops.MultiClient, items []azdevops.WorkItem, now time.Time, states coremetrics.StateConfig) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return snapshotSavedMsg{}
		}
		path, err := coremetrics.DefaultSnapshotPath()
		if err != nil {
			return snapshotSavedMsg{err: err}
		}
		if err := coremetrics.EnsureSnapshotDir(path); err != nil {
			return snapshotSavedMsg{err: err}
		}

		existing, err := coremetrics.ReadSnapshots(path)
		if err != nil {
			return snapshotSavedMsg{err: err}
		}

		today := now.Format("2006-01-02")
		if coremetrics.HasSnapshotForToday(existing, today) {
			return snapshotSavedMsg{alreadyToday: true}
		}

		todaySnaps := coremetrics.BuildSnapshots(items, now)
		latest := coremetrics.LatestPerItem(existing)

		gapRows, skipped := runGapFallback(client, todaySnaps, latest, now, states)

		all := append(gapRows, todaySnaps...)
		const retention = 90 * 24 * time.Hour
		if err := coremetrics.AppendSnapshots(path, all, retention, now); err != nil {
			return snapshotSavedMsg{err: err}
		}
		return snapshotSavedMsg{saved: len(all), skipped: skipped}
	}
}

// runGapFallback identifies items whose new state can't be reconciled with the
// previous snapshot row in a single legal transition, fires /updates for each,
// and synthesizes the missing intermediate daily rows. Returns the synthesized
// rows plus a count of items the fallback couldn't recover (network blip,
// missing project client, etc.).
func runGapFallback(client *azdevops.MultiClient, todaySnaps []coremetrics.Snapshot, latest map[int]coremetrics.Snapshot, now time.Time, states coremetrics.StateConfig) ([]coremetrics.Snapshot, int) {
	type req struct {
		today coremetrics.Snapshot
		prev  coremetrics.Snapshot
	}
	var needs []req
	for _, snap := range todaySnaps {
		prev := latest[snap.ID]
		if coremetrics.ClassifyTransition(prev.State, snap.State, states) == coremetrics.GapNeedsFallback {
			needs = append(needs, req{today: snap, prev: prev})
		}
	}
	if len(needs) == 0 {
		return nil, 0
	}

	type result struct {
		rows    []coremetrics.Snapshot
		skipped bool
	}
	results := make([]result, len(needs))

	var wg sync.WaitGroup
	sem := make(chan struct{}, gapFallbackConcurrency)
	for i, n := range needs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, n req) {
			defer wg.Done()
			defer func() { <-sem }()

			cli := client.ClientFor(n.today.Project)
			if cli == nil {
				results[i] = result{skipped: true}
				return
			}
			txns, err := cli.WorkItemUpdates(n.today.ID)
			if err != nil {
				results[i] = result{skipped: true}
				return
			}

			prevDate, perr := time.Parse("2006-01-02", n.prev.TS)
			if perr != nil {
				// No usable previous row — backfill the full window the
				// caller has visibility into. Cap at 90 days to match
				// retention.
				prevDate = now.AddDate(0, 0, -90)
			}
			template := n.today
			template.TS = ""
			template.State = ""
			template.Source = coremetrics.SourceUpdates
			rows := coremetrics.SynthesizeGapRows(
				coremetrics.FromAzDevTransitions(txns),
				prevDate, now, template,
			)
			results[i] = result{rows: rows}
		}(i, n)
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
