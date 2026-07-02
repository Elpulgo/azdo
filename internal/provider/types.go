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
	// KindGitHub identifies entities originating from GitHub.
	KindGitHub
)

// String returns a stable, lowercase identifier for the Kind, suitable for
// on-disk serialization (state persistence). Unknown kinds return "".
// This is distinct from display.KindLabel, which returns a human-facing name.
func (k Kind) String() string {
	switch k {
	case KindAzure:
		return "azure"
	case KindGitHub:
		return "github"
	default:
		return ""
	}
}

// ParseKind maps a stable string identifier (see Kind.String) back to a Kind.
// Unrecognized input returns the zero Kind.
func ParseKind(s string) Kind {
	switch s {
	case "azure":
		return KindAzure
	case "github":
		return KindGitHub
	default:
		return 0
	}
}

// Identity stamps every neutral entity with its origin.
// Kind identifies the backend, Scope is the project API name (ProjectName from
// the wire layer), ScopeDisplay is the human-readable project display name
// (ProjectDisplayName from the wire layer), and ID is the backend's string
// representation of the entity's numeric or UUID identifier.
// All fields except ScopeDisplay must be non-zero on any entity returned
// through the interface. ScopeDisplay falls back to Scope at the adapter
// boundary when no display name is configured.
type Identity struct {
	Kind         Kind
	Scope        string // project API name (ProjectName from the wire layer)
	ScopeDisplay string // human-readable project display name (ProjectDisplayName)
	ID           string // wire ID converted to string
}

// IsZero reports whether the identity is unset (no backend ID).
func (i Identity) IsZero() bool {
	return i.ID == ""
}

// SameItem reports whether two identities refer to the same entity across
// backends. It compares Kind, Scope, and ID — the fields that make an item
// unique in a merged Azure + GitHub list — and deliberately ignores
// ScopeDisplay, which is a presentation detail that can vary by config.
func (i Identity) SameItem(other Identity) bool {
	return i.Kind == other.Kind && i.Scope == other.Scope && i.ID == other.ID
}

// WorkItem is the neutral representation of an Azure DevOps work item (or its
// equivalent in another backend).
type WorkItem struct {
	Identity     Identity
	Title        string
	State        string
	WorkItemType string
	// StateCategory is the neutral semantic bucket for the work item's state.
	// Populated by the adapter at mapping time via azdevops.MapStateCategory.
	StateCategory StateCategory
	// ItemKind is the neutral semantic type enum for the work item.
	// Populated by the adapter at mapping time via azdevops.MapItemType.
	ItemKind        ItemType
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
	StatusCategory StateCategory // neutral semantic bucket derived from Status
	CreationDate   time.Time
	// SourceRefName and TargetRefName are plain branch names (e.g. "main"),
	// already stripped of any backend ref prefix (Azure's refs/heads/) at the
	// adapter boundary. The UI renders them verbatim.
	SourceRefName string
	TargetRefName string
	IsDraft       bool
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
	Kind        VoteKind // neutral semantic enum derived from Vote
}

// PipelineRun is the neutral representation of a pipeline/build run.
type PipelineRun struct {
	Identity       Identity
	BuildNumber    string
	Status         string
	Result         string
	RunStatus      RunStatus // neutral enum; populated by MapRunStatus at the adapter boundary
	// SourceBranch is a plain branch name, stripped of any backend ref prefix
	// (Azure's refs/heads/) at the adapter boundary.
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

// Iteration is the neutral representation of a single PR iteration (push).
// Each push to the source branch creates a new iteration; the adapter uses
// the latest iteration ID when fetching changed files.
type Iteration struct {
	ID          int
	Description string
}

// IterationChange is the neutral representation of a single file changed in a
// PR iteration. OriginalPath is non-empty for renamed files.
type IterationChange struct {
	ChangeID      int
	Path          string // the new (or only) file path
	GitObjectType string // "blob" for files, "tree" for folders
	ChangeType    string // "add", "edit", "delete", "rename"
	OriginalPath  string // non-empty on renames
	// Patch holds a ready-made unified-diff for this file when the backend can
	// supply one (GitHub's PR files API). When non-empty the diff view renders
	// it directly instead of fetching file content at branch refs. Azure leaves
	// this empty and the view computes the diff from fetched content as before.
	Patch string
}

// WorkItemTypeState is the neutral representation of a state that is valid for
// a given work item type (e.g. "Active", "Resolved", "Closed").
type WorkItemTypeState struct {
	Name     string
	Color    string
	Category string
}

// WorkItemComment is the neutral representation of a comment in a work item's
// Discussion section.
type WorkItemComment struct {
	Identity    Identity
	ID          int
	Text        string
	AuthorName  string
	CreatedDate time.Time
}
