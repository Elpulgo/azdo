package workitems

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/provider"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/components/listview"
	"github.com/Elpulgo/azdo/internal/ui/components/table"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ViewMode re-exports listview.ViewMode for backward compatibility.
type ViewMode = listview.ViewMode

const (
	ViewList   = listview.ViewList
	ViewDetail = listview.ViewDetail
)

// TagSelectedMsg is sent when a tag is selected from the tag picker.
// An empty Tag means "clear filter".
type TagSelectedMsg = components.TagSelectedMsg

// Model represents the work items list view with sub-views
type Model struct {
	list        listview.Model[provider.WorkItem]
	client      provider.Provider
	styles      *styles.Styles
	myItemsOnly bool
	allItems    []provider.WorkItem
	myItems     []provider.WorkItem // base my-items set (before tag/state filter)
	activeTag   string
	activeState string
	tagPicker   components.TagPicker
	statePicker components.ListPicker

	// pendingDetailID is the work-item ID requested by startup state
	// restore. Cleared on first populate so polling can't re-trigger it.
	pendingDetailID       int
	pendingRestoreHandled bool
}

// NewModel creates a new work items list model with default styles
func NewModel(client provider.Provider) Model {
	return NewModelWithStyles(client, styles.DefaultStyles())
}

// NewModelWithStyles creates a new work items list model with custom styles
func NewModelWithStyles(client provider.Provider, s *styles.Styles) Model {
	isMulti := client != nil && client.IsMultiProject()

	columns := []listview.ColumnSpec{
		{Title: "Type", WidthPct: 10, MinWidth: 8},
		{Title: "ID", WidthPct: 8, MinWidth: 6},
		{Title: "Title", WidthPct: 40, MinWidth: 25},
		{Title: "State", WidthPct: 10, MinWidth: 10},
		{Title: "Prio", WidthPct: 6, MinWidth: 4},
		{Title: "Assigned", WidthPct: 26, MinWidth: 10},
	}

	if isMulti {
		columns = append(
			[]listview.ColumnSpec{{Title: "Project", WidthPct: 10, MinWidth: 8}},
			columns...,
		)
	}

	listview.NormalizeWidths(columns)

	toRows := workItemsToRows
	if isMulti {
		toRows = workItemsToRowsMulti
	}

	filterFunc := filterWorkItem
	if isMulti {
		filterFunc = filterWorkItemMulti
	}

	cfg := listview.Config[provider.WorkItem]{
		Columns:        columns,
		LoadingMessage: "Loading work items...",
		EntityName:     "work items",
		MinWidth:       50,
		ToRows:         toRows,
		Fetch: func() tea.Cmd {
			return fetchWorkItems(client)
		},
		EnterDetail: func(item provider.WorkItem, st *styles.Styles, w, h int) (listview.DetailView, tea.Cmd) {
			d := NewDetailModelWithStyles(client, item, st)
			d.SetSize(w, h)
			// d.Init() kicks off the comment fetch so the Discussion section
			// populates as soon as the detail view opens.
			return &detailAdapter{d}, d.Init()
		},
		HasContextBar: func(mode listview.ViewMode) bool {
			return mode == listview.ViewDetail
		},
		FilterFunc: filterFunc,
	}

	return Model{
		list:        listview.New(cfg, s),
		client:      client,
		styles:      s,
		tagPicker:   components.NewTagPicker(s),
		statePicker: components.NewListPicker(s),
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return m.list.Init()
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case workItemsMsg:
		if msg.err != nil {
			// For partial errors, treat data as valid (some projects succeeded)
			var partialErr *azdevops.PartialError
			if errors.As(msg.err, &partialErr) {
				m.allItems = msg.workItems
				if m.myItemsOnly {
					return m, fetchMyWorkItems(m.client)
				}
				m.list = m.list.HandleFetchResult(msg.workItems, nil)
				return m.withRestore(nil)
			}

			criticalCmd := components.NewCriticalErrorCmd(msg.err)
			if criticalCmd != nil {
				// Critical errors are shown via the error modal; don't display inline
				m.list = m.list.HandleFetchResult(nil, nil)
				return m, criticalCmd
			}
			m.list = m.list.HandleFetchResult(msg.workItems, msg.err)
			return m.withRestore(nil)
		}
		m.allItems = msg.workItems
		if m.myItemsOnly {
			// Chain to my-items fetch so loading state is eventually cleared
			return m, fetchMyWorkItems(m.client)
		}
		m.list = m.list.HandleFetchResult(msg.workItems, nil)
		return m.withRestore(nil)
	case myWorkItemsMsg:
		if msg.err != nil {
			// For partial errors, use partial data as valid
			var partialErr *azdevops.PartialError
			if errors.As(msg.err, &partialErr) {
				m.myItems = msg.workItems
				m.list = m.list.SetItems(m.applyAllFilters(msg.workItems))
				return m.withRestore(nil)
			}
			// On error, fall back to showing all items and clear loading state
			m.myItemsOnly = false
			m.myItems = nil
			m.list = m.list.SetItems(m.applyAllFilters(m.allItems))
			return m.withRestore(nil)
		}
		m.myItems = msg.workItems
		m.list = m.list.SetItems(m.applyAllFilters(msg.workItems))
		return m.withRestore(nil)
	case WorkItemStateChangedMsg:
		// Re-fetch work items so the list reflects the updated state
		return m, fetchWorkItems(m.client)
	case SetWorkItemsMsg:
		m.allItems = msg.WorkItems
		if !m.myItemsOnly {
			m.list = m.list.SetItems(m.applyAllFilters(msg.WorkItems))
			return m.withRestore(nil)
		}
		return m, nil
	case components.TagSelectedMsg:
		m.activeTag = msg.Tag
		m.tagPicker.Hide()
		// Re-apply filters on the appropriate base set
		m.list = m.list.SetItems(m.applyAllFilters(m.getBaseItems()))
		return m, nil
	case components.ListPickerSelectedMsg:
		m.activeState = msg.Value
		m.statePicker.Hide()
		// Re-apply filters on the appropriate base set
		m.list = m.list.SetItems(m.applyAllFilters(m.getBaseItems()))
		return m, nil
	case tea.KeyMsg:
		// When a picker modal is open, forward all keystrokes to it below so
		// characters like T/m/s can be typed into the picker's search input.
		pickerOpen := m.tagPicker.IsVisible() || m.statePicker.IsVisible()
		if !pickerOpen {
			if msg.String() == "T" && !m.list.IsSearching() && m.GetViewMode() == ViewList {
				tags := collectUniqueTags(m.allItems)
				m.tagPicker.SetTags(tags, m.activeTag)
				m.tagPicker.Show()
				return m, nil
			}
			if msg.String() == "m" && !m.list.IsSearching() && m.GetViewMode() == ViewList {
				m.myItemsOnly = !m.myItemsOnly
				if m.myItemsOnly {
					return m, fetchMyWorkItems(m.client)
				}
				// Toggle OFF: restore all items (with filters if active)
				m.myItems = nil
				m.list = m.list.SetItems(m.applyAllFilters(m.allItems))
				return m, nil
			}
			// esc clears an active "my items" filter, mirroring how esc exits
			// search. It only ever turns the filter OFF — never on. When
			// searching, esc is left to exit search first.
			if msg.String() == "esc" && !m.list.IsSearching() && m.GetViewMode() == ViewList && m.myItemsOnly {
				m.myItemsOnly = false
				m.myItems = nil
				m.list = m.list.SetItems(m.applyAllFilters(m.allItems))
				return m, nil
			}
			if msg.String() == "s" && !m.list.IsSearching() && m.GetViewMode() == ViewList {
				states := collectUniqueStates(m.allItems)
				options := make([]components.ListPickerOption, len(states))
				for i, state := range states {
					options[i] = components.ListPickerOption{Name: state, Icon: "●"}
				}
				m.statePicker.SetConfig("Filter by State", options, m.activeState, true)
				m.statePicker.Show()
				return m, nil
			}
		}
	}

	// When tag picker is visible, route all input to it
	if m.tagPicker.IsVisible() {
		if kmsg, ok := msg.(tea.KeyMsg); ok {
			var cmd tea.Cmd
			m.tagPicker, cmd = m.tagPicker.Update(kmsg)
			return m, cmd
		}
		return m, nil
	}

	// When state picker is visible, route all input to it
	if m.statePicker.IsVisible() {
		if kmsg, ok := msg.(tea.KeyMsg); ok {
			var cmd tea.Cmd
			m.statePicker, cmd = m.statePicker.Update(kmsg)
			return m, cmd
		}
		return m, nil
	}

	// When in detail view, intercept esc to check for modals first
	if m.GetViewMode() == ViewDetail {
		if kmsg, ok := msg.(tea.KeyMsg); ok && kmsg.String() == "esc" {
			// If the detail view has a modal/form open (state picker or comment
			// form), route esc directly to the detail model to close it, bypassing
			// the listview which would otherwise close the entire detail view.
			if adapter, ok := m.list.Detail().(*detailAdapter); ok {
				if adapter.model.statePicker.IsVisible() || adapter.model.commentForm.IsVisible() {
					var cmd tea.Cmd
					adapter.model, cmd = adapter.model.Update(msg)
					return m, cmd
				}
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View renders the view
func (m Model) View() string {
	return m.list.View()
}

// GetViewMode returns the current view mode (for testing)
func (m Model) GetViewMode() ViewMode {
	return m.list.GetViewMode()
}

// GetContextItems returns context bar items for the current view
func (m Model) GetContextItems() []components.ContextItem {
	return m.list.GetContextItems()
}

// GetScrollPercent returns the scroll percentage for the current view
func (m Model) GetScrollPercent() float64 {
	return m.list.GetScrollPercent()
}

// GetStatusMessage returns the status message for the current view
func (m Model) GetStatusMessage() string {
	return m.list.GetStatusMessage()
}

// HasContextBar returns true if the current view should show a context bar
func (m Model) HasContextBar() bool {
	return m.list.HasContextBar()
}

// IsSearching returns true if the list is currently in search/filter mode.
func (m Model) IsSearching() bool {
	return m.list.IsSearching()
}

// IsMyItemsActive returns true if the "my items" filter is active.
func (m Model) IsMyItemsActive() bool {
	return m.myItemsOnly
}

// IsCommentFormVisible reports whether the work item detail view currently has
// its comment form open. Used by the app to suppress global shortcuts so
// keystrokes reach the form's textarea.
func (m Model) IsCommentFormVisible() bool {
	if m.GetViewMode() != ViewDetail {
		return false
	}
	if adapter, ok := m.list.Detail().(*detailAdapter); ok {
		return adapter.model.commentForm.IsVisible()
	}
	return false
}

// DetailItemID returns the ID of the work item whose detail view is
// currently open, or 0 when the user is on the list. Used by the state
// store to persist the last-viewed item across sessions.
func (m Model) DetailItemID() int {
	if m.GetViewMode() != ViewDetail {
		return 0
	}
	adapter, ok := m.list.Detail().(*detailAdapter)
	if !ok || adapter == nil {
		return 0
	}
	return adapter.model.GetWorkItemID()
}

// WithPendingDetailRestore queues a one-shot request to open the work
// item with this ID in detail view once the list is populated.
func (m Model) WithPendingDetailRestore(id int) Model {
	m.pendingDetailID = id
	m.pendingRestoreHandled = false
	return m
}

// tryRestoreDetail attempts to open detail for the pending ID, if any.
func (m Model) tryRestoreDetail() (Model, tea.Cmd) {
	if m.pendingRestoreHandled || m.pendingDetailID == 0 {
		return m, nil
	}
	target := m.pendingDetailID
	m.pendingDetailID = 0
	m.pendingRestoreHandled = true

	idx := m.list.FindIndex(func(wi provider.WorkItem) bool {
		id, _ := strconv.Atoi(wi.Identity.ID)
		return id == target
	})
	if idx < 0 {
		return m, nil
	}
	m.list.SetCursor(idx)
	list, cmd := m.list.OpenSelectedDetail()
	m.list = list
	return m, cmd
}

// withRestore combines tryRestoreDetail with a caller-supplied cmd.
func (m Model) withRestore(prev tea.Cmd) (Model, tea.Cmd) {
	m, restoreCmd := m.tryRestoreDetail()
	switch {
	case prev == nil:
		return m, restoreCmd
	case restoreCmd == nil:
		return m, prev
	default:
		return m, tea.Batch(prev, restoreCmd)
	}
}

// IsTagFilterActive returns true if a tag filter is active.
func (m Model) IsTagFilterActive() bool {
	return m.activeTag != ""
}

// ActiveTag returns the currently active tag filter, or "" if none.
func (m Model) ActiveTag() string {
	return m.activeTag
}

// IsTagPickerVisible returns true if the tag picker modal is open.
func (m Model) IsTagPickerVisible() bool {
	return m.tagPicker.IsVisible()
}

// TagPickerSearchQuery returns the tag picker's current search input value
// (for testing).
func (m Model) TagPickerSearchQuery() string {
	return m.tagPicker.SearchQuery()
}

// TagPickerView returns the rendered tag picker overlay.
func (m Model) TagPickerView() string {
	return m.tagPicker.View()
}

// SetTagPickerSize sets the dimensions for the tag picker overlay.
func (m *Model) SetTagPickerSize(width, height int) {
	m.tagPicker.SetSize(width, height)
}

// IsStateFilterActive returns true if a state filter is active.
func (m Model) IsStateFilterActive() bool {
	return m.activeState != ""
}

// ActiveState returns the currently active state filter, or "" if none.
func (m Model) ActiveState() string {
	return m.activeState
}

// IsStatePickerVisible returns true if the state picker modal is open.
func (m Model) IsStatePickerVisible() bool {
	return m.statePicker.IsVisible()
}

// StatePickerView returns the rendered state picker overlay.
func (m Model) StatePickerView() string {
	return m.statePicker.View()
}

// SetStatePickerSize sets the dimensions for the state picker overlay.
func (m *Model) SetStatePickerSize(width, height int) {
	m.statePicker.SetSize(width, height)
}

// getBaseItems returns the appropriate base items (allItems or myItems)
func (m Model) getBaseItems() []provider.WorkItem {
	if m.myItemsOnly {
		return m.myItems
	}
	return m.allItems
}

// applyAllFilters applies tag and state filters to the given items.
func (m Model) applyAllFilters(items []provider.WorkItem) []provider.WorkItem {
	result := applyTagFilter(items, m.activeTag)
	result = applyStateFilter(result, m.activeState)
	return result
}

// detailAdapter wraps *DetailModel to satisfy listview.DetailView
type detailAdapter struct {
	model *DetailModel
}

func (a *detailAdapter) Update(msg tea.Msg) (listview.DetailView, tea.Cmd) {
	var cmd tea.Cmd
	a.model, cmd = a.model.Update(msg)
	return a, cmd
}

func (a *detailAdapter) View() string {
	return a.model.View()
}

func (a *detailAdapter) SetSize(width, height int) {
	a.model.SetSize(width, height)
}

func (a *detailAdapter) GetContextItems() []components.ContextItem {
	return a.model.GetContextItems()
}

func (a *detailAdapter) GetScrollPercent() float64 {
	return a.model.GetScrollPercent()
}

func (a *detailAdapter) GetStatusMessage() string {
	return a.model.GetStatusMessage()
}

// workItemsToRows converts work items to table rows
func workItemsToRows(items []provider.WorkItem, s *styles.Styles) []table.Row {
	rows := make([]table.Row, len(items))
	for i, wi := range items {
		assignedTo := wi.AssignedToName
		if assignedTo == "" {
			assignedTo = "-"
		}
		rows[i] = table.Row{
			typeIconWithStyles(wi.WorkItemType, s),
			wi.Identity.ID,
			wi.Title,
			stateTextWithStyles(wi.State, s),
			priorityTextWithStyles(wi.Priority, s),
			assignedTo,
		}
	}
	return rows
}

// workItemsToRowsMulti converts work items to table rows with a Project column.
func workItemsToRowsMulti(items []provider.WorkItem, s *styles.Styles) []table.Row {
	rows := make([]table.Row, len(items))
	for i, wi := range items {
		assignedTo := wi.AssignedToName
		if assignedTo == "" {
			assignedTo = "-"
		}
		rows[i] = table.Row{
			wi.Identity.ScopeDisplay,
			typeIconWithStyles(wi.WorkItemType, s),
			wi.Identity.ID,
			wi.Title,
			stateTextWithStyles(wi.State, s),
			priorityTextWithStyles(wi.Priority, s),
			assignedTo,
		}
	}
	return rows
}

// filterWorkItem returns true if the work item matches the search query.
func filterWorkItem(wi provider.WorkItem, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	if strings.Contains(strings.ToLower(wi.Title), q) ||
		strings.Contains(wi.Identity.ID, q) ||
		strings.Contains(strings.ToLower(wi.State), q) ||
		strings.Contains(strings.ToLower(wi.WorkItemType), q) {
		return true
	}
	if strings.Contains(strings.ToLower(wi.AssignedToName), q) {
		return true
	}
	if strings.Contains(strings.ToLower(wi.Tags), q) {
		return true
	}
	return false
}

// filterWorkItemMulti matches work item fields including project name.
func filterWorkItemMulti(wi provider.WorkItem, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	if strings.Contains(strings.ToLower(wi.Identity.ScopeDisplay), q) ||
		strings.Contains(strings.ToLower(wi.Identity.Scope), q) {
		return true
	}
	return filterWorkItem(wi, query)
}

// WorkItemStateChangedMsg is emitted after a work item state is successfully updated.
// The list model uses it to trigger a data refresh.
type WorkItemStateChangedMsg struct{}

// Messages

type workItemsMsg struct {
	workItems []provider.WorkItem
	err       error
}

type myWorkItemsMsg struct {
	workItems []provider.WorkItem
	err       error
}

// SetWorkItemsMsg is a message to directly set the work items (from polling)
type SetWorkItemsMsg struct {
	WorkItems []provider.WorkItem
}

// fetchWorkItems fetches work items from all projects via the provider.
func fetchWorkItems(client provider.Provider) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return workItemsMsg{workItems: nil, err: nil}
		}
		workItems, err := client.ListWorkItems(50, provider.ListOpts{})
		return workItemsMsg{workItems: workItems, err: err}
	}
}

// fetchMyWorkItems fetches work items assigned to the authenticated user
// using the @Me WIQL macro.
func fetchMyWorkItems(client provider.Provider) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return myWorkItemsMsg{workItems: nil, err: nil}
		}
		workItems, err := client.ListMyWorkItems(50, provider.ListOpts{})
		return myWorkItemsMsg{workItems: workItems, err: err}
	}
}

// tagList splits a semicolon-separated tags string into a trimmed slice.
// Returns nil if there are no tags.
func tagList(tags string) []string {
	if tags == "" {
		return nil
	}
	raw := strings.Split(tags, ";")
	result := make([]string, 0, len(raw))
	for _, t := range raw {
		t = strings.TrimSpace(t)
		if t != "" {
			result = append(result, t)
		}
	}
	return result
}

// collectUniqueTags extracts all unique tags from the work items, sorted alphabetically.
func collectUniqueTags(items []provider.WorkItem) []string {
	seen := make(map[string]struct{})
	for i := range items {
		for _, tag := range tagList(items[i].Tags) {
			seen[tag] = struct{}{}
		}
	}
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

// collectUniqueStates extracts all unique states from the work items, sorted alphabetically.
func collectUniqueStates(items []provider.WorkItem) []string {
	seen := make(map[string]struct{})
	for i := range items {
		if items[i].State != "" {
			seen[items[i].State] = struct{}{}
		}
	}
	states := make([]string, 0, len(seen))
	for state := range seen {
		states = append(states, state)
	}
	sort.Strings(states)
	return states
}

// applyTagFilter returns only work items that have the given tag.
// If tag is empty, all items are returned unfiltered.
func applyTagFilter(items []provider.WorkItem, tag string) []provider.WorkItem {
	if tag == "" {
		return items
	}
	var filtered []provider.WorkItem
	for _, wi := range items {
		for _, t := range tagList(wi.Tags) {
			if t == tag {
				filtered = append(filtered, wi)
				break
			}
		}
	}
	return filtered
}

// applyStateFilter returns only work items that have the given state.
// If state is empty, all items are returned unfiltered.
func applyStateFilter(items []provider.WorkItem, state string) []provider.WorkItem {
	if state == "" {
		return items
	}
	var filtered []provider.WorkItem
	for _, wi := range items {
		if wi.State == state {
			filtered = append(filtered, wi)
		}
	}
	return filtered
}

// Icon/text formatting functions (unchanged)

// typeIconWithStyles returns a styled text label for the work item type using provided styles
func typeIconWithStyles(workItemType string, s *styles.Styles) string {
	accentStyle := lipgloss.NewStyle().Foreground(s.Theme.Accent)

	switch workItemType {
	case "Bug":
		return s.Error.Render("Bug")
	case "Task":
		return s.Info.Render("Task")
	case "User Story":
		return s.Success.Render("Story")
	case "Feature":
		return accentStyle.Render("Feature")
	case "Epic":
		return s.Warning.Render("Epic")
	case "Issue":
		return s.Error.Render("Issue")
	default:
		return s.Muted.Render("Item")
	}
}

// stateTextWithStyles returns styled text for the work item state using provided styles
func stateTextWithStyles(state string, s *styles.Styles) string {
	stateLower := strings.ToLower(state)
	secondaryStyle := lipgloss.NewStyle().Foreground(s.Theme.Secondary)

	switch {
	case stateLower == "new":
		return s.Muted.Render("New")
	case stateLower == "active":
		return s.Info.Render("Active")
	case stateLower == "resolved":
		return s.Warning.Render("Resolved")
	case strings.Contains(stateLower, "ready"):
		return secondaryStyle.Render(state)
	case stateLower == "closed":
		return s.Success.Render("Closed")
	case stateLower == "removed":
		return s.Error.Render("Removed")
	default:
		return s.Muted.Render(state)
	}
}

// priorityTextWithStyles returns styled text for priority using provided styles
func priorityTextWithStyles(priority int, s *styles.Styles) string {
	switch priority {
	case 1:
		return s.Error.Render("P1")
	case 2:
		return s.Warning.Render("P2")
	case 3:
		return s.Warning.Render("P3")
	case 4:
		return s.Muted.Render("P4")
	default:
		return s.Muted.Render(fmt.Sprintf("P%d", priority))
	}
}
