package azdevops

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// WorkItemStateTransition is one state change on a work item, extracted from
// the /updates revision history. Sorted ascending by `At` by the time it
// reaches the caller.
type WorkItemStateTransition struct {
	State string
	At    time.Time
}

// workItemUpdate is the shape of one entry in the /updates response. Only the
// fields we care about (System.State change + System.ChangedDate timestamp)
// are modeled; the rest of the revision payload is ignored.
type workItemUpdate struct {
	Fields map[string]workItemFieldChange `json:"fields"`
}

type workItemFieldChange struct {
	NewValue any `json:"newValue"`
}

type workItemUpdatesResponse struct {
	Value []workItemUpdate `json:"value"`
}

// MetricsWorkItems fetches every in-flight item (Active / Ready for Test) for
// the project plus items closed on or after `since`, with the metrics fields
// populated. Unlike ListWorkItems this query is org-wide (no @Me filter) and
// not capped at 50 — it powers the management/metrics view.
//
// The WIQL excludes the New state by construction: New items are backlog,
// nobody is working them, and they would only add noise to the dashboard.
func (c *Client) MetricsWorkItems(since time.Time) ([]WorkItem, error) {
	sinceStr := since.Format("2006-01-02")
	query := fmt.Sprintf(`SELECT [System.Id] FROM WorkItems
WHERE [System.TeamProject] = @project
  AND (
        [System.State] IN ('Active','Ready for Test')
     OR ([System.State] = 'Closed' AND [Microsoft.VSTS.Common.ClosedDate] >= '%s')
  )
ORDER BY [System.ChangedDate] DESC`, sinceStr)

	ids, err := c.QueryWorkItemIDs(query, 2000)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []WorkItem{}, nil
	}
	return c.getWorkItemsBatched(ids)
}

// WorkItemUpdates fetches the revision history for a single work item and
// returns the chronological list of state changes. Used by the snapshot
// gap-fallback path (and, in PR 3, the one-shot 90-day backfill) — never on
// every poll.
func (c *Client) WorkItemUpdates(id int) ([]WorkItemStateTransition, error) {
	path := fmt.Sprintf("/wit/workItems/%d/updates?api-version=7.1", id)
	body, err := c.get(path)
	if err != nil {
		return nil, fmt.Errorf("fetch updates for %d: %w", id, err)
	}
	var resp workItemUpdatesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse updates for %d: %w", id, err)
	}
	return parseStateTransitions(resp.Value), nil
}

// parseStateTransitions extracts state-change events from raw /updates
// payloads. Pure helper, table-tested without HTTP.
func parseStateTransitions(updates []workItemUpdate) []WorkItemStateTransition {
	var out []WorkItemStateTransition
	for _, u := range updates {
		stateChange, ok := u.Fields["System.State"]
		if !ok {
			continue
		}
		newState := asString(stateChange.NewValue)
		if newState == "" {
			continue
		}
		var at time.Time
		if cd, ok := u.Fields["System.ChangedDate"]; ok {
			at, _ = time.Parse(time.RFC3339, asString(cd.NewValue))
		}
		if at.IsZero() {
			continue
		}
		out = append(out, WorkItemStateTransition{State: newState, At: at})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

// getWorkItemsBatched fans GetWorkItems calls out in batches of 200, the
// Azure DevOps per-request cap. Returns the concatenated result; on a batch
// error returns whatever was collected so far alongside a wrapped error.
func (c *Client) getWorkItemsBatched(ids []int) ([]WorkItem, error) {
	const batch = 200
	all := make([]WorkItem, 0, len(ids))
	for i := 0; i < len(ids); i += batch {
		end := i + batch
		if end > len(ids) {
			end = len(ids)
		}
		items, err := c.GetWorkItems(ids[i:end])
		if err != nil {
			return all, fmt.Errorf("metrics batch %d-%d: %w", i, end, err)
		}
		all = append(all, items...)
	}
	return all, nil
}
