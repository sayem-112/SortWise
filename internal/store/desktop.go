package store

import "context"

// DesktopSetUp reports whether Sortwise has added itself to Windows (the
// Start menu, and starting at sign-in) before, so it only does so once.
func (s *Store) DesktopSetUp(ctx context.Context) bool {
	return s.setting(ctx, "desktop_set_up") == "true"
}

func (s *Store) MarkDesktopSetUp(ctx context.Context) error {
	return s.setSetting(ctx, "desktop_set_up", "true")
}
