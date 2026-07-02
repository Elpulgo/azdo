package github

import (
	"fmt"
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
)

// MapWorkItem maps a GitHub wire Issue to a provider.WorkItem.
//
// scope is the "owner/repo" string (the API-level scope identifier) and
// scopeDisplay is its human-readable equivalent. Both are stamped onto Identity
// at this boundary, mirroring the azdevops mapping convention.
//
// conv is the LabelConvention used to derive ItemKind, Priority, and Tags from
// the issue's labels. Callers in Phase 3 pass DefaultLabelConvention(); Phase 4
// will pass a user-configured instance.
//
// StateChangeDate note: GitHub issues expose no dedicated state-change timestamp.
// When the issue is closed we approximate it with ClosedAt; for open issues it
// is left as the zero time.Time.
//
// ActivatedDate, ReproSteps, and StoryPoints have no GitHub equivalent and are
// left at their zero values.
func MapWorkItem(issue Issue, conv LabelConvention, scope, scopeDisplay string) provider.WorkItem {
	assignedTo := ""
	if issue.Assignee != nil {
		assignedTo = issue.Assignee.Login
	}

	iterationPath := ""
	if issue.Milestone != nil {
		iterationPath = issue.Milestone.Title
	}

	stateReason := derefString(issue.StateReason)
	stateCategory := MapStateCategory(issue.State, stateReason)

	itemKind, priority, tags := conv.Parse(issue.Labels)
	workItemType := itemTypeDisplay(itemKind)

	closedDate := derefTime(issue.ClosedAt)

	// GitHub has no dedicated state-change timestamp. Approximate with ClosedAt
	// for closed issues; leave zero for open issues.
	stateChangeDate := time.Time{}
	if !closedDate.IsZero() {
		stateChangeDate = closedDate
	}

	return provider.WorkItem{
		Identity: provider.Identity{
			Kind:         provider.KindGitHub,
			Scope:        scope,
			ScopeDisplay: scopeDisplay,
			ID:           fmt.Sprintf("%d", issue.Number),
		},
		Title:           issue.Title,
		State:           issue.State,
		WorkItemType:    workItemType,
		StateCategory:   stateCategory,
		ItemKind:        itemKind,
		AssignedToName:  assignedTo,
		Priority:        priority,
		ChangedDate:     issue.UpdatedAt,
		CreatedDate:     issue.CreatedAt,
		StateChangeDate: stateChangeDate,
		// ActivatedDate has no GitHub equivalent; left zero.
		ActivatedDate: time.Time{},
		ClosedDate:    closedDate,
		IterationPath: iterationPath,
		Description:   issue.Body,
		// ReproSteps has no GitHub equivalent; left zero.
		ReproSteps: "",
		Tags:       tags,
		// StoryPoints has no GitHub equivalent; left zero.
		StoryPoints: 0,
		URL:         issue.HTMLURL,
	}
}

// MapWorkItemComment maps a GitHub wire IssueComment to a provider.WorkItemComment.
//
// scope and scopeDisplay are stamped onto Identity, mirroring the azdevops
// mapping convention.
func MapWorkItemComment(c IssueComment, scope, scopeDisplay string) provider.WorkItemComment {
	return provider.WorkItemComment{
		Identity: provider.Identity{
			Kind:         provider.KindGitHub,
			Scope:        scope,
			ScopeDisplay: scopeDisplay,
			ID:           fmt.Sprintf("%d", c.ID),
		},
		ID:          int(c.ID),
		Text:        c.Body,
		AuthorName:  c.User.Login,
		CreatedDate: c.CreatedAt,
	}
}

// itemTypeDisplay derives a human-readable WorkItemType string from the neutral
// ItemType enum. This string is displayed directly in the work-item detail
// header (detail.go: fmt.Sprintf("%s | ...", wi.WorkItemType, ...)) and is also
// used by the search filter. It must not be empty — the header renders it
// verbatim, so a meaningful label is always preferable to an empty string.
func itemTypeDisplay(t provider.ItemType) string {
	switch t {
	case provider.ItemTypeBug:
		return "Bug"
	case provider.ItemTypeTask:
		return "Task"
	case provider.ItemTypeUserStory:
		return "User Story"
	case provider.ItemTypeFeature:
		return "Feature"
	case provider.ItemTypeEpic:
		return "Epic"
	case provider.ItemTypeIssue:
		return "Issue"
	default:
		return "Issue"
	}
}

// derefString returns the string value pointed to by s, or "" when s is nil.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// derefTime returns the time.Time value pointed to by t, or the zero time.Time
// when t is nil.
func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
