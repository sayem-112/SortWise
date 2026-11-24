package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync/atomic"
	"syscall"

	"fyne.io/systray"

	"sortwise/internal/buildinfo"
	"sortwise/internal/config"
	"sortwise/internal/desktop"
	"sortwise/internal/flyout"
)

//go:embed icon.ico
var trayIcon []byte

// runInTray runs Sortwise with an icon by the clock instead of a console
// window. Clicking the icon opens a small panel with what Sortwise is doing
// and its controls; right-clicking shows a plain menu to open or quit.
func runInTray(cfg config.Config) {
	ctx, cancel := context.WithCancel(context.Background())
	url := "http://" + cfg.Address
	desk := desktop.ForThisCopy(cfg.Carried)
	var running atomic.Pointer[library]
	systray.Run(func() {
		systray.SetIcon(trayIcon)
		systray.SetTitle("Sortwise")
		systray.SetTooltip("Sortwise is starting…")
		open := systray.AddMenuItem("Open Sortwise", "Open your library in the browser")
		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit Sortwise", "Stop Sortwise; new bookmarks wait until it runs again")

		options := flyout.Options{
			Icon:    trayIcon,
			Version: versionLabel(),
			Status: func() flyout.Status {
				if lib := running.Load(); lib != nil {
					return lib.status()
				}
				return flyout.Status{Headline: "Starting…", Tone: flyout.Quiet, Detail: "Opening your library"}
			},
			Open:         func() { openBrowser(url) },
			OpenSettings: func() { openBrowser(url + "/settings") },
			OpenUpdate: func() {
				if lib := running.Load(); lib != nil && lib.updates != nil {
					if newer := lib.updates.State(ctx).Available; newer != nil {
						openBrowser(newer.URL)
					}
				}
			},
			Quit: cancel,
		}
		if desk != nil {
			options.Startup = func() bool { return desk.State().Startup }
			options.SetStartup = desk.SetStartup
		}
		panel, err := flyout.Start(options)
		if err != nil {
			slog.Warn("The tray panel could not open; clicking the icon opens the library instead", "error", err)
			systray.SetOnTapped(func() { openBrowser(url) })
		} else {
			systray.SetOnTapped(panel.Toggle)
		}

		go func() {
			err := serve(ctx, cfg, func(lib *library) {
				running.Store(lib)
				systray.SetTooltip("Sortwise is running")
			}, desk)
			if err != nil {
				slog.Error("Sortwise stopped", "error", err)
				showError("Sortwise stopped", err.Error())
			}
			if panel != nil {
				panel.Close()
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

// versionLabel is the version as the panel shows it.
func versionLabel() string {
	if buildinfo.Version == "dev" {
		return "dev build"
	}
	return "v" + buildinfo.Version
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
