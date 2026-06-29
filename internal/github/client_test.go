package github

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Constructor and field accessors
// ---------------------------------------------------------------------------

func TestNewClient_Fields(t *testing.T) {
	c := NewClient("acme", "widget", "ghp_secret")

	if c.Owner() != "acme" {
		t.Errorf("Owner() = %q, want %q", c.Owner(), "acme")
	}
	if c.Repo() != "widget" {
		t.Errorf("Repo() = %q, want %q", c.Repo(), "widget")
	}
	if c.token != "ghp_secret" {
		t.Errorf("token = %q, want %q", c.token, "ghp_secret")
	}
	if c.baseURL != defaultBaseURL {
		t.Errorf("baseURL = %q, want %q", c.baseURL, defaultBaseURL)
	}
	if c.httpClient == nil {
		t.Error("httpClient must not be nil")
	}
}

func TestClient_Scope(t *testing.T) {
	c := NewClient("octocat", "hello-world", "tok")
	if got := c.Scope(); got != "octocat/hello-world" {
		t.Errorf("Scope() = %q, want %q", got, "octocat/hello-world")
	}
}

func TestClient_SetBaseURL(t *testing.T) {
	c := NewClient("o", "r", "t")
	c.SetBaseURL("http://localhost:9999")
	if c.baseURL != "http://localhost:9999" {
		t.Errorf("baseURL after SetBaseURL = %q, want %q", c.baseURL, "http://localhost:9999")
	}
}

// ---------------------------------------------------------------------------
// Request headers
// ---------------------------------------------------------------------------

func TestClient_RequestHeaders(t *testing.T) {
	var capturedAuth, capturedAccept, capturedAPIVersion string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedAccept = r.Header.Get("Accept")
		capturedAPIVersion = r.Header.Get("X-GitHub-Api-Version")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "ghp_mytoken")
	c.SetBaseURL(srv.URL)

	if _, err := c.get("/test"); err != nil {
		t.Fatalf("get() error: %v", err)
	}

	if want := "Bearer ghp_mytoken"; capturedAuth != want {
		t.Errorf("Authorization = %q, want %q", capturedAuth, want)
	}
	if want := "application/vnd.github+json"; capturedAccept != want {
		t.Errorf("Accept = %q, want %q", capturedAccept, want)
	}
	if capturedAPIVersion != apiVersion {
		t.Errorf("X-GitHub-Api-Version = %q, want %q", capturedAPIVersion, apiVersion)
	}
}

// ---------------------------------------------------------------------------
// Successful GET + JSON decode
// ---------------------------------------------------------------------------

func TestClient_Get_ReturnsBody(t *testing.T) {
	want := `{"id":42,"login":"octocat"}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/repos/o/r/issues" {
			t.Errorf("path = %q, want /repos/o/r/issues", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(want))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	body, err := c.get("/repos/o/r/issues")
	if err != nil {
		t.Fatalf("get() error: %v", err)
	}
	if string(body) != want {
		t.Errorf("body = %q, want %q", string(body), want)
	}
}

func TestClient_GetJSON_Decodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"number":7,"title":"Fix the thing"}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	var dst struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
	}
	if err := c.getJSON("/repos/o/r/issues/7", &dst); err != nil {
		t.Fatalf("getJSON() error: %v", err)
	}
	if dst.Number != 7 {
		t.Errorf("Number = %d, want 7", dst.Number)
	}
	if dst.Title != "Fix the thing" {
		t.Errorf("Title = %q, want %q", dst.Title, "Fix the thing")
	}
}

// ---------------------------------------------------------------------------
// Non-2xx status codes → apiError
// ---------------------------------------------------------------------------

func TestClient_Get_401_MentionsToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "bad-token")
	c.SetBaseURL(srv.URL)

	_, err := c.get("/repos/o/r/issues")
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention 401: %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "token") {
		t.Errorf("error should mention token: %v", err)
	}
}

func TestClient_Get_403_MentionsScopes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	_, err := c.get("/repos/o/r/issues")
	if err == nil {
		t.Fatal("expected error for 403, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention 403: %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "scope") {
		t.Errorf("error should mention scopes: %v", err)
	}
}

func TestClient_Get_404_MentionsRepository(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	_, err := c.get("/repos/o/r/issues")
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention 404: %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "repository") && !strings.Contains(strings.ToLower(err.Error()), "repo") {
		t.Errorf("error should mention repository: %v", err)
	}
}

func TestClient_Get_429_MentionsRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	_, err := c.get("/repos/o/r/issues")
	if err == nil {
		t.Fatal("expected error for 429, got nil")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error should mention 429: %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "rate limit") {
		t.Errorf("error should mention rate limit: %v", err)
	}
}

func TestClient_Get_500_MentionsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	_, err := c.get("/repos/o/r/issues")
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention 500: %v", err)
	}
}

func TestClient_Get_UnknownStatus_IncludesCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(418) // I'm a teapot
	}))
	defer srv.Close()

	c := NewClient("o", "r", "tok")
	c.SetBaseURL(srv.URL)

	_, err := c.get("/test")
	if err == nil {
		t.Fatal("expected error for 418, got nil")
	}
	if !strings.Contains(err.Error(), "418") {
		t.Errorf("error should include status code 418: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Response body not leaked in error messages
// ---------------------------------------------------------------------------

func TestClient_ErrorsDoNotLeakResponseBody(t *testing.T) {
	secret := "SUPER_SECRET_SERVER_DATA_XYZ"

	for _, code := range []int{401, 403, 500} {
		code := code
		t.Run(http.StatusText(code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
				w.Write([]byte(`{"message":"` + secret + `"}`))
			}))
			defer srv.Close()

			c := NewClient("o", "r", "tok")
			c.SetBaseURL(srv.URL)

			_, err := c.get("/test")
			if err == nil {
				t.Fatalf("expected error for %d, got nil", code)
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("error message must not contain response body: %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Transport / network errors
// ---------------------------------------------------------------------------

func TestClient_Get_NetworkError(t *testing.T) {
	c := NewClient("o", "r", "tok")
	c.SetBaseURL("http://localhost:1") // nothing listening here

	_, err := c.get("/test")
	if err == nil {
		t.Fatal("expected network error, got nil")
	}
	if !strings.Contains(err.Error(), "github:") {
		t.Errorf("network error should be wrapped with github: prefix: %v", err)
	}
}

func TestClient_Get_InvalidURL(t *testing.T) {
	c := NewClient("o", "r", "tok")
	c.SetBaseURL("://bad-url")

	_, err := c.get("/test")
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

// ---------------------------------------------------------------------------
// apiError direct tests
// ---------------------------------------------------------------------------

func TestApiError_StatusCodes(t *testing.T) {
	cases := []struct {
		code        int
		wantContain string
	}{
		{http.StatusUnauthorized, "401"},
		{http.StatusForbidden, "403"},
		{http.StatusNotFound, "404"},
		{http.StatusTooManyRequests, "429"},
		{http.StatusInternalServerError, "500"},
		{http.StatusServiceUnavailable, "503"},
		{422, "422"},
	}
	for _, tc := range cases {
		err := apiError(tc.code)
		if err == nil {
			t.Errorf("apiError(%d) = nil, want non-nil", tc.code)
			continue
		}
		if !strings.Contains(err.Error(), tc.wantContain) {
			t.Errorf("apiError(%d) = %q, want to contain %q", tc.code, err.Error(), tc.wantContain)
		}
	}
}
