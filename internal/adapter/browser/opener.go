// Package browser implements the out.BrowserOpener driven port by shelling
// out to the OS-native "open a URL" command.
package browser

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/Emanuel3k/spotify-manager/internal/core/port/out"
)

// Opener launches the default web browser for the current OS.
type Opener struct{}

var _ out.BrowserOpener = Opener{}

// New builds an Opener.
func New() Opener { return Opener{} }

func (Opener) Open(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		// "" is the (required, empty) window title argument for `start`.
		cmd = exec.Command("cmd", "/c", "start", "", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default: // linux and other unix-likes
		cmd = exec.Command("xdg-open", url)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}
