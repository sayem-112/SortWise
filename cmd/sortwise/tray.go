package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"syscall"

	"fyne.io/systray"

	"sortwise/internal/config"
)

//go:embed icon.ico
var trayIcon []byte

// runInTray runs Sortwise with an icon by the clock instead of a console
// window: Open Sortwise brings up the app, Quit stops it.
func runInTray(cfg config.Config) {
	ctx, cancel := context.WithCancel(context.Background())
	url := "http://" + cfg.Address
	systray.Run(func() {
		systray.SetIcon(trayIcon)
		systray.SetTitle("Sortwise")
		systray.SetTooltip("Sortwise is starting…")
		open := systray.AddMenuItem("Open Sortwise", "Open your library in the browser")
		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit Sortwise", "Stop Sortwise; new bookmarks wait until it runs again")
		systray.SetOnTapped(func() { openBrowser(url) })

		go func() {
			err := serve(ctx, cfg, func() { systray.SetTooltip("Sortwise is running") })
			if err != nil {
				slog.Error("Sortwise stopped", "error", err)
				showError("Sortwise stopped", err.Error())
			}
			systray.Quit()
		}()
		go func() {
			for {
				select {
				case <-open.ClickedCh:
					openBrowser(url)
				case <-quit.ClickedCh:
					cancel()
					return
				}
			}
		}()
	}, cancel)
}

// listen opens the server's port with a readable error when another program
// already uses it.
func listen(address string) (net.Listener, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		var errno syscall.Errno
		if errors.As(err, &errno) && (errno == 10048 || errno == syscall.EADDRINUSE) {
			return nil, fmt.Errorf("another program is using %s. Close it, or start Sortwise with --port and a different number", address)
		}
		return nil, err
	}
	return listener, nil
}
