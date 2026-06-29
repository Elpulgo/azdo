package azdevops

import (
	"strings"

	"github.com/Elpulgo/azdo/internal/provider"
)

// MapStateCategory translates an Azure DevOps wire state string (work-item or
// PR status) into a neutral provider.StateCategory. Comparison is
// case-insensitive. States whose names contain "ready" map to
// StateCategoryReadyForTest; "completed" maps to StateCategoryClosedDone;
// "abandoned" maps to StateCategoryRemoved.
func MapStateCategory(state string) provider.StateCategory {
	lower := strings.ToLower(state)
	switch {
	case lower == "new":
		return provider.StateCategoryNew
	case lower == "active":
		return provider.StateCategoryActive
	case lower == "resolved":
		return provider.StateCategoryResolved
	case strings.Contains(lower, "ready"):
		return provider.StateCategoryReadyForTest
	case lower == "closed" || lower == "completed":
		return provider.StateCategoryClosedDone
	case lower == "removed" || lower == "abandoned":
		return provider.StateCategoryRemoved
	default:
		return provider.StateCategoryUnknown
	}
}

// MapItemType translates an Azure DevOps wire work-item type string into a
// neutral provider.ItemType. Comparison is case-insensitive.
func MapItemType(workItemType string) provider.ItemType {
	lower := strings.ToLower(workItemType)
	switch lower {
	case "bug":
		return provider.ItemTypeBug
	case "task":
		return provider.ItemTypeTask
	case "user story":
		return provider.ItemTypeUserStory
	case "feature":
		return provider.ItemTypeFeature
	case "epic":
		return provider.ItemTypeEpic
	case "issue":
		return provider.ItemTypeIssue
	default:
		return provider.ItemTypeUnknown
	}
}

// MapVoteKind translates an Azure DevOps wire reviewer vote integer into a
// neutral provider.VoteKind. Unknown values map to VoteKindNoVote (the safe
// default).
func MapVoteKind(vote int) provider.VoteKind {
	switch vote {
	case VoteApprove:
		return provider.VoteKindApproved
	case VoteApproveWithSuggestions:
		return provider.VoteKindApprovedWithSuggestions
	case VoteNoVote:
		return provider.VoteKindNoVote
	case VoteWaitForAuthor:
		return provider.VoteKindWaitingForAuthor
	case VoteReject:
		return provider.VoteKindRejected
	default:
		return provider.VoteKindNoVote
	}
}

// MapRunStatus translates an Azure DevOps wire pipeline run status+result pair
// into a neutral provider.RunStatus. Status is checked before result because
// in-flight states (inProgress, notStarted, canceling) are authoritative
// regardless of the result field.
func MapRunStatus(status, result string) provider.RunStatus {
	statusLower := strings.ToLower(status)
	resultLower := strings.ToLower(result)

	switch {
	case statusLower == "inprogress":
		return provider.RunStatusRunning
	case statusLower == "notstarted":
		return provider.RunStatusQueued
	case statusLower == "canceling":
		return provider.RunStatusCanceling
	case resultLower == "succeeded":
		return provider.RunStatusSucceeded
	case resultLower == "failed":
		return provider.RunStatusFailed
	case resultLower == "canceled":
		return provider.RunStatusCanceled
	case resultLower == "partiallysucceeded":
		return provider.RunStatusPartiallySucceeded
	default:
		return provider.RunStatusUnknown
	}
}
