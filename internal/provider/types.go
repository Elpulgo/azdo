// Package provider defines neutral domain types that are independent of any
// specific backend (Azure DevOps, GitHub, etc.). All types carry an Identity
// field that is populated only at the adapter mapping boundary.
package provider

import "time"

// Kind identifies the backend that produced a domain entity.
type Kind int

const (
	// KindAzure identifies entities originating from Azure DevOps.
	KindAzure Kind = iota + 1
)

// Identity stamps every neutral entity with its origin.
// Kind identifies the backend, Scope is the project name, and ID is the
// backend's string representation of the entity's numeric or UUID identifier.
// All three fields must be non-zero on any entity returned through the interface.
type Identity struct {
	Kind  Kind
	Scope string // project name (ProjectName from the wire layer)
	ID    string // wire ID converted to string
}

// WorkItem is the neutral representation of an Azure DevOps work item (or its
// equivalent in another backend).
type WorkItem struct {
	Identity        Identity
	Title           string
	State           string
	WorkItemType    string
	AssignedToName  string
	Priority        int
	ChangedDate     time.Time
	CreatedDate     time.Time
	StateChangeDate time.Time
	ActivatedDate   time.Time
	ClosedDate      time.Time
	IterationPath   string
	Description     string
	ReproSteps      string
	Tags            string
	StoryPoints     float64
	URL             string
}

// PullRequest is the neutral representation of a pull request.
type PullRequest struct {
	Identity       Identity
	Title          string
	Description    string
	Status         string
	CreationDate   time.Time
	SourceRefName  string
	TargetRefName  string
	IsDraft        bool
	CreatedByName  string
	CreatedByID    string
	RepositoryID   string
	RepositoryName string
	Reviewers      []Reviewer
	WebURL         string
}

// Reviewer is the neutral representation of a pull request reviewer.
type Reviewer struct {
	ID          string
	DisplayName string
	Vote        int
}

// PipelineRun is the neutral representation of a pipeline/build run.
type PipelineRun struct {
	Identity       Identity
	BuildNumber    string
	Status         string
	Result         string
	SourceBranch   string
	SourceVersion  string
	QueueTime      time.Time
	StartTime      *time.Time
	FinishTime     *time.Time
	DefinitionID   int
	DefinitionName string
	WebURL         string
}

// Thread is the neutral representation of a pull request comment thread.
type Thread struct {
	Identity        Identity
	PublishedDate   time.Time
	LastUpdatedDate time.Time
	Status          string
	FilePath        string // non-empty when this is a code comment
	Line            int    // new-file line number from RightFileStart.Line; 0 for general comments
	Comments        []Comment
	IsDeleted       bool
}

// Comment is the neutral representation of a single comment within a thread.
type Comment struct {
	Identity        Identity
	ParentCommentID int
	Content         string
	PublishedDate   time.Time
	LastUpdatedDate time.Time
	CommentType     string
	AuthorName      string
	AuthorID        string
}

// Timeline is the neutral representation of a pipeline build timeline, which
// contains the ordered set of stages, jobs, and tasks for a run.
type Timeline struct {
	Identity Identity
	Records  []TimelineRecord
}

// TimelineRecord is a single entry in a Timeline (stage, job, or task).
type TimelineRecord struct {
	ID         string
	ParentID   string
	Type       string
	Name       string
	State      string
	Result     string
	Order      int
	StartTime  *time.Time
	FinishTime *time.Time
	LogID      int
	Issues     []TimelineIssue
}

// TimelineIssue is an error or warning within a TimelineRecord.
type TimelineIssue struct {
	Type    string
	Message string
}

// BuildLog is the neutral representation of a single log artifact for a build.
type BuildLog struct {
	Identity      Identity
	LogID         int
	LineCount     int
	CreatedOn     *time.Time
	LastChangedOn *time.Time
	URL           string
}
