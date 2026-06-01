package azdevops

import (
	"fmt"
	"time"
)

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
