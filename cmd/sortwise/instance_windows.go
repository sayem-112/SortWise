package main

import (
	"strings"

	"golang.org/x/sys/windows"
)

// claimInstance reports whether this is the only Sortwise starting or
// running on address. Two can start at once, as when the extension's Start
// button is pressed twice; the second then waits for the first rather than
// failing to open the port. release lets the claim go.
func claimInstance(address string) (release func(), only bool) {
	name, err := windows.UTF16PtrFromString(`Local\Sortwise-` + strings.ReplaceAll(address, ":", "-"))
	if err != nil {
		return func() {}, true
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if err == windows.ERROR_ALREADY_EXISTS {
		windows.CloseHandle(handle)
		return nil, false
	}
	if err != nil {
		return func() {}, true // can't tell; start as usual
	}
	return func() { windows.CloseHandle(handle) }, true
}
