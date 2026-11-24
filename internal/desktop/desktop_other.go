//go:build !windows

package desktop

// ForThisCopy is nil away from Windows: there is nothing to set up.
func ForThisCopy(args []string) *Integration { return nil }

func (i *Integration) State() State               { return State{} }
func (i *Integration) SetStartMenu(on bool) error { return nil }
func (i *Integration) SetStartup(on bool) error   { return nil }
func (i *Integration) Refresh(first bool) error   { return nil }
