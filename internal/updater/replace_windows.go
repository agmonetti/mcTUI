//go:build windows

package updater

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExecutablePath returns the current running binary path.
// If the running executable was renamed to .old during update replacement,
// the original executable target path is returned.
func ExecutablePath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	if strings.HasSuffix(strings.ToLower(p), ".old") {
		trimmed := p[:len(p)-4]
		if _, err := os.Stat(trimmed); err == nil {
			return trimmed, nil
		}
	}
	return p, nil
}

// Restart launches the specified executable with the given args on Windows.
func Restart(binary string, args []string) error {
	cmdArgs := []string{}
	if len(args) > 1 {
		cmdArgs = args[1:]
	}
	cmd := exec.Command(binary, cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
