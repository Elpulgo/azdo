package azdevops

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name        string
		org         string
		project     string
		pat         string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid client",
			org:     "myorg",
			project: "myproject",
			pat:     "test-pat-token",
			wantErr: false,
		},
		{
			name:        "empty org",
			org:         "",
			project:     "myproject",
			pat:         "test-pat-token",
			wantErr:     true,
			errContains: "organization",
		},
		{
			name:        "empty project",
			org:         "myorg",
			project:     "",
			pat:         "test-pat-token",
			wantErr:     true,
			errContains: "project",
		},
		{
			name:        "empty PAT",
			org:         "myorg",
			project:     "myproject",
			pat:         "",
			wantErr:     true,
			errContains: "PAT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.org, tt.project, tt.pat)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error to contain %q, got %q", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("NewClient() failed: %v", err)
			}

			if client == nil {
				t.Fatal("Expected client to be non-nil")
			}
		})
	}
}

func TestClient_GetOrg(t *testing.T) {
	client, err := NewClient("myorg", "myproject", "test-pat")
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	if got := client.GetOrg(); got != "myorg" {
		t.Errorf("GetOrg() = %q, want %q", got, "myorg")
	}
}

func TestClient_GetProject(t *testing.T) {
	client, err := NewClient("myorg", "myproject", "test-pat")
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	if got := client.GetProject(); got != "myproject" {
		t.Errorf("GetProject() = %q, want %q", got, "myproject")
	}
}

func TestClient_BaseURL(t *testing.T) {
	client, err := NewClient("myorg", "myproject", "test-pat")
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	expectedBaseURL := "https://dev.azure.com/myorg/myproject/_apis"
	if client.baseURL != expectedBaseURL {
		t.Errorf("Expected baseURL to be %q, got %q", expectedBaseURL, client.baseURL)
	}
}

func TestClient_AuthHeader(t *testing.T) {
	pat := "my-secret-token"

	// Create a test server to inspect the request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			t.Error("Authorization header is missing")
		}

		// Verify it's Basic auth with correct format
		if !strings.HasPrefix(authHeader, "Basic ") {
			t.Errorf("Expected Authorization header to start with 'Basic ', got %q", authHeader)
		}

		// Decode and verify the token
		encodedToken := strings.TrimPrefix(authHeader, "Basic ")
		decoded, err := base64.StdEncoding.DecodeString(encodedToken)
		if err != nil {
			t.Errorf("Failed to decode auth token: %v", err)
		}

		// Azure DevOps uses ":{PAT}" format for basic auth
		expectedAuth := ":" + pat
		if string(decoded) != expectedAuth {
			t.Errorf("Expected decoded auth to be %q, got %q", expectedAuth, string(decoded))
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"value": []}`))
	}))
	defer server.Close()

	client, err := NewClient("myorg", "myproject", pat)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	// Override baseURL to use test server
	client.baseURL = server.URL

	// Make a GET request
	_, err = client.get("/test")
	if err != nil {
		t.Fatalf("get() failed: %v", err)
	}
}

func TestClient_Get_Success(t *testing.T) {
	responseBody := `{"id": "123", "name": "test-item"}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify HTTP method
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}

		// Verify path
		if r.URL.Path != "/test/endpoint" {
			t.Errorf("Expected path /test/endpoint, got %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(responseBody))
	}))
	defer server.Close()

	client, err := NewClient("myorg", "myproject", "test-pat")
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	client.baseURL = server.URL

	body, err := client.get("/test/endpoint")
	if err != nil {
		t.Fatalf("get() failed: %v", err)
	}

	if string(body) != responseBody {
		t.Errorf("Expected response body %q, got %q", responseBody, string(body))
	}
}

func TestClient_Get_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message": "Unauthorized"}`))
	}))
	defer server.Close()

	client, err := NewClient("myorg", "myproject", "test-pat")
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	client.baseURL = server.URL

	_, err = client.get("/test")
	if err == nil {
		t.Error("Expected error for 401 response, got nil")
	}

	if !strings.Contains(err.Error(), "401") {
		t.Errorf("Expected error to contain '401', got %q", err.Error())
	}
}

func TestClient_Get_InvalidURL(t *testing.T) {
	client, err := NewClient("myorg", "myproject", "test-pat")
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	// Set invalid baseURL
	client.baseURL = "://invalid-url"

	_, err = client.get("/test")
	if err == nil {
		t.Error("Expected error for invalid URL, got nil")
	}
}

func TestClient_Get_WithAPIVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for api-version query parameter
		apiVersion := r.URL.Query().Get("api-version")
		if apiVersion == "" {
			t.Error("api-version query parameter is missing")
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"value": []}`))
	}))
	defer server.Close()

	client, err := NewClient("myorg", "myproject", "test-pat")
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	client.baseURL = server.URL

	_, err = client.get("/test?api-version=7.0")
	if err != nil {
		t.Fatalf("get() failed: %v", err)
	}
}

func TestClient_Get_NetworkError(t *testing.T) {
	client, err := NewClient("myorg", "myproject", "test-pat")
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	// Use a URL that will fail
	client.baseURL = "http://localhost:1"

	_, err = client.get("/test")
	if err == nil {
		t.Error("Expected network error, got nil")
	}
}

func TestClient_Get_ContentTypeHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check Content-Type header
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Expected Content-Type to be 'application/json', got %q", contentType)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client, err := NewClient("myorg", "myproject", "test-pat")
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	client.baseURL = server.URL

	_, err = client.get("/test")
	if err != nil {
		t.Fatalf("get() failed: %v", err)
	}
}
