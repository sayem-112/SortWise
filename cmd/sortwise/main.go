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
	"sortwise/internal/config"
	"sortwise/internal/credentials"
	"sortwise/internal/database"
	"sortwise/internal/httpapi"
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
	// A second launch while Sortwise is already running just opens it.
	if alreadyRunning(cfg.Address) {
		openBrowser("http://" + cfg.Address)
		return
	}
	if cfg.Tray {
		runInTray(cfg)
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := serve(ctx, cfg, nil); err != nil {
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

// serve runs Sortwise until ctx ends. ready, when given, is called once the
// server is accepting connections.
func serve(ctx context.Context, cfg config.Config, ready func()) error {
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()
	credentialManager := credentials.New()
	providers := ai.DefaultFactories()
	worker := ai.NewWorkerWithProviders(db, credentialManager, providers)
	worker.SetPostFetcher(xembed.New())
	go worker.Run(ctx)

	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           httpapi.New(db, httpapi.Dependencies{Credentials: credentialManager, Worker: worker, Providers: providers}),
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
		ready()
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
