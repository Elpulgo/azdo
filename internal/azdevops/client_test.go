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

func TestClient_Get_ExpiredOrInvalidPAT(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message": "TF401019: The Git repository with name or identifier xyz does not exist or you do not have permissions"}`))
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

	// Check for user-friendly error message about PAT
	if !strings.Contains(err.Error(), "PAT") {
		t.Errorf("Expected error to mention PAT, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "expired") || !strings.Contains(err.Error(), "invalid") {
		t.Errorf("Expected error to mention 'expired' or 'invalid', got %q", err.Error())
	}
}

func TestClient_Get_InsufficientPermissions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message": "Access denied"}`))
	}))
	defer server.Close()

	client, err := NewClient("myorg", "myproject", "test-pat")
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	client.baseURL = server.URL

	_, err = client.get("/test")
	if err == nil {
		t.Error("Expected error for 403 response, got nil")
	}

	// Check for user-friendly error message about permissions
	if !strings.Contains(err.Error(), "PAT") {
		t.Errorf("Expected error to mention PAT, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "permission") {
		t.Errorf("Expected error to mention 'permission', got %q", err.Error())
	}
}

func TestClient_DoRequest_ExpiredOrInvalidPAT(t *testing.T) {
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

	_, err = client.doRequest("POST", "/test", nil)
	if err == nil {
		t.Error("Expected error for 401 response, got nil")
	}

	// Check for user-friendly error message about PAT
	if !strings.Contains(err.Error(), "PAT") {
		t.Errorf("Expected error to mention PAT, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "expired") || !strings.Contains(err.Error(), "invalid") {
		t.Errorf("Expected error to mention 'expired' or 'invalid', got %q", err.Error())
	}
}

func TestClient_DoRequest_InsufficientPermissions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message": "Access denied"}`))
	}))
	defer server.Close()

	client, err := NewClient("myorg", "myproject", "test-pat")
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	client.baseURL = server.URL

	_, err = client.doRequest("PUT", "/test", nil)
	if err == nil {
		t.Error("Expected error for 403 response, got nil")
	}

	// Check for user-friendly error message about permissions
	if !strings.Contains(err.Error(), "PAT") {
		t.Errorf("Expected error to mention PAT, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "permission") {
		t.Errorf("Expected error to mention 'permission', got %q", err.Error())
	}
}

func TestClient_ErrorMessages_DoNotLeakResponseBody(t *testing.T) {
	sensitiveData := "SENSITIVE_SECRET_DATA_12345"

	tests := []struct {
		name       string
		statusCode int
	}{
		{"401 Unauthorized", http.StatusUnauthorized},
		{"403 Forbidden", http.StatusForbidden},
		{"500 Internal Server Error", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(`{"secret": "` + sensitiveData + `"}`))
			}))
			defer server.Close()

			client, err := NewClient("myorg", "myproject", "test-pat")
			if err != nil {
				t.Fatalf("NewClient() failed: %v", err)
			}

			client.baseURL = server.URL

			_, err = client.get("/test")
			if err == nil {
				t.Error("Expected error, got nil")
			}

			// Verify sensitive data is NOT in the error message
			if strings.Contains(err.Error(), sensitiveData) {
				t.Errorf("Error message should NOT contain response body, but got %q", err.Error())
			}
		})
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

func TestFormatHTTPError_NotFound(t *testing.T) {
	err := formatHTTPError(http.StatusNotFound, []byte(`{}`))

	if !strings.Contains(err.Error(), "404") {
		t.Errorf("Expected error to contain '404', got %q", err.Error())
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Errorf("Expected error to mention 'not found', got %q", err.Error())
	}
	if !strings.Contains(strings.ToLower(err.Error()), "organization") && !strings.Contains(strings.ToLower(err.Error()), "project") {
		t.Errorf("Expected error to mention organization or project, got %q", err.Error())
	}
}

func TestFormatHTTPError_RateLimit(t *testing.T) {
	err := formatHTTPError(http.StatusTooManyRequests, []byte(`{}`))

	if !strings.Contains(err.Error(), "429") {
		t.Errorf("Expected error to contain '429', got %q", err.Error())
	}
	if !strings.Contains(strings.ToLower(err.Error()), "rate limit") {
		t.Errorf("Expected error to mention 'rate limit', got %q", err.Error())
	}
	if !strings.Contains(strings.ToLower(err.Error()), "wait") && !strings.Contains(strings.ToLower(err.Error()), "retry") {
		t.Errorf("Expected error to suggest waiting or retrying, got %q", err.Error())
	}
}

func TestFormatHTTPError_ServerError(t *testing.T) {
	err := formatHTTPError(http.StatusInternalServerError, []byte(`{}`))

	if !strings.Contains(err.Error(), "500") {
		t.Errorf("Expected error to contain '500', got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "Azure DevOps") {
		t.Errorf("Expected error to mention 'Azure DevOps', got %q", err.Error())
	}
}

func TestFormatHTTPError_ServiceUnavailable(t *testing.T) {
	err := formatHTTPError(http.StatusServiceUnavailable, []byte(`{}`))

	if !strings.Contains(err.Error(), "503") {
		t.Errorf("Expected error to contain '503', got %q", err.Error())
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unavailable") || !strings.Contains(strings.ToLower(err.Error()), "temporary") {
		t.Errorf("Expected error to mention service unavailability or temporary issue, got %q", err.Error())
	}
}
