package store

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var exportTables = []string{"bookmarks", "media", "imports", "enrichments", "categories", "tags", "tag_aliases", "bookmark_categories", "bookmark_tags", "lists", "list_items"}

type ExportDocument struct {
	Version    string                      `json:"version"`
	ExportedAt string                      `json:"exportedAt"`
	Tables     map[string][]map[string]any `json:"tables"`
}

func (s *Store) ExportJSON(ctx context.Context, writer io.Writer) error {
	document := ExportDocument{Version: "1", ExportedAt: time.Now().UTC().Format(time.RFC3339), Tables: map[string][]map[string]any{}}
	for _, table := range exportTables {
		rows, err := s.DB.QueryContext(ctx, `SELECT * FROM `+table)
		if err != nil {
			return err
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			return err
		}
		values := []map[string]any{}
		for rows.Next() {
			raw := make([]any, len(columns))
			dest := make([]any, len(columns))
			for index := range raw {
				dest[index] = &raw[index]
			}
			if err := rows.Scan(dest...); err != nil {
				rows.Close()
				return err
			}
			record := map[string]any{}
			for index, column := range columns {
				if bytes, ok := raw[index].([]byte); ok {
					record[column] = string(bytes)
				} else {
					record[column] = raw[index]
				}
			}
			values = append(values, record)
		}
		rows.Close()
		document.Tables[table] = values
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(document)
}

func (s *Store) ExportCSV(ctx context.Context, writer io.Writer) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT b.post_id,b.author,b.username,b.text,b.url,b.posted_at,b.bookmarked_at,b.imported_at,b.language,COALESCE(e.manual_summary,e.ai_summary,''),COALESCE(e.media_description,''),b.processing_status,b.archived_at,COALESCE((SELECT group_concat(c.name,' | ') FROM bookmark_categories bc JOIN categories c ON c.id=bc.category_id WHERE bc.bookmark_id=b.id AND bc.manual_state <> 'removed' AND (bc.manual_state='added' OR bc.ai_confidence IS NOT NULL)),''),COALESCE((SELECT group_concat(t.name,' | ') FROM bookmark_tags bt JOIN tags t ON t.id=bt.tag_id WHERE bt.bookmark_id=b.id AND bt.manual_state <> 'removed' AND (bt.manual_state='added' OR bt.ai_confidence IS NOT NULL)),'') FROM bookmarks b LEFT JOIN enrichments e ON e.bookmark_id=b.id ORDER BY b.imported_at,b.id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	csvWriter := csv.NewWriter(writer)
	defer csvWriter.Flush()
	header := []string{"post_id", "author", "username", "text", "url", "posted_at", "bookmarked_at", "imported_at", "language", "summary", "media_description", "processing_status", "archived_at", "categories", "tags"}
	if err := csvWriter.Write(header); err != nil {
		return err
	}
	for rows.Next() {
		values := make([]any, len(header))
		dest := make([]any, len(header))
		for index := range values {
			dest[index] = &values[index]
		}
		if err := rows.Scan(dest...); err != nil {
			return err
		}
		record := make([]string, len(values))
		for index, value := range values {
			if value != nil {
				record[index] = fmt.Sprint(value)
			}
		}
		if err := csvWriter.Write(record); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Store) DatabasePath(ctx context.Context) (string, error) {
	rows, err := s.DB.QueryContext(ctx, `PRAGMA database_list`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var seq int
		var name, path string
		if err := rows.Scan(&seq, &name, &path); err != nil {
			return "", err
		}
		if name == "main" {
			return path, nil
		}
	}
	return "", fmt.Errorf("main database path not found")
}
func (s *Store) CreateBackup(ctx context.Context) (string, string, error) {
	databasePath, err := s.DatabasePath(ctx)
	if err != nil {
		return "", "", err
	}
	dir := filepath.Join(filepath.Dir(databasePath), "backups")
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	name := "sortwise-" + time.Now().UTC().Format("20060102-150405") + ".db"
	path := filepath.Join(dir, name)
	quoted := strings.ReplaceAll(path, "'", "''")
	if _, err = s.DB.ExecContext(ctx, `VACUUM INTO '`+quoted+`'`); err != nil {
		return "", "", err
	}
	return name, path, nil
}
func (s *Store) BackupPath(ctx context.Context, name string) (string, error) {
	if filepath.Base(name) != name || !strings.HasPrefix(name, "sortwise-") || !strings.HasSuffix(name, ".db") {
		return "", fmt.Errorf("invalid backup name")
	}
	databasePath, err := s.DatabasePath(ctx)
	if err != nil {
		return "", err
	}
	path := filepath.Join(filepath.Dir(databasePath), "backups", name)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", os.ErrNotExist
	}
	return path, nil
}
