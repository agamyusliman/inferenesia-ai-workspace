package core

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// PickDirectory opens a native folder chooser and returns the absolute path.
// Empty path means the user cancelled (not an error).
func (s *Service) PickDirectory() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return pickDirDarwin()
	case "windows":
		return pickDirWindows()
	default:
		return pickDirLinux()
	}
}

func pickDirDarwin() (string, error) {
	// osascript: POSIX path of chosen folder; empty on cancel. `tell app "System Events"`
	// forces the dialog to a real GUI process so it does not hide behind other
	// windows or get denied when the caller has no dock icon (headless --serve).
	// Hard cap 5 minutes so a forgotten dialog cannot hang the UI forever.
	script := `tell application "System Events"
  activate
  try
    set p to POSIX path of (choose folder with prompt "Open workspace")
    return p
  on error number -128
    return ""
  end try
end tell`
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("core: folder picker timed out (5m). Type the path in Open workspace instead")
	}
	if err != nil {
		return "", fmt.Errorf("core: folder picker: %w", err)
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return "", nil
	}
	return filepath.Clean(p), nil
}

func pickDirWindows() (string, error) {
	// PowerShell FolderBrowserDialog (works without WinForms app model in many hosts).
	ps := `
Add-Type -AssemblyName System.Windows.Forms
$d = New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description = 'Open workspace'
$d.ShowNewFolderButton = $true
if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
  Write-Output $d.SelectedPath
}
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("core: folder picker: %w", err)
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return "", nil
	}
	return filepath.Clean(p), nil
}

func pickDirLinux() (string, error) {
	// Prefer zenity, then kdialog.
	if path, err := tryCmd("zenity", "--file-selection", "--directory", "--title=Open workspace"); err == nil {
		return path, nil
	}
	if path, err := tryCmd("kdialog", "--getexistingdirectory", "."); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("core: no folder picker (install zenity or kdialog)")
}

func tryCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	p := strings.TrimSpace(stdout.String())
	if p == "" {
		return "", fmt.Errorf("cancelled")
	}
	return filepath.Clean(p), nil
}
