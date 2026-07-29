package web

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/domain"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/extractqueue"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/library"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/queue"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/store"
)

func TestDeleteTaskEndpointOnlyDeletesHistory(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "app.db"), filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	l := library.NewStatic(s, nil)
	handler := New(s, l, queue.New(s, l), extractqueue.New(s, l), dir).Handler()
	ctx := context.Background()
	profile, _ := s.SaveProfile(ctx, domain.Profile{Name: "PS5", Host: "127.0.0.1", Port: 2121, BasePath: "/"})
	task, _ := s.CreateTask(ctx, domain.Task{ProfileID: profile.ID, Sources: []domain.SourceLocator{{RootID: "root", Path: "game"}}, Destination: "/data"})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/"+task.ID, nil))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("queued delete status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	if err := s.SetTaskState(ctx, task.ID, domain.TaskCanceled, ""); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/"+task.ID, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("history delete status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if _, err := s.Task(ctx, task.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("task record still exists: %v", err)
	}
}

func TestCreateDownloadTaskUsesLibraryLocatorAsDestination(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "app.db"), filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	l := library.NewStatic(s, []domain.LibraryRoot{{ID: "root", Path: dir, Label: "Root"}})
	handler := New(s, l, queue.New(s, l), extractqueue.New(s, l), dir).Handler()
	profile, err := s.SaveProfile(context.Background(), domain.Profile{Name: "PS5", Host: "127.0.0.1", Port: 2121, BasePath: "/"})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(domain.Task{
		Type:           "download",
		ProfileID:      profile.ID,
		Sources:        []domain.SourceLocator{{RootID: "root", Path: "/data/game.exfat"}},
		Destination:    "downloads/new-library",
		ConflictPolicy: "smart",
	})
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Task domain.Task `json:"task"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Task.Type != "download" || response.Task.Destination != "downloads/new-library" || len(response.Task.Sources) != 1 || response.Task.Sources[0].RootID != "root" || response.Task.Sources[0].Path != "/data/game.exfat" {
		t.Fatalf("task=%+v", response.Task)
	}
}
