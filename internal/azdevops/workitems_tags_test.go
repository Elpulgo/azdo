package azdevops

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// tagServer is an httptest stand-in for the Azure DevOps work item API (the
// HTTP process boundary). GET returns currentTags as System.Tags; PATCH bodies
// are recorded in patches and answered with patchStatus (200 when zero).
type tagServer struct {
	currentTags string
	patchStatus int
	patches     []string
}

func (s *tagServer) client(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			json.NewEncoder(w).Encode(WorkItem{ID: 42, Fields: WorkItemFields{Tags: s.currentTags}})
		case http.MethodPatch:
			body, _ := io.ReadAll(r.Body)
			s.patches = append(s.patches, string(body))
			if s.patchStatus != 0 {
				w.WriteHeader(s.patchStatus)
				w.Write([]byte(`{"message":"boom"}`))
				return
			}
			w.Write([]byte(`{"id":42}`))
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
	t.Cleanup(srv.Close)
	return &Client{
		org:        "test-org",
		project:    "test-project",
		pat:        "test-pat",
		baseURL:    srv.URL + "/test-org/test-project/_apis",
		httpClient: http.DefaultClient,
	}
}

// patchedTags decodes the System.Tags value from a single recorded PATCH.
func (s *tagServer) patchedTags(t *testing.T) string {
	t.Helper()
	if len(s.patches) != 1 {
		t.Fatalf("expected exactly one PATCH, got %d: %v", len(s.patches), s.patches)
	}
	var ops []struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(s.patches[0]), &ops); err != nil {
		t.Fatalf("PATCH body is not a JSON Patch document: %v (%s)", err, s.patches[0])
	}
	if len(ops) != 1 || ops[0].Op != "replace" || ops[0].Path != "/fields/System.Tags" {
		t.Fatalf("PATCH = %s, want a single replace of /fields/System.Tags", s.patches[0])
	}
	return ops[0].Value
}

func TestClient_AddWorkItemTag_KeepsExistingTags(t *testing.T) {
	tests := []struct {
		name    string
		current string
		add     string
		want    string
	}{
		{"appended after existing tags", "a; b", "c", "a; b; c"},
		{"item without tags gets only the new tag", "", "c", "c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &tagServer{currentTags: tt.current}
			if err := s.client(t).AddWorkItemTag(42, tt.add); err != nil {
				t.Fatalf("AddWorkItemTag() error = %v", err)
			}
			if got := s.patchedTags(t); got != tt.want {
				t.Errorf("System.Tags = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClient_AddWorkItemTag_AlreadyPresentCaseInsensitive_SendsNoPatch(t *testing.T) {
	s := &tagServer{currentTags: "a; Frontend"}
	if err := s.client(t).AddWorkItemTag(42, "frontend"); err != nil {
		t.Fatalf("AddWorkItemTag() error = %v", err)
	}
	if len(s.patches) != 0 {
		t.Errorf("expected no PATCH, got %v", s.patches)
	}
}

func TestClient_RemoveWorkItemTag_KeepsOtherTagsCaseInsensitive(t *testing.T) {
	s := &tagServer{currentTags: "a; b; c"}
	if err := s.client(t).RemoveWorkItemTag(42, "B"); err != nil {
		t.Fatalf("RemoveWorkItemTag() error = %v", err)
	}
	if got := s.patchedTags(t); got != "a; c" {
		t.Errorf("System.Tags = %q, want %q", got, "a; c")
	}
}

func TestClient_RemoveWorkItemTag_AbsentTag_SendsNoPatch(t *testing.T) {
	s := &tagServer{currentTags: "a; b"}
	if err := s.client(t).RemoveWorkItemTag(42, "x"); err != nil {
		t.Fatalf("RemoveWorkItemTag() error = %v", err)
	}
	if len(s.patches) != 0 {
		t.Errorf("expected no PATCH, got %v", s.patches)
	}
}

func TestClient_WorkItemTag_PatchErrorIsReturned(t *testing.T) {
	tests := []struct {
		name string
		call func(c *Client) error
	}{
		{"add", func(c *Client) error { return c.AddWorkItemTag(42, "c") }},
		{"remove", func(c *Client) error { return c.RemoveWorkItemTag(42, "a") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &tagServer{currentTags: "a; b", patchStatus: http.StatusBadRequest}
			err := tt.call(s.client(t))
			if err == nil {
				t.Fatal("expected an error when the PATCH fails, got nil")
			}
			if !strings.Contains(err.Error(), "failed to update work item tags") {
				t.Errorf("error = %v, want it to mention the tag update", err)
			}
		})
	}
}
