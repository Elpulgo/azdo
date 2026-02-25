package azdevops

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// WorkItemTypeState represents a state available for a work item type
type WorkItemTypeState struct {
	Name     string `json:"name"`
	Color    string `json:"color"`
	Category string `json:"category"`
}

// WorkItemTypeStatesResponse represents the response from the work item type states API
type WorkItemTypeStatesResponse struct {
	Count int                 `json:"count"`
	Value []WorkItemTypeState `json:"value"`
}

// WorkItem represents a work item in Azure DevOps
type WorkItem struct {
	ID          int            `json:"id"`
	Rev         int            `json:"rev"`
	Fields      WorkItemFields `json:"fields"`
	URL         string         `json:"url"`
	ProjectName string         `json:"-"` // Set by MultiClient, not from API
}

// WorkItemFields represents the fields of a work item
type WorkItemFields struct {
	Title         string    `json:"System.Title"`
	State         string    `json:"System.State"`
	WorkItemType  string    `json:"System.WorkItemType"`
	AssignedTo    *Identity `json:"System.AssignedTo"`
	Priority      int       `json:"Microsoft.VSTS.Common.Priority"`
	ChangedDate   time.Time `json:"System.ChangedDate"`
	IterationPath string    `json:"System.IterationPath"`
	Description   string    `json:"System.Description"`
	ReproSteps    string    `json:"Microsoft.VSTS.TCM.ReproSteps"`
}

// WorkItemReference represents a reference to a work item from WIQL queries
type WorkItemReference struct {
	ID  int    `json:"id"`
	URL string `json:"url"`
}

// WIQLResponse represents the response from a WIQL query
type WIQLResponse struct {
	WorkItems []WorkItemReference `json:"workItems"`
}

// WorkItemsResponse represents the response from getting work items
type WorkItemsResponse struct {
	Count int        `json:"count"`
	Value []WorkItem `json:"value"`
}

// StateIcon returns an icon for the work item state
// Workflow: New → Active → Resolved/Ready for Test → Closed
func (wi *WorkItem) StateIcon() string {
	stateLower := strings.ToLower(wi.Fields.State)

	switch {
	case stateLower == "new":
		return "○"
	case stateLower == "active":
		return "◐"
	case stateLower == "resolved" || strings.Contains(stateLower, "ready"):
		return "●"
	case stateLower == "closed":
		return "✓"
	case stateLower == "removed":
		return "✗"
	default:
		return "○"
	}
}

// EffectiveDescription returns the appropriate description field based on work item type.
// Bugs use Microsoft.VSTS.TCM.ReproSteps; other types use System.Description.
func (wi *WorkItem) EffectiveDescription() string {
	if wi.Fields.WorkItemType == "Bug" && wi.Fields.ReproSteps != "" {
		return wi.Fields.ReproSteps
	}
	return wi.Fields.Description
}

// AssignedToName returns the display name of the assigned user, or "-" if unassigned
func (wi *WorkItem) AssignedToName() string {
	if wi.Fields.AssignedTo == nil {
		return "-"
	}
	return wi.Fields.AssignedTo.DisplayName
}

// QueryWorkItemIDs executes a WIQL query and returns the work item IDs
// top: maximum number of results to return
func (c *Client) QueryWorkItemIDs(query string, top int) ([]int, error) {
	path := fmt.Sprintf("/wit/wiql?api-version=7.1&$top=%d", top)

	payload := fmt.Sprintf(`{"query": %s}`, escapeJSONString(query))
	body, err := c.post(path, strings.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to execute WIQL query: %w", err)
	}

	var response WIQLResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse Azure DevOps API response for work item query: %w. "+
			"This may indicate an API structure change. Please check for updates or report this issue", err)
	}

	ids := make([]int, len(response.WorkItems))
	for i, wi := range response.WorkItems {
		ids[i] = wi.ID
	}

	return ids, nil
}

// GetWorkItems retrieves work items by their IDs
// Azure DevOps supports up to 200 IDs per request
func (c *Client) GetWorkItems(ids []int) ([]WorkItem, error) {
	if len(ids) == 0 {
		return []WorkItem{}, nil
	}

	// Convert IDs to comma-separated string
	idStrs := make([]string, len(ids))
	for i, id := range ids {
		idStrs[i] = strconv.Itoa(id)
	}
	idsParam := strings.Join(idStrs, ",")

	// Specify fields to retrieve
	fields := strings.Join([]string{
		"System.Id",
		"System.Title",
		"System.State",
		"System.WorkItemType",
		"System.AssignedTo",
		"Microsoft.VSTS.Common.Priority",
		"System.ChangedDate",
		"System.IterationPath",
		"System.Description",
		"Microsoft.VSTS.TCM.ReproSteps",
	}, ",")

	path := fmt.Sprintf("/wit/workitems?ids=%s&fields=%s&api-version=7.1", idsParam, fields)

	body, err := c.get(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get work items: %w", err)
	}

	var response WorkItemsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse Azure DevOps API response for work items: %w. "+
			"This may indicate an API structure change. Please check for updates or report this issue", err)
	}

	return response.Value, nil
}

// ListWorkItems retrieves work items assigned to the current user
// top: maximum number of work items to return (max 50 enforced)
func (c *Client) ListWorkItems(top int) ([]WorkItem, error) {
	// Enforce cap at 50
	if top > 50 {
		top = 50
	}

	// WIQL query to get active work items assigned to current user
	// @project scopes to the project in the API URL context, preventing
	// duplicates when multiple project clients query simultaneously
	query := `SELECT [System.Id] FROM WorkItems
WHERE [System.TeamProject] = @project
  AND [System.State] <> 'Closed'
  AND [System.State] <> 'Removed'
ORDER BY [System.ChangedDate] DESC`

	ids, err := c.QueryWorkItemIDs(query, top)
	if err != nil {
		return nil, err
	}

	if len(ids) == 0 {
		return []WorkItem{}, nil
	}

	return c.GetWorkItems(ids)
}

// ListMyWorkItems retrieves work items assigned to the authenticated user
// using the @Me WIQL macro, which Azure DevOps resolves server-side from the PAT.
// top: maximum number of work items to return (max 50 enforced)
func (c *Client) ListMyWorkItems(top int) ([]WorkItem, error) {
	if top > 50 {
		top = 50
	}

	query := `SELECT [System.Id] FROM WorkItems
WHERE [System.TeamProject] = @project
  AND [System.AssignedTo] = @Me
  AND [System.State] <> 'Closed'
  AND [System.State] <> 'Removed'
ORDER BY [System.ChangedDate] DESC`

	ids, err := c.QueryWorkItemIDs(query, top)
	if err != nil {
		return nil, err
	}

	if len(ids) == 0 {
		return []WorkItem{}, nil
	}

	return c.GetWorkItems(ids)
}

// GetWorkItemTypeStates retrieves the available states for a work item type.
// States in the "Removed" category are excluded since they are not typical user transitions.
func (c *Client) GetWorkItemTypeStates(workItemType string) ([]WorkItemTypeState, error) {
	path := fmt.Sprintf("/wit/workitemtypes/%s/states?api-version=7.1", workItemType)

	body, err := c.get(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get work item type states: %w", err)
	}

	var response WorkItemTypeStatesResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse work item type states response: %w", err)
	}

	// Filter out "Removed" category states
	states := make([]WorkItemTypeState, 0, len(response.Value))
	for _, s := range response.Value {
		if s.Category != "Removed" {
			states = append(states, s)
		}
	}

	return states, nil
}

// UpdateWorkItemState updates the state of a work item using JSON Patch.
func (c *Client) UpdateWorkItemState(id int, state string) error {
	path := fmt.Sprintf("/wit/workitems/%d?api-version=7.1", id)

	payload := fmt.Sprintf(`[{"op":"replace","path":"/fields/System.State","value":%s}]`, escapeJSONString(state))
	_, err := c.doRequestWithContentType("PATCH", path, strings.NewReader(payload), "application/json-patch+json")
	if err != nil {
		return fmt.Errorf("failed to update work item state: %w", err)
	}

	return nil
}
