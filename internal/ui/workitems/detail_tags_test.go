package workitems

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/provider"
	tea "github.com/charmbracelet/bubbletea"
)

// adoTagServer stands in for the Azure DevOps work item API (the HTTP process
// boundary) behind a real azdevops adapter. GET returns tags as System.Tags;
// PATCH answers patchStatus (200 when zero). requests counts every call.
type adoTagServer struct {
	tags        string
	patchStatus int
	requests    int
}

func (s *adoTagServer) provider(t *testing.T) provider.Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests++
		if r.Method == http.MethodPatch && s.patchStatus != 0 {
			w.WriteHeader(s.patchStatus)
			w.Write([]byte(`{"message":"boom"}`))
			return
		}
		json.NewEncoder(w).Encode(azdevops.WorkItem{ID: 7, Fields: azdevops.WorkItemFields{Tags: s.tags}})
	}))
	t.Cleanup(srv.Close)
	mc, err := azdevops.NewMultiClient("org", []string{"testproject"}, "pat", nil)
	if err != nil {
		t.Fatal(err)
	}
	mc.ClientFor("testproject").SetBaseURL(srv.URL)
	return azdevops.NewAdapter(mc)
}

func newTagDetail(t *testing.T, srv *adoTagServer) *DetailModel {
	t.Helper()
	wi := newTestWI(7, "T", "Active", "Task")
	wi.Identity.Kind = provider.KindAzure
	wi.Tags = srv.tags
	m := NewDetailModel(srv.provider(t), wi)
	m.SetSize(100, 40)
	return m
}

func typeKeys(m *DetailModel, s string) *DetailModel {
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return m
}

// drain runs cmd and feeds each resulting message back into the model until no
// command is left, following the chain the bubbletea runtime would dispatch.
func drain(t *testing.T, m *DetailModel, cmd tea.Cmd) *DetailModel {
	t.Helper()
	for i := 0; cmd != nil; i++ {
		if i > 10 {
			t.Fatal("command chain did not settle")
		}
		msg := cmd()
		if _, ok := msg.(WorkItemStateChangedMsg); ok {
			return m // handled by the list, not the detail
		}
		m, cmd = m.Update(msg)
	}
	return m
}

// renderedTagsLine returns the Tags line of the detail content (not the editor).
func renderedTagsLine(m *DetailModel) string {
	for _, line := range strings.Split(m.viewport.View(), "\n") {
		if strings.Contains(line, "Tags:") {
			return line
		}
	}
	return ""
}

func TestDetailModel_LKeyOpensTagEditorWithCurrentTags_EscClosesWithoutProviderCall(t *testing.T) {
	srv := &adoTagServer{tags: "alpha; beta"}
	m := newTagDetail(t, srv)

	m = typeKeys(m, "l")
	if !m.tagEditor.IsVisible() {
		t.Fatal("Expected 'l' to open the tag editor")
	}
	editor := m.tagEditor.View()
	for _, want := range []string{"alpha", "beta"} {
		if !strings.Contains(editor, want) {
			t.Errorf("Expected tag editor to list %q, got:\n%s", want, editor)
		}
	}

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = drain(t, m, cmd)

	if m.tagEditor.IsVisible() {
		t.Error("Expected esc to close the tag editor")
	}
	if srv.requests != 0 {
		t.Errorf("Expected no provider call on esc, got %d requests", srv.requests)
	}
}

func TestDetailModel_InvalidTagShowsErrorKeepsEditorOpenWithoutProviderCall(t *testing.T) {
	srv := &adoTagServer{tags: "alpha"}
	m := newTagDetail(t, srv)

	m = typeKeys(m, "l")
	m = typeKeys(m, "--error")
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drain(t, m, cmd)

	if !m.tagEditor.IsVisible() {
		t.Error("Expected the editor to stay open after invalid input")
	}
	if m.tagEditor.Error() == "" || !strings.Contains(m.View(), m.tagEditor.Error()) {
		t.Errorf("Expected an inline error in the editor, got %q", m.tagEditor.Error())
	}
	if got := m.tagEditor.Value(); got != "--error" {
		t.Errorf("Expected the typed value to be kept, got %q", got)
	}
	if srv.requests != 0 {
		t.Errorf("Expected no provider call for invalid input, got %d requests", srv.requests)
	}
}

func TestDetailModel_TagAddSuccessShowsTagOnTagsLineAndStatus(t *testing.T) {
	srv := &adoTagServer{tags: "alpha"}
	m := newTagDetail(t, srv)

	m = typeKeys(m, "l")
	m = typeKeys(m, "  Beta-2 ")
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drain(t, m, cmd)

	if got := renderedTagsLine(m); !strings.Contains(got, "alpha, Beta-2") {
		t.Errorf("Expected Tags line to contain %q, got %q", "alpha, Beta-2", got)
	}
	if got := m.GetStatusMessage(); got != "Tag added" {
		t.Errorf("Expected status %q, got %q", "Tag added", got)
	}
}

func TestDetailModel_DeleteOnSelectedTagRemovesItFromTagsLine(t *testing.T) {
	srv := &adoTagServer{tags: "alpha; beta; gamma"}
	m := newTagDetail(t, srv)

	m = typeKeys(m, "l")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDelete})
	m = drain(t, m, cmd)

	if got := renderedTagsLine(m); !strings.Contains(got, "alpha, gamma") || strings.Contains(got, "beta") {
		t.Errorf("Expected Tags line %q to read \"alpha, gamma\" without beta", got)
	}
	if got := m.GetStatusMessage(); got != "Tag removed" {
		t.Errorf("Expected status %q, got %q", "Tag removed", got)
	}
}

func TestDetailModel_TagBackendErrorSetsErrorStatusAndKeepsTypedValue(t *testing.T) {
	srv := &adoTagServer{tags: "alpha", patchStatus: http.StatusInternalServerError}
	m := newTagDetail(t, srv)

	m = typeKeys(m, "l")
	m = typeKeys(m, "beta")
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drain(t, m, cmd)

	if got := m.GetStatusMessage(); !strings.HasPrefix(got, "Error") {
		t.Errorf("Expected an error status, got %q", got)
	}
	if got := m.tagEditor.Value(); got != "beta" {
		t.Errorf("Expected the typed value to be kept, got %q", got)
	}
	if got := renderedTagsLine(m); strings.Contains(got, "beta") {
		t.Errorf("Expected Tags line unchanged after a failed add, got %q", got)
	}
}

func TestDetailModel_CommentsLoadedWhileTagEditorOpenFillDiscussion(t *testing.T) {
	srv := &adoTagServer{tags: "alpha"}
	m := newTagDetail(t, srv)
	m.Init() // comments start loading; the fetch result is fed in below

	m = typeKeys(m, "l")
	m, _ = m.Update(commentsLoadedMsg{comments: sampleComments()})

	view := m.viewport.View()
	if strings.Contains(view, "Loading comments") || !strings.Contains(view, "Newest discussion point") {
		t.Errorf("Expected loaded comments in Discussion while the tag editor is open, got:\n%s", view)
	}
}

func TestDetailModel_TagResultArrivingWhileStatePickerOpenIsApplied(t *testing.T) {
	srv := &adoTagServer{tags: "alpha"}
	m := newTagDetail(t, srv)

	m = typeKeys(m, "l")
	m = typeKeys(m, "beta")
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m, cmd = m.Update(cmd()) // TagAddMsg starts the request
	result := cmd()

	// Close the editor and open the state picker before the result lands.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = m.Update(statesLoadedMsg{states: []provider.WorkItemTypeState{{Name: "Active"}, {Name: "Closed"}}})
	if !m.statePicker.IsVisible() {
		t.Fatal("precondition failed: state picker should be visible")
	}

	m, _ = m.Update(result)

	if got := m.GetStatusMessage(); got != "Tag added" {
		t.Errorf("Expected status %q, got %q", "Tag added", got)
	}
	if got := renderedTagsLine(m); !strings.Contains(got, "alpha, beta") {
		t.Errorf("Expected Tags line to contain %q, got %q", "alpha, beta", got)
	}
}
