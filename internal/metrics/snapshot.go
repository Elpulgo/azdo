package metrics

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
)

// DefaultSnapshotPath returns the standard snapshot file location
// (~/.config/azdo-tui/metrics.jsonl). The parent directory is not created;
// the writer's caller is responsible for that.
func DefaultSnapshotPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".config", "azdo-tui", "metrics.jsonl"), nil
}

// EnsureSnapshotDir creates the directory holding the snapshot file if it
// doesn't already exist. Safe to call repeatedly.
func EnsureSnapshotDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0755)
}

// Snapshot is one observed work item state on a particular calendar day.
// Append-only daily rows form the basis of the Trends view.
type Snapshot struct {
	TS         string   `json:"ts"`         // "YYYY-MM-DD"
	ID         int      `json:"id"`
	Project    string   `json:"project"`    // API project name
	State      string   `json:"state"`
	AssignedTo string   `json:"assignedTo"`
	Points     float64  `json:"points"`
	Tags       []string `json:"tags"`
	Iteration  string   `json:"iteration"`
	StateSince string   `json:"stateSince"` // RFC3339 copy of StateChangeDate
	Source     string   `json:"source"`     // SourceSnapshot or SourceUpdates
}

const (
	// SourceSnapshot marks rows observed live during the daily snapshot run.
	SourceSnapshot = "snapshot"
	// SourceUpdates marks rows synthesized from the /updates revision history,
	// either by the gap-fallback path (PR 2) or the first-launch backfill (PR 3).
	SourceUpdates = "updates"
)

// ReadSnapshots reads a JSONL snapshot file. Returns nil and no error if the
// file does not exist — a fresh install has no snapshot file yet.
// Malformed lines are skipped silently so a single bad write can't bring the
// metrics tab down for everything else.
func ReadSnapshots(path string) ([]Snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open snapshots: %w", err)
	}
	defer f.Close()

	var out []Snapshot
	sc := bufio.NewScanner(f)
	// Allow lines up to 1 MiB; a snapshot row is small but tags can grow.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var s Snapshot
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			continue
		}
		out = append(out, s)
	}
	if err := sc.Err(); err != nil {
		return out, fmt.Errorf("scan snapshots: %w", err)
	}
	return out, nil
}

// AppendSnapshots merges newSnaps into the existing file, deduplicating by
// (TS, ID) where newSnaps wins over existing, and pruning rows older than
// retention measured from now. Writes atomically via a temp file + rename so
// the caller never sees a partially-written file.
func AppendSnapshots(path string, newSnaps []Snapshot, retention time.Duration, now time.Time) error {
	existing, err := ReadSnapshots(path)
	if err != nil {
		return err
	}

	type key struct {
		TS string
		ID int
	}
	merged := make(map[key]Snapshot, len(existing)+len(newSnaps))
	for _, s := range existing {
		merged[key{s.TS, s.ID}] = s
	}
	for _, s := range newSnaps {
		merged[key{s.TS, s.ID}] = s
	}

	cutoff := now.Add(-retention)
	kept := make([]Snapshot, 0, len(merged))
	for _, s := range merged {
		d, err := time.Parse("2006-01-02", s.TS)
		if err != nil {
			continue
		}
		if d.Before(cutoff) {
			continue
		}
		kept = append(kept, s)
	}

	sort.Slice(kept, func(i, j int) bool {
		if kept[i].TS != kept[j].TS {
			return kept[i].TS < kept[j].TS
		}
		return kept[i].ID < kept[j].ID
	})

	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create snapshot tmp: %w", err)
	}
	enc := json.NewEncoder(f)
	for _, s := range kept {
		if err := enc.Encode(s); err != nil {
			f.Close()
			os.Remove(tmp)
			return fmt.Errorf("encode snapshot: %w", err)
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close snapshot tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename snapshot tmp: %w", err)
	}
	return nil
}

// BuildSnapshots converts the current live work-item fetch into today's
// snapshot rows. Tagged with Source="snapshot" so the Trends layer can
// distinguish observed-today rows from backfill-synthesized rows later.
func BuildSnapshots(items []azdevops.WorkItem, now time.Time) []Snapshot {
	today := now.Format("2006-01-02")
	out := make([]Snapshot, 0, len(items))
	for i := range items {
		wi := items[i]
		out = append(out, Snapshot{
			TS:         today,
			ID:         wi.ID,
			Project:    wi.ProjectName,
			State:      wi.Fields.State,
			AssignedTo: wi.AssignedToName(),
			Points:     wi.EffectivePoints(),
			Tags:       wi.TagList(),
			Iteration:  wi.Fields.IterationPath,
			StateSince: rfc3339Or(wi.Fields.StateChangeDate),
			Source:     SourceSnapshot,
		})
	}
	return out
}

// LatestPerItem returns the most recent snapshot row for each item ID, with
// "most recent" defined by TS lexicographic order (YYYY-MM-DD sorts correctly).
// Used by the gap-fallback path to compare today's observation against the
// last known state.
func LatestPerItem(snaps []Snapshot) map[int]Snapshot {
	out := make(map[int]Snapshot)
	for _, s := range snaps {
		cur, ok := out[s.ID]
		if !ok || s.TS > cur.TS {
			out[s.ID] = s
		}
	}
	return out
}

// HasSnapshotForToday reports whether the snapshot file already contains a row
// dated `today` — i.e. whether the per-day snapshot run has already happened.
// Used to short-circuit the snapshot write when the tab is opened multiple
// times the same day.
func HasSnapshotForToday(snaps []Snapshot, today string) bool {
	for _, s := range snaps {
		if s.TS == today {
			return true
		}
	}
	return false
}

func rfc3339Or(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
