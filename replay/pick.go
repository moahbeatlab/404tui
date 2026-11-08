package replay

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// errNoDialog says the computer has no program that can show a file dialog.
var errNoDialog = errors.New("no file dialog on this computer: type the path instead")

// pickFile opens the system's own file dialog on the computer that runs the program and returns the chosen path ("" when the dialog was cancelled).
func pickFile() (string, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("osascript", "-e", `POSIX path of (choose file with prompt "Choose the video of you playing" of type {"public.movie"})`)
	case "windows":
		cmd = exec.Command("powershell", "-NoProfile", "-Command", `Add-Type -AssemblyName System.Windows.Forms; $d = New-Object System.Windows.Forms.OpenFileDialog; $d.Filter = 'Video|*.mp4;*.mov;*.m4v;*.mkv;*.webm;*.avi|All files|*.*'; if ($d.ShowDialog() -eq 'OK') { $d.FileName }`)
	default:
		if p, err := exec.LookPath("zenity"); err == nil {
			cmd = exec.Command(p, "--file-selection", "--title=Choose the video of you playing")
		} else if p, err := exec.LookPath("kdialog"); err == nil {
			cmd = exec.Command(p, "--getopenfilename", ".", "video/*")
		} else {
			return "", errNoDialog
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) { // cancelled
			return "", nil
		}
		return "", errNoDialog
	}
	return strings.TrimSpace(string(out)), nil
}
