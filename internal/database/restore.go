package database

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"time"
)

func Restore(ctx context.Context, target, backup string) error {
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	backupAbs, err := filepath.Abs(backup)
	if err != nil {
		return err
	}
	if targetAbs == backupAbs {
		return fmt.Errorf("backup and target database must differ")
	}
	if err = validateBackup(ctx, backupAbs); err != nil {
		return err
	}
	if _, err = os.Stat(targetAbs); err == nil {
		preserved := targetAbs + ".pre-restore-" + time.Now().UTC().Format("20060102-150405") + ".bak"
		if err = copyFile(targetAbs, preserved); err != nil {
			return fmt.Errorf("preserve current database: %w", err)
		}
	}
	temporary := targetAbs + ".restore.tmp"
	if err = copyFile(backupAbs, temporary); err != nil {
		return err
	}
	if err = os.Rename(temporary, targetAbs); err != nil {
		if copyErr := copyFile(temporary, targetAbs); copyErr != nil {
			return copyErr
		}
		_ = os.Remove(temporary)
	}
	return nil
}
func validateBackup(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", path+"?mode=ro")
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	defer db.Close()
	var integrity string
	if err = db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		return fmt.Errorf("backup integrity check failed: %s %v", integrity, err)
	}
	var migrations int
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&migrations); err != nil || migrations < 1 {
		return fmt.Errorf("file is not a Sortwise backup")
	}
	return nil
}
func copyFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err = os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	syncErr := output.Sync()
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
