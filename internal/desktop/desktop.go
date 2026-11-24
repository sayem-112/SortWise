// Package desktop fits Sortwise into Windows without an installer: a Start
// menu shortcut, which is how Windows search finds an app, and an entry
// that starts Sortwise quietly in the tray when you sign in.
//
// Both point at whichever sortwise.exe last ran, so moving the file and
// starting it again from its new place keeps them working.
package desktop

// State is how this copy of Sortwise is set up in Windows.
type State struct {
	// Available is false where Sortwise can't be set up: on another system,
	// or in a temporary build such as `go run`.
	Available bool `json:"available"`
	// StartMenu is whether the Start menu shortcut exists.
	StartMenu bool `json:"startMenu"`
	// Startup is whether Sortwise starts when you sign in. Turning it off in
	// Task Manager's Startup apps counts as off.
	Startup bool `json:"startup"`
}

// Integration sets up one copy of sortwise.exe in Windows.
type Integration struct {
	exe string
	// args are the flags to start it with (--port, --data-dir).
	args []string

	// Where the entries live. Tests point these somewhere harmless.
	runKey      string // HKEY_CURRENT_USER key listing what starts at sign-in
	approvedKey string // where Task Manager records startup apps turned off
	valueName   string
	shortcut    string // the .lnk file
}
