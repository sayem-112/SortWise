package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sortwise/internal/database"
	"sortwise/internal/model"
	"sortwise/internal/store"
	"testing"
)

func testWorkerDatabase(ctx context.Context, t *testing.T, name, postID string) (*sql.DB, error) {
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), name))
	if err != nil {
		return nil, err
	}
	item := model.ImportedBookmark{Platform: "x", PostID: postID, Author: "Ada", Username: "ada", Text: "Queue guide", URL: "https://x.com/ada/status/" + postID, ExtractedAt: "2026-07-18T12:00:00Z", ExtractorVersion: "1.0.0"}
	if _, err = store.New(db).Import(ctx, model.ImportBatch{Bookmarks: []model.ImportedBookmark{item}}); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func TestGroqReturnsValidatedStructuredAnalysis(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key-value" {
			t.Error("missing bearer key")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != GroqModel || body["reasoning_format"] != "hidden" {
			t.Errorf("unexpected model or reasoning settings: %v %v", body["model"], body["reasoning_format"])
		}
		format, _ := body["response_format"].(map[string]any)
		if format["type"] != "json_schema" {
			t.Errorf("expected strict json_schema response format, got %v", format["type"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"summary\":\"A practical queue guide.\",\"mediaDescription\":\"\",\"categories\":[{\"name\":\"Backend\",\"confidence\":0.9}],\"tags\":[{\"name\":\"queue\",\"confidence\":0.9},{\"name\":\"retry\",\"confidence\":0.8},{\"name\":\"backend\",\"confidence\":0.7}]}"}}]}`))
	}))
	defer server.Close()
	previous := groqChatCompletionsURL
	groqChatCompletionsURL = server.URL
	defer func() { groqChatCompletionsURL = previous }()
	provider, err := NewGroq(context.Background(), "test-key-value")
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Analyze(context.Background(), AnalysisInput{Text: "Queue guide", Categories: []CategoryOption{{ID: 1, Name: "Backend"}}})
	if err != nil {
		t.Fatal(err)
	}
	// "backend" repeats the Backend category name, so it is dropped as too broad.
	if result.Summary == "" || len(result.Tags) != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestWorkerPauseDoesNotClaimJob(t *testing.T) {
	ctx := context.Background()
	db, err := testWorkerDatabase(ctx, t, "paused.db", "4001")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO settings(key,value) VALUES ('ai_paused','true')`); err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(db, fakeCredentials{key: "fake"}, func(context.Context, string) (Provider, error) { return fakeProvider{}, nil })
	processed, err := worker.ProcessOne(ctx)
	if err != nil || processed {
		t.Fatalf("paused worker processed=%v err=%v", processed, err)
	}
	var status string
	var attempts int
	if err := db.QueryRow(`SELECT status,attempts FROM jobs`).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || attempts != 0 {
		t.Fatalf("paused job changed: %s attempts=%d", status, attempts)
	}
}

type retiredModelProvider struct{}

func (retiredModelProvider) Analyze(context.Context, AnalysisInput) (AnalysisResult, error) {
	return AnalysisResult{}, errors.New(`Groq returned 404 Not Found: {"error":{"code":"model_not_found"}}`)
}

func TestWorkerBlocksQueueOnConfigurationError(t *testing.T) {
	ctx := context.Background()
	db, err := testWorkerDatabase(ctx, t, "retired.db", "5001")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	worker := NewWorker(db, fakeCredentials{key: "fake"}, func(context.Context, string) (Provider, error) { return retiredModelProvider{}, nil })
	worker.minRequestInterval = 0
	if processed, _ := worker.ProcessOne(ctx); !processed {
		t.Fatal("expected the job to be attempted")
	}
	var status string
	var attempts int
	if err := db.QueryRow(`SELECT status,attempts FROM jobs`).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "blocked" || attempts != 0 {
		t.Fatalf("configuration error should block without spending attempts: %s attempts=%d", status, attempts)
	}
	if processed, _ := worker.ProcessOne(ctx); processed {
		t.Fatal("worker should idle during the configuration cooldown")
	}
	worker.Unblock(ctx)
	if err := db.QueryRow(`SELECT status FROM jobs`).Scan(&status); err != nil || status != "pending" {
		t.Fatalf("unblock should release the job: %s %v", status, err)
	}
}

type overloadedProvider struct{}

func (overloadedProvider) Analyze(context.Context, AnalysisInput) (AnalysisResult, error) {
	return AnalysisResult{}, errors.New("Error 503, Message: This model is currently experiencing high demand. Please try again later., Status: UNAVAILABLE")
}

func TestWorkerRequeuesProviderOutageWithoutSpendingAttempts(t *testing.T) {
	ctx := context.Background()
	db, err := testWorkerDatabase(ctx, t, "overloaded.db", "6001")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	worker := NewWorker(db, fakeCredentials{key: "fake"}, func(context.Context, string) (Provider, error) { return overloadedProvider{}, nil })
	worker.minRequestInterval = 0
	if processed, _ := worker.ProcessOne(ctx); !processed {
		t.Fatal("expected the job to be attempted")
	}
	var status string
	var attempts int
	if err := db.QueryRow(`SELECT status,attempts FROM jobs`).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || attempts != 0 {
		t.Fatalf("an outage must requeue without spending attempts: %s attempts=%d", status, attempts)
	}
	if processed, _ := worker.ProcessOne(ctx); processed {
		t.Fatal("worker should back off after an outage")
	}
}
