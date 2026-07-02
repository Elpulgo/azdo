package azdevops

import (
	"fmt"

	"github.com/Elpulgo/azdo/internal/provider"
)

// MapWorkItem maps an azdevops wire WorkItem to a provider.WorkItem.
// scope is the project API name (ProjectName) and scopeDisplay is its human-readable
// display name (ProjectDisplayName). Both are stamped onto Identity at this boundary.
func MapWorkItem(w WorkItem, scope, scopeDisplay string) provider.WorkItem {
	assignedTo := ""
	if w.Fields.AssignedTo != nil {
		assignedTo = w.Fields.AssignedTo.DisplayName
	}
	return provider.WorkItem{
		Identity: provider.Identity{
			Kind:         provider.KindAzure,
			Scope:        scope,
			ScopeDisplay: scopeDisplay,
			ID:           fmt.Sprintf("%d", w.ID),
		},
		Title:           w.Fields.Title,
		State:           w.Fields.State,
		WorkItemType:    w.Fields.WorkItemType,
		StateCategory:   MapStateCategory(w.Fields.State),
		ItemKind:        MapItemType(w.Fields.WorkItemType),
		AssignedToName:  assignedTo,
		Priority:        w.Fields.Priority,
		ChangedDate:     w.Fields.ChangedDate,
		CreatedDate:     w.Fields.CreatedDate,
		StateChangeDate: w.Fields.StateChangeDate,
		ActivatedDate:   w.Fields.ActivatedDate,
		ClosedDate:      w.Fields.ClosedDate,
		IterationPath:   w.Fields.IterationPath,
		Description:     w.Fields.Description,
		ReproSteps:      w.Fields.ReproSteps,
		Tags:            w.Fields.Tags,
		StoryPoints:     w.Fields.StoryPoints,
		URL:             w.URL,
	}
}

// MapPullRequest maps an azdevops wire PullRequest to a provider.PullRequest.
func MapPullRequest(pr PullRequest, scope, scopeDisplay string) provider.PullRequest {
	reviewers := make([]provider.Reviewer, len(pr.Reviewers))
	for i, r := range pr.Reviewers {
		reviewers[i] = provider.Reviewer{
			ID:          r.ID,
			DisplayName: r.DisplayName,
			Vote:        r.Vote,
			Kind:        MapVoteKind(r.Vote),
		}
	}
	return provider.PullRequest{
		Identity: provider.Identity{
			Kind:         provider.KindAzure,
			Scope:        scope,
			ScopeDisplay: scopeDisplay,
			ID:           fmt.Sprintf("%d", pr.ID),
		},
		Title:          pr.Title,
		Description:    pr.Description,
		Status:         pr.Status,
		StatusCategory: MapStateCategory(pr.Status),
		CreationDate:   pr.CreationDate,
		SourceRefName:  pr.SourceRefName,
		TargetRefName:  pr.TargetRefName,
		IsDraft:        pr.IsDraft,
		CreatedByName:  pr.CreatedBy.DisplayName,
		CreatedByID:    pr.CreatedBy.ID,
		RepositoryID:   pr.Repository.ID,
		RepositoryName: pr.Repository.Name,
		Reviewers:      reviewers,
	}
}

// MapPipelineRun maps an azdevops wire PipelineRun to a provider.PipelineRun.
func MapPipelineRun(p PipelineRun, scope, scopeDisplay string) provider.PipelineRun {
	return provider.PipelineRun{
		Identity: provider.Identity{
			Kind:         provider.KindAzure,
			Scope:        scope,
			ScopeDisplay: scopeDisplay,
			ID:           fmt.Sprintf("%d", p.ID),
		},
		BuildNumber:    p.BuildNumber,
		Status:         p.Status,
		Result:         p.Result,
		RunStatus:      MapRunStatus(p.Status, p.Result),
		SourceBranch:   p.SourceBranch,
		SourceVersion:  p.SourceVersion,
		QueueTime:      p.QueueTime,
		StartTime:      p.StartTime,
		FinishTime:     p.FinishTime,
		DefinitionID:   p.Definition.ID,
		DefinitionName: p.Definition.Name,
		WebURL:         p.Links.Web.Href,
	}
}

// MapThread maps an azdevops wire Thread to a provider.Thread.
// The Line field is populated from ThreadContext.RightFileStart.Line; it is 0
// for general (non-file) comment threads.
func MapThread(t Thread, scope, scopeDisplay string) provider.Thread {
	var filePath string
	var line int
	if t.ThreadContext != nil {
		filePath = t.ThreadContext.FilePath
		if t.ThreadContext.RightFileStart != nil {
			line = t.ThreadContext.RightFileStart.Line
		}
	}

	comments := make([]provider.Comment, len(t.Comments))
	for i, c := range t.Comments {
		comments[i] = MapComment(c, scope, scopeDisplay)
	}

	return provider.Thread{
		Identity: provider.Identity{
			Kind:         provider.KindAzure,
			Scope:        scope,
			ScopeDisplay: scopeDisplay,
			ID:           fmt.Sprintf("%d", t.ID),
		},
		PublishedDate:   t.PublishedDate,
		LastUpdatedDate: t.LastUpdatedDate,
		Status:          t.Status,
		FilePath:        filePath,
		Line:            line,
		Comments:        comments,
		IsDeleted:       t.IsDeleted,
	}
}

// MapComment maps an azdevops wire Comment to a provider.Comment.
func MapComment(c Comment, scope, scopeDisplay string) provider.Comment {
	return provider.Comment{
		Identity: provider.Identity{
			Kind:         provider.KindAzure,
			Scope:        scope,
			ScopeDisplay: scopeDisplay,
			ID:           fmt.Sprintf("%d", c.ID),
		},
		ParentCommentID: c.ParentCommentID,
		Content:         c.Content,
		PublishedDate:   c.PublishedDate,
		LastUpdatedDate: c.LastUpdatedDate,
		CommentType:     c.CommentType,
		AuthorName:      c.Author.DisplayName,
		AuthorID:        c.Author.ID,
	}
}

// MapTimeline maps an azdevops wire Timeline to a provider.Timeline.
// The Timeline ID (a UUID) is used as the Identity.ID.
func MapTimeline(t Timeline, scope, scopeDisplay string) provider.Timeline {
	records := make([]provider.TimelineRecord, len(t.Records))
	for i, r := range t.Records {
		records[i] = mapTimelineRecord(r)
	}
	return provider.Timeline{
		Identity: provider.Identity{
			Kind:         provider.KindAzure,
			Scope:        scope,
			ScopeDisplay: scopeDisplay,
			ID:           t.ID,
		},
		Records: records,
	}
}

func mapTimelineRecord(r TimelineRecord) provider.TimelineRecord {
	var logID int
	if r.Log != nil {
		logID = r.Log.ID
	}
	var parentID string
	if r.ParentID != nil {
		parentID = *r.ParentID
	}
	issues := make([]provider.TimelineIssue, len(r.Issues))
	for i, iss := range r.Issues {
		issues[i] = provider.TimelineIssue{
			Type:    iss.Type,
			Message: iss.Message,
		}
	}
	return provider.TimelineRecord{
		ID:         r.ID,
		ParentID:   parentID,
		Type:       r.Type,
		Name:       r.Name,
		State:      r.State,
		Result:     r.Result,
		Order:      r.Order,
		StartTime:  r.StartTime,
		FinishTime: r.FinishTime,
		LogID:      logID,
		Issues:     issues,
	}
}

// MapBuildLog maps an azdevops wire BuildLog to a provider.BuildLog.
func MapBuildLog(b BuildLog, scope, scopeDisplay string) provider.BuildLog {
	return provider.BuildLog{
		Identity: provider.Identity{
			Kind:         provider.KindAzure,
			Scope:        scope,
			ScopeDisplay: scopeDisplay,
			ID:           fmt.Sprintf("%d", b.ID),
		},
		LogID:         b.ID,
		LineCount:     b.LineCount,
		CreatedOn:     b.CreatedOn,
		LastChangedOn: b.LastChangedOn,
		URL:           b.URL,
	}
}

// MapIteration maps an azdevops wire Iteration to a provider.Iteration.
// Iterations are sub-entities of a PR (one per push) and carry no Identity.
func MapIteration(it Iteration) provider.Iteration {
	return provider.Iteration{
		ID:          it.ID,
		Description: it.Description,
	}
}

// MapIterationChange maps an azdevops wire IterationChange to a provider.IterationChange.
// IterationChanges are sub-entities of a PR iteration and carry no Identity.
func MapIterationChange(ic IterationChange) provider.IterationChange {
	return provider.IterationChange{
		ChangeID:      ic.ChangeID,
		Path:          ic.Item.Path,
		GitObjectType: ic.Item.GitObjectType,
		ChangeType:    ic.ChangeType,
		OriginalPath:  ic.OriginalPath,
	}
}

// MapWorkItemTypeState maps an azdevops wire WorkItemTypeState to a provider.WorkItemTypeState.
// WorkItemTypeStates are metadata sub-entities and carry no Identity.
func MapWorkItemTypeState(s WorkItemTypeState) provider.WorkItemTypeState {
	return provider.WorkItemTypeState{
		Name:     s.Name,
		Color:    s.Color,
		Category: s.Category,
	}
}

// MapWorkItemComment maps an azdevops wire WorkItemComment to a provider.WorkItemComment.
func MapWorkItemComment(c WorkItemComment, scope, scopeDisplay string) provider.WorkItemComment {
	return provider.WorkItemComment{
		Identity: provider.Identity{
			Kind:         provider.KindAzure,
			Scope:        scope,
			ScopeDisplay: scopeDisplay,
			ID:           fmt.Sprintf("%d", c.ID),
		},
		ID:          c.ID,
		Text:        c.Text,
		AuthorName:  c.CreatedBy.DisplayName,
		CreatedDate: c.CreatedDate,
	}
}
