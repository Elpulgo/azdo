package provider

// StateCategory is a neutral semantic bucket for work-item and pull-request
// states. Views use it to decide color and glyph without inspecting Azure
// string values.
type StateCategory int

const (
	// StateCategoryUnknown is the zero value; used for unmapped/custom states.
	StateCategoryUnknown StateCategory = iota
	// StateCategoryNew corresponds to Azure's "New" state.
	StateCategoryNew
	// StateCategoryActive corresponds to Azure's "Active" state.
	StateCategoryActive
	// StateCategoryResolved corresponds to Azure's "Resolved" state.
	StateCategoryResolved
	// StateCategoryReadyForTest corresponds to Azure custom states whose names
	// contain "ready" (e.g. "Ready for Test", "Ready for Review"). These are
	// rendered with the theme's Secondary color, distinct from Resolved.
	StateCategoryReadyForTest
	// StateCategoryClosedDone corresponds to Azure's "Closed" and PR's
	// "completed" — the happy-path terminal state.
	StateCategoryClosedDone
	// StateCategoryRemoved corresponds to Azure's "Removed" and PR's
	// "abandoned" — the discarded terminal state.
	StateCategoryRemoved
)

// ItemType is a neutral semantic enum for work-item types. Views use it to
// decide label, color, and glyph without inspecting Azure string values.
type ItemType int

const (
	// ItemTypeUnknown is the zero value; used for unmapped or custom types.
	ItemTypeUnknown ItemType = iota
	// ItemTypeBug corresponds to Azure's "Bug" work-item type.
	ItemTypeBug
	// ItemTypeTask corresponds to Azure's "Task" work-item type.
	ItemTypeTask
	// ItemTypeUserStory corresponds to Azure's "User Story" work-item type.
	ItemTypeUserStory
	// ItemTypeFeature corresponds to Azure's "Feature" work-item type.
	ItemTypeFeature
	// ItemTypeEpic corresponds to Azure's "Epic" work-item type.
	ItemTypeEpic
	// ItemTypeIssue corresponds to Azure's "Issue" work-item type.
	ItemTypeIssue
)

// VoteKind is a neutral semantic enum for reviewer vote values on a pull
// request. Views use it to decide icon and color without inspecting wire
// integers.
type VoteKind int

const (
	// VoteKindNoVote is the zero value; reviewer has not voted (wire value 0).
	VoteKindNoVote VoteKind = iota
	// VoteKindApproved indicates the reviewer approved (wire value 10).
	VoteKindApproved
	// VoteKindApprovedWithSuggestions indicates approved with suggestions
	// (wire value 5).
	VoteKindApprovedWithSuggestions
	// VoteKindWaitingForAuthor indicates the reviewer is waiting for the
	// author to respond (wire value -5).
	VoteKindWaitingForAuthor
	// VoteKindRejected indicates the reviewer rejected the PR (wire value -10).
	VoteKindRejected
)

// RunStatus is a neutral semantic enum for the combined pipeline run
// status+result. Views use it to decide icon and color without inspecting
// Azure string pairs.
type RunStatus int

const (
	// RunStatusUnknown is the zero value; used for unmapped status/result
	// combinations.
	RunStatusUnknown RunStatus = iota
	// RunStatusRunning corresponds to Azure status "inProgress".
	RunStatusRunning
	// RunStatusQueued corresponds to Azure status "notStarted".
	RunStatusQueued
	// RunStatusCanceling corresponds to Azure status "canceling" (run is
	// being cancelled but has not finished yet).
	RunStatusCanceling
	// RunStatusSucceeded corresponds to Azure result "succeeded".
	RunStatusSucceeded
	// RunStatusFailed corresponds to Azure result "failed".
	RunStatusFailed
	// RunStatusCanceled corresponds to Azure result "canceled" (run finished
	// after cancellation).
	RunStatusCanceled
	// RunStatusPartiallySucceeded corresponds to Azure result
	// "partiallySucceeded".
	RunStatusPartiallySucceeded
)
