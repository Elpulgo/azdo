// Package browser opens URLs in the user's default web browser across
// macOS, Linux, and Windows. It hands off to the OS handler so whichever
// browser the user has configured as default (Firefox, Chrome, Safari,
// Edge, etc.) is what opens the URL.
package browser

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
)

// execCommand is a package-level seam so tests can intercept the launcher
// invocation without actually spawning a browser.
var execCommand = exec.Command

// Open launches the given URL in the user's default browser. The call
// returns as soon as the launcher process has been started — it does not
// wait for the browser to finish opening.
func Open(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("browser: empty URL")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("browser: invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("browser: only http/https URLs are supported, got %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return fmt.Errorf("browser: URL missing host: %q", rawURL)
	}

	name, args := platformCommand(rawURL)
	cmd := execCommand(name, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("browser: failed to launch %s: %w", name, err)
	}
	return nil
}

// platformCommand returns the OS-appropriate launcher command for opening
// a URL with the user's default browser.
func platformCommand(rawURL string) (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{rawURL}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", rawURL}
	default:
		return "xdg-open", []string{rawURL}
	}
}
