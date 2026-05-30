package browser

import (
	"os/exec"
	"runtime"
)

// Opener is the function used to open URLs. Replace in tests to capture calls.
var Opener func(url string) error = openWithSystem

// Open opens url in the user's default browser.
func Open(url string) error {
	return Opener(url)
}

func openWithSystem(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
