package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aydencharles/ps5-ftp-fnOS/src/backend/internal/domain"
	"github.com/aydencharles/ps5-ftp-fnOS/src/backend/internal/extractqueue"
	"github.com/aydencharles/ps5-ftp-fnOS/src/backend/internal/library"
	"github.com/aydencharles/ps5-ftp-fnOS/src/backend/internal/queue"
	"github.com/aydencharles/ps5-ftp-fnOS/src/backend/internal/store"
)

func extractionHandler(t *testing.T) (http.Handler, *store.Store, string) {
	t.Helper()
	directory := t.TempDir()
	storage := filepath.Join(directory, "storage")
	if err := os.Mkdir(storage, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storage, "Game.7z.001"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(filepath.Join(directory, "app.db"), filepath.Join(directory, "key"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	libraryStore := library.NewStatic(s, []domain.LibraryRoot{{ID: "root", Path: storage, Label: "Storage"}})
	extractions := extractqueue.New(s, libraryStore)
	return New(s, libraryStore, queue.New(s, libraryStore), extractions, directory).Handler(), s, storage
}

func TestCreateExtractionTaskUsesIndependentAPIAndHidesPassword(t *testing.T) {
	handler, s, _ := extractionHandler(t)
	body := `{"source":{"root_id":"root","path":"Game.7z.001"},"destination_parent":{"root_id":"root","path":""},"password":"top-secret","delete_sources":true}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/extraction-tasks", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "top-secret") || strings.Contains(recorder.Body.String(), "password") {
		t.Fatalf("response leaked secret: %s", recorder.Body.String())
	}
	var response struct {
		Task domain.ExtractionTask `json:"task"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Task.Destination.Path != "Game" || !response.Task.DeleteSources || response.Task.State != domain.ExtractionQueued {
		t.Fatalf("task=%+v", response.Task)
	}
	if count, err := s.ExtractionTaskCount(context.Background()); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if tasks, err := s.Tasks(context.Background()); err != nil || len(tasks) != 0 {
		t.Fatalf("transfer tasks=%d err=%v", len(tasks), err)
	}
}

func TestCreateExtractionRejectsNonFirstVolume(t *testing.T) {
	handler, _, storage := extractionHandler(t)
	if err := os.WriteFile(filepath.Join(storage, "Game.7z.002"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	body := `{"source":{"root_id":"root","path":"Game.7z.002"},"destination_parent":{"root_id":"root","path":""}}`
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/extraction-tasks", bytes.NewBufferString(body)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestBootstrapReturnsSeparateExtractionCollection(t *testing.T) {
	handler, _, _ := extractionHandler(t)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/bootstrap", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"extraction_tasks":[]`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
