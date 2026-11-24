package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"sortwise/internal/ai"
	"sortwise/internal/avatars"
	"sortwise/internal/buildinfo"
	"sortwise/internal/config"
	"sortwise/internal/credentials"
	"sortwise/internal/database"
	"sortwise/internal/desktop"
	"sortwise/internal/httpapi"
	"sortwise/internal/semantic"
	"sortwise/internal/store"
	"sortwise/internal/updates"
	"sortwise/internal/xembed"
)

func main() {
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		fail(err)
	}
	closeLog := setupLogging(cfg)
	defer closeLog()

	if cfg.Command == "restore" {
		if err := restore(cfg); err != nil {
			fail(err)
		}
		return
	}
	// A second launch while Sortwise is already running just opens it, or,
	// started quietly (at sign-in, or by the extension's sortwise: link),
	// does nothing.
	if alreadyRunning(cfg.Address) {
		if cfg.Open {
			openBrowser("http://" + cfg.Address)
		}
		return
	}
	// Another copy is starting up at this moment: once it answers, this
	// launch is a second one too.
	release, only := claimInstance(cfg.Address)
	if !only {
		if waitUntilRunning(cfg.Address, 30*time.Second) && cfg.Open {
			openBrowser("http://" + cfg.Address)
		}
		return
	}
	defer release()
	if cfg.Tray {
		runInTray(cfg)
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := serve(ctx, cfg, nil, nil); err != nil {
		fail(err)
	}
}

func restore(cfg config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := database.Restore(ctx, cfg.Database, cfg.RestorePath); err != nil {
		return err
	}
	slog.Info("Backup restored", "database", cfg.Database)
	return nil
}

// serve runs Sortwise until ctx ends. ready, when given, is called with the
// running library once the server is accepting connections. desk, when
// given, is this copy's place in Windows: the Start menu shortcut and
// starting at sign-in.
func serve(ctx context.Context, cfg config.Config, ready func(*library), desk *desktop.Integration) error {
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()
	var windows httpapi.Desktop
	if desk != nil {
		settleIn(ctx, store.New(db), desk)
		windows = desk
	}
	credentialManager := credentials.New()
	providers := ai.DefaultFactories()
	worker := ai.NewWorkerWithProviders(db, credentialManager, providers)
	worker.SetPostFetcher(xembed.New())
	go worker.Run(ctx)
	// Authors' profile pictures are saved beside the library.
	avatarDir := filepath.Join(cfg.DataDir, "avatars")
	go avatars.New(store.New(db), avatarDir).Run(ctx)
	// Posts are indexed for search by meaning with the Gemini key.
	meaning := semantic.New(store.New(db), credentialManager, ai.NewGeminiEmbedder)
	go meaning.Run(ctx)
	// Once a day, whether a newer release is out (never in a development build).
	newer := updates.New(buildinfo.Version, store.New(db))
	go newer.Run(ctx)

	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           httpapi.New(db, httpapi.Dependencies{Credentials: credentialManager, Worker: worker, Providers: providers, AvatarDir: avatarDir, Semantic: meaning, Desktop: windows, Updates: newer}),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	listener, err := listen(cfg.Address)
	if err != nil {
		return err
	}
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(listener) }()
	slog.Info("Sortwise is ready", "url", "http://"+cfg.Address)
	if ready != nil {
		ready(&library{db: db, store: store.New(db), keys: credentialManager, providers: providers, worker: worker, updates: newer})
	}
	if cfg.Open {
		openBrowser("http://" + cfg.Address)
	}
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// settleIn adds Sortwise to the Start menu and to the apps that start at
// sign-in the first time it runs, and on later runs points whichever of
// those are still there at this copy, in case it moved.
func settleIn(ctx context.Context, st *store.Store, desk *desktop.Integration) {
	if err := desk.RegisterLink(); err != nil {
		slog.Warn("Could not set up the sortwise: link the extension starts Sortwise with", "error", err)
	}
	first := !st.DesktopSetUp(ctx)
	if err := desk.Refresh(first); err != nil {
		slog.Warn("Could not update Sortwise's place in Windows", "error", err)
		return
	}
	if first {
		if err := st.MarkDesktopSetUp(ctx); err != nil {
			slog.Warn("Could not record that Sortwise was added to Windows", "error", err)
		}
	}
}

// waitUntilRunning waits up to limit for Sortwise to answer on address.
func waitUntilRunning(address string, limit time.Duration) bool {
	for deadline := time.Now().Add(limit); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
		if alreadyRunning(address) {
			return true
		}
	}
	return false
}

// alreadyRunning reports whether a Sortwise server answers on the address.
func alreadyRunning(address string) bool {
	client := http.Client{Timeout: 700 * time.Millisecond}
	response, err := client.Get("http://" + address + "/api/v1/health")
	if err != nil {
		return false
	}
	defer response.Body.Close()
	var health struct {
		Status string `json:"status"`
	}
	return response.StatusCode == http.StatusOK && json.NewDecoder(response.Body).Decode(&health) == nil && health.Status == "ok"
}

func openBrowser(url string) {
	if runtime.GOOS == "windows" {
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
}

// setupLogging writes the log to sortwise.log in the data folder (there is no
// console when Sortwise runs from the tray) and to stderr when there is one.
func setupLogging(cfg config.Config) func() {
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return func() {}
	}
	path := filepath.Join(cfg.DataDir, "sortwise.log")
	// Start a fresh log once it passes 2 MB, keeping one previous copy.
	if info, err := os.Stat(path); err == nil && info.Size() > 2<<20 {
		_ = os.Rename(path, path+".1")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return func() {}
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(file, stderrIfAny()), nil)))
	return func() { file.Close() }
}

// stderrIfAny returns stderr when it is usable, or a discard writer for a
// windowed launch with no console (writing there would fail every call).
func stderrIfAny() io.Writer {
	if _, err := os.Stderr.Stat(); err != nil {
		return io.Discard
	}
	return os.Stderr
}

func fail(err error) {
	slog.Error("Sortwise stopped", "error", err)
	showError("Sortwise could not start", err.Error())
	os.Exit(1)
}
