package desktop

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	runKey      = `Software\Microsoft\Windows\CurrentVersion\Run`
	approvedKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
)

// ForThisCopy returns the running sortwise.exe's integration, started with
// args, or nil when this copy should not be set up: a temporary build that
// will be gone soon, such as one made by `go run`.
func ForThisCopy(args []string) *Integration {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	temp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		temp = os.TempDir()
	}
	if inside(exe, temp) {
		return nil
	}
	programs, err := windows.KnownFolderPath(windows.FOLDERID_Programs, 0)
	if err != nil {
		return nil
	}
	return &Integration{
		exe:         exe,
		args:        args,
		runKey:      runKey,
		approvedKey: approvedKey,
		valueName:   "Sortwise",
		shortcut:    filepath.Join(programs, "Sortwise.lnk"),
	}
}

func inside(path, folder string) bool {
	relative, err := filepath.Rel(folder, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (i *Integration) State() State {
	return State{Available: true, StartMenu: i.hasShortcut(), Startup: i.startsAtSignIn() && !i.turnedOffInTaskManager()}
}

// SetStartMenu adds or removes the Start menu shortcut.
func (i *Integration) SetStartMenu(on bool) error {
	if on {
		return i.writeShortcut()
	}
	if err := os.Remove(i.shortcut); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// SetStartup turns starting at sign-in on or off. Turning it on also clears
// Task Manager's record of it being turned off, so it really does start.
func (i *Integration) SetStartup(on bool) error {
	if err := deleteValue(i.approvedKey, i.valueName); err != nil {
		return err
	}
	if !on {
		return deleteValue(i.runKey, i.valueName)
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, i.runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(i.valueName, i.startupCommand())
}

// Refresh points the shortcut and the startup entry at this copy of
// sortwise.exe, so they follow it when it moves. On the first run (first)
// it adds both; after that it only updates the ones still there, so what
// you turned off stays off. A startup entry turned off in Task Manager
// stays off too.
func (i *Integration) Refresh(first bool) error {
	var problems []error
	if first || i.hasShortcut() {
		problems = append(problems, i.writeShortcut())
	}
	if first || i.startsAtSignIn() {
		key, _, err := registry.CreateKey(registry.CURRENT_USER, i.runKey, registry.SET_VALUE)
		if err == nil {
			err = key.SetStringValue(i.valueName, i.startupCommand())
			key.Close()
		}
		problems = append(problems, err)
	}
	return errors.Join(problems...)
}

// startupCommand starts Sortwise in the tray without opening the browser.
func (i *Integration) startupCommand() string {
	return commandLine(i.exe, append([]string{"--no-open"}, i.args...))
}

func commandLine(exe string, args []string) string {
	return strings.TrimSpace(`"` + exe + `" ` + quoted(args))
}

func quoted(args []string) string {
	parts := make([]string, len(args))
	for index, arg := range args {
		parts[index] = syscall.EscapeArg(arg)
	}
	return strings.Join(parts, " ")
}

func (i *Integration) hasShortcut() bool {
	_, err := os.Stat(i.shortcut)
	return err == nil
}

func (i *Integration) startsAtSignIn() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, i.runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	_, _, err = key.GetStringValue(i.valueName)
	return err == nil
}

// turnedOffInTaskManager reads the flag Task Manager keeps for a startup
// app: its first byte is odd when the app is turned off.
func (i *Integration) turnedOffInTaskManager() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, i.approvedKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	flags, _, err := key.GetBinaryValue(i.valueName)
	return err == nil && len(flags) > 0 && flags[0]&1 == 1
}

func (i *Integration) writeShortcut() error {
	return writeShortcut(i.shortcut, shortcut{
		target:      i.exe,
		args:        quoted(i.args),
		folder:      filepath.Dir(i.exe),
		description: "Open your Sortwise library",
	})
}

func deleteValue(path, name string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.DeleteValue(name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}
