package browser

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// fakeExec records what command would have been run.
type fakeExec struct {
	name string
	args []string
}

func (f *fakeExec) command(name string, args ...string) *exec.Cmd {
	f.name = name
	f.args = args
	// Use a no-op command so Start() succeeds across platforms.
	return exec.Command("true")
}

func TestOpen_RejectsEmptyURL(t *testing.T) {
	if err := Open(""); err == nil {
		t.Fatal("expected error for empty URL, got nil")
	}
}

func TestOpen_RejectsNonHTTPScheme(t *testing.T) {
	cases := []string{
		"file:///etc/passwd",
		"javascript:alert(1)",
		"ftp://example.com",
		"not-a-url",
		"://broken",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if err := Open(c); err == nil {
				t.Errorf("expected error for URL %q, got nil", c)
			}
		})
	}
}

func TestOpen_AcceptsHTTPSchemes(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()

	fake := &fakeExec{}
	execCommand = fake.command

	for _, url := range []string{
		"https://dev.azure.com/org/proj/_workitems/edit/1",
		"http://example.com",
	} {
		fake.name = ""
		fake.args = nil
		if err := Open(url); err != nil {
			t.Errorf("unexpected error for %q: %v", url, err)
		}
		if fake.name == "" {
			t.Errorf("expected execCommand to be invoked for %q", url)
		}
	}
}

func TestOpen_UsesPlatformCommand(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()

	fake := &fakeExec{}
	execCommand = fake.command

	url := "https://dev.azure.com/org/proj"
	if err := Open(url); err != nil {
		t.Fatalf("Open returned error: %v", err)
	}

	switch runtime.GOOS {
	case "darwin":
		if fake.name != "open" {
			t.Errorf("on darwin expected command 'open', got %q", fake.name)
		}
		if len(fake.args) != 1 || fake.args[0] != url {
			t.Errorf("expected args [%q], got %v", url, fake.args)
		}
	case "windows":
		if fake.name != "rundll32" {
			t.Errorf("on windows expected command 'rundll32', got %q", fake.name)
		}
		joined := strings.Join(fake.args, " ")
		if !strings.Contains(joined, "url.dll,FileProtocolHandler") {
			t.Errorf("expected rundll32 url.dll,FileProtocolHandler invocation, got %v", fake.args)
		}
		if fake.args[len(fake.args)-1] != url {
			t.Errorf("expected URL as last argument, got %v", fake.args)
		}
	default:
		if fake.name != "xdg-open" {
			t.Errorf("on %s expected command 'xdg-open', got %q", runtime.GOOS, fake.name)
		}
		if len(fake.args) != 1 || fake.args[0] != url {
			t.Errorf("expected args [%q], got %v", url, fake.args)
		}
	}
}

func TestOpen_PropagatesStartError(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		// A command that doesn't exist - Start() will fail.
		return exec.Command("definitely-not-a-real-binary-azdo-test")
	}

	err := Open("https://dev.azure.com/org/proj")
	if err == nil {
		t.Fatal("expected error when launcher binary is missing, got nil")
	}
}
