package config

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
)

type Config struct {
	Address string
	DataDir string
	Open    bool
	// Tray runs Sortwise with an icon by the clock instead of in a console.
	Tray        bool
	Database    string
	Command     string
	RestorePath string
}

func Parse(args []string) (Config, error) {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		local = "."
	}
	defaults := filepath.Join(local, "Sortwise")
	command := "serve"
	if len(args) > 0 && args[0] == "restore" {
		command = "restore"
		args = args[1:]
	}
	set := flag.NewFlagSet("sortwise", flag.ContinueOnError)
	port := set.String("port", "8787", "loopback port")
	dataDir := set.String("data-dir", defaults, "application data directory")
	noOpen := set.Bool("no-open", false, "do not open the browser")
	noTray := set.Bool("no-tray", false, "run in the console instead of the system tray")
	restore := set.String("backup", "", "backup database to restore")
	if err := set.Parse(args); err != nil {
		return Config{}, err
	}
	if *port == "" {
		return Config{}, errors.New("port cannot be empty")
	}
	restorePath := *restore
	if command == "restore" && restorePath == "" && set.NArg() > 0 {
		restorePath = set.Arg(0)
	}
	if command == "restore" && restorePath == "" {
		return Config{}, errors.New("restore requires a backup path")
	}
	return Config{
		Address:     "127.0.0.1:" + *port,
		DataDir:     *dataDir,
		Open:        !*noOpen,
		Tray:        !*noTray,
		Database:    filepath.Join(*dataDir, "sortwise.db"),
		Command:     command,
		RestorePath: restorePath,
	}, nil
}
