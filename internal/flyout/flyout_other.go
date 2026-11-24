//go:build !windows || !(amd64 || 386)

package flyout

import "errors"

// Flyout is only drawn on Windows on Intel and AMD processors: the calls
// into GDI+ pass floats the way only those processors' Windows calls carry
// them.
type Flyout struct{}

func Start(options Options) (*Flyout, error) {
	return nil, errors.New("the tray panel is not available on this system")
}

func (f *Flyout) Toggle() {}
func (f *Flyout) Close()  {}
