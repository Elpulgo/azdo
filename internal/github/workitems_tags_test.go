package github

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_AddWorkItemTag_PostsSingleLabelToIssue(t *testing.T) {
	var capturedMethod, capturedPath string
	var capturedBody map[string][]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"name":"existing"},{"name":"x"}]`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	if err := c.AddWorkItemTag(7, "x"); err != nil {
		t.Fatalf("AddWorkItemTag() error = %v", err)
	}

	if capturedMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", capturedMethod)
	}
	if capturedPath != "/repos/o/r/issues/7/labels" {
		t.Errorf("path = %q, want /repos/o/r/issues/7/labels", capturedPath)
	}
	labels := capturedBody["labels"]
	if len(capturedBody) != 1 || len(labels) != 1 || labels[0] != "x" {
		t.Errorf(`body = %v, want {"labels":["x"]}`, capturedBody)
	}
}

func TestClient_RemoveWorkItemTag_DeletesEscapedLabelFromIssue(t *testing.T) {
	var capturedMethod, capturedPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	if err := c.RemoveWorkItemTag(7, "area/ui-å"); err != nil {
		t.Fatalf("RemoveWorkItemTag() error = %v", err)
	}

	if capturedMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", capturedMethod)
	}
	const wantPath = "/repos/o/r/issues/7/labels/area%2Fui-%C3%A5"
	if capturedPath != wantPath {
		t.Errorf("path = %q, want %q", capturedPath, wantPath)
	}
}

func TestClient_RemoveWorkItemTag_LabelNotOnIssue_IsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Label does not exist"}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	if err := c.RemoveWorkItemTag(7, "gone"); err != nil {
		t.Errorf("RemoveWorkItemTag() on 404 error = %v, want nil", err)
	}
}

func TestClient_WorkItemTag_APIErrorsAreReturned(t *testing.T) {
	ops := map[string]func(c *Client) error{
		"add":    func(c *Client) error { return c.AddWorkItemTag(7, "x") },
		"remove": func(c *Client) error { return c.RemoveWorkItemTag(7, "x") },
	}
	statuses := []int{http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusInternalServerError}

	for name, op := range ops {
		for _, status := range statuses {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				w.Write([]byte(`{"message":"nope"}`))
			}))

			c := NewClient("o", "r", "tok")
			c.SetBaseURL(srv.URL)

			err := op(c)
			srv.Close()

			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != status {
				t.Errorf("%s with HTTP %d: error = %v, want *APIError with status %d", name, status, err, status)
			}
		}
	}
}
