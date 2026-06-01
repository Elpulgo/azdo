package azdevops

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClient_MetricsWorkItems_WIQLShape(t *testing.T) {
	var capturedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			bodyBytes, _ := io.ReadAll(r.Body)
			capturedBody = string(bodyBytes)
			json.NewEncoder(w).Encode(WIQLResponse{})
			return
		}
		json.NewEncoder(w).Encode(WorkItemsResponse{})
	}))
	defer server.Close()

	client := &Client{
		org:        "test-org",
		project:    "test-project",
		pat:        "test-pat",
		baseURL:    server.URL + "/test-org/test-project/_apis",
		httpClient: http.DefaultClient,
	}

	since := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	_, err := client.MetricsWorkItems(since)
	if err != nil {
		t.Fatalf("MetricsWorkItems() error = %v", err)
	}

	checks := []struct {
		needle string
		why    string
	}{
		{"@project", "must scope to @project to avoid cross-project duplicates"},
		{"'Active'", "must include Active state literal"},
		{"'Ready for Test'", "must include Ready for Test state literal (exact spelling)"},
		{"'Closed'", "must include Closed state for points-closed window"},
		{"2026-05-01", "must format the since date as YYYY-MM-DD"},
		{"Microsoft.VSTS.Common.ClosedDate", "must filter closed items by ClosedDate >= since"},
	}
	for _, c := range checks {
		if !strings.Contains(capturedBody, c.needle) {
			t.Errorf("WIQL missing %q (%s).\nBody: %s", c.needle, c.why, capturedBody)
		}
	}
	// New is explicitly excluded per spec
	if strings.Contains(capturedBody, "'New'") {
		t.Errorf("WIQL must not include 'New' state.\nBody: %s", capturedBody)
	}
	// No @Me filter — this query is org-wide
	if strings.Contains(capturedBody, "@Me") {
		t.Errorf("WIQL must not filter by @Me (this is the management view).\nBody: %s", capturedBody)
	}
}

func TestClient_MetricsWorkItems_BatchesOver200IDs(t *testing.T) {
	// Azure DevOps caps GetWorkItems at 200 IDs per request. With 250 IDs
	// returned by WIQL, the workitems endpoint should be hit twice.
	const totalIDs = 250
	workItemsCalls := 0
	var capturedIDsParam []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			refs := make([]WorkItemReference, totalIDs)
			for i := range refs {
				refs[i] = WorkItemReference{ID: i + 1, URL: ""}
			}
			json.NewEncoder(w).Encode(WIQLResponse{WorkItems: refs})
			return
		}
		// GET /workitems
		workItemsCalls++
		idsParam := r.URL.Query().Get("ids")
		capturedIDsParam = append(capturedIDsParam, idsParam)

		// Echo back items so the response size matches the request
		ids := strings.Split(idsParam, ",")
		resp := WorkItemsResponse{Count: len(ids)}
		for _, idStr := range ids {
			var id int
			fmt.Sscanf(idStr, "%d", &id)
			resp.Value = append(resp.Value, WorkItem{ID: id})
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := &Client{
		org:        "test-org",
		project:    "test-project",
		pat:        "test-pat",
		baseURL:    server.URL + "/test-org/test-project/_apis",
		httpClient: http.DefaultClient,
	}

	items, err := client.MetricsWorkItems(time.Now().Add(-14 * 24 * time.Hour))
	if err != nil {
		t.Fatalf("MetricsWorkItems() error = %v", err)
	}

	if workItemsCalls != 2 {
		t.Errorf("expected 2 batched calls (250 IDs / 200), got %d", workItemsCalls)
	}
	if len(items) != totalIDs {
		t.Errorf("expected %d items, got %d", totalIDs, len(items))
	}
	// First batch should have 200 IDs; second 50.
	if got := len(strings.Split(capturedIDsParam[0], ",")); got != 200 {
		t.Errorf("first batch IDs = %d, want 200", got)
	}
	if got := len(strings.Split(capturedIDsParam[1], ",")); got != 50 {
		t.Errorf("second batch IDs = %d, want 50", got)
	}
}

func TestMultiClient_MetricsWorkItems_MergedAndTagged(t *testing.T) {
	now := time.Now()
	alphaItems := []WorkItem{
		{ID: 100, Fields: WorkItemFields{Title: "Alpha", State: "Active", ChangedDate: now.Add(-2 * time.Hour)}},
	}
	betaItems := []WorkItem{
		{ID: 200, Fields: WorkItemFields{Title: "Beta", State: "Ready for Test", ChangedDate: now}},
	}

	alphaServer := newWorkItemServer(t, alphaItems)
	defer alphaServer.Close()
	betaServer := newWorkItemServer(t, betaItems)
	defer betaServer.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"alpha": alphaServer,
		"beta":  betaServer,
	})

	items, err := mc.MetricsWorkItems(now.Add(-14 * 24 * time.Hour))
	if err != nil {
		t.Fatalf("MetricsWorkItems failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	// Sorted by ChangedDate desc — Beta is newest
	if items[0].ID != 200 {
		t.Errorf("expected first item ID=200 (newest), got %d", items[0].ID)
	}
	// Project tagging
	if items[0].ProjectName != "beta" || items[1].ProjectName != "alpha" {
		t.Errorf("project tagging wrong: [0]=%q [1]=%q", items[0].ProjectName, items[1].ProjectName)
	}
}

func TestMultiClient_MetricsWorkItems_PartialFailure(t *testing.T) {
	now := time.Now()
	alphaItems := []WorkItem{
		{ID: 100, Fields: WorkItemFields{Title: "Alpha", State: "Active", ChangedDate: now}},
	}

	alphaServer := newWorkItemServer(t, alphaItems)
	defer alphaServer.Close()
	betaServer := newErrorServer(t)
	defer betaServer.Close()

	mc := newMultiClientWithServers(t, map[string]*httptest.Server{
		"alpha": alphaServer,
		"beta":  betaServer,
	})

	items, err := mc.MetricsWorkItems(now.Add(-14 * 24 * time.Hour))

	// PartialError pattern: partial data + structured error
	var pe *PartialError
	if err == nil {
		t.Fatal("expected PartialError, got nil")
	}
	if !errors.As(err, &pe) {
		t.Fatalf("expected *PartialError, got %T: %v", err, err)
	}
	if pe.Failed != 1 || pe.Total != 2 {
		t.Errorf("PartialError counts wrong: Failed=%d Total=%d", pe.Failed, pe.Total)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 partial item, got %d", len(items))
	}
}

func TestClient_MetricsWorkItems_NoResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(WIQLResponse{WorkItems: []WorkItemReference{}})
	}))
	defer server.Close()

	client := &Client{
		org:        "test-org",
		project:    "test-project",
		pat:        "test-pat",
		baseURL:    server.URL + "/test-org/test-project/_apis",
		httpClient: http.DefaultClient,
	}

	items, err := client.MetricsWorkItems(time.Now())
	if err != nil {
		t.Fatalf("MetricsWorkItems() error = %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}
