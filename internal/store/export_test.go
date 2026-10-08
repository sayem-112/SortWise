package store

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sortwise/internal/database"
	"sortwise/internal/model"
)

func TestExportsExcludeCredentialTablesAndBackupRestores(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if _, err := s.Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{sampleBookmark()}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO extension_tokens(token_hash) VALUES ('never-export-this')`); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := s.ExportJSON(ctx, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "never-export-this") || strings.Contains(output.String(), "extension_tokens") {
		t.Fatal("JSON export leaked credential data")
	}
	output.Reset()
	if err := s.ExportCSV(ctx, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Reliable queues") {
		t.Fatal("CSV omitted bookmark")
	}
	_, backupPath, err := s.CreateBackup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "restored.db")
	if err := database.Restore(ctx, target, backupPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal(err)
	}
	restored, err := database.Open(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var count int
	if err := restored.QueryRow(`SELECT COUNT(*) FROM bookmarks`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("restored %d bookmarks: %v", count, err)
	}
}

func TestSearchTenThousandBookmarks(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	statement, err := tx.Prepare(`INSERT INTO bookmarks(post_id,author,username,text,url,raw_payload_json,extractor_version,content_hash,processing_status) VALUES (?,?,?,?,?,'{}','test',?,'completed')`)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 10000; index++ {
		text := "A regular saved post"
		if index%1000 == 0 {
			text = "A unique performance needle about queues"
		}
		id := fmt.Sprintf("%d", 900000+index)
		if _, err = statement.Exec(id, "Test Author", "tester", text, "https://x.com/tester/status/"+id, id); err != nil {
			statement.Close()
			tx.Rollback()
			t.Fatal(err)
		}
	}
	statement.Close()
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	page, err := s.SearchBookmarks(ctx, model.BookmarkQuery{Text: "performance needle", PageSize: 24})
	elapsed := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 10 {
		t.Fatalf("expected 10 matches, got %d", page.Total)
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("10,000-item search took %s", elapsed)
	}
}
