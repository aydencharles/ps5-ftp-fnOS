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
	"strings"
	"testing"

	"github.com/aydencharles/ps5-ftp-fnOS/src/backend/internal/domain"
	"github.com/aydencharles/ps5-ftp-fnOS/src/backend/internal/extractqueue"
	"github.com/aydencharles/ps5-ftp-fnOS/src/backend/internal/library"
	"github.com/aydencharles/ps5-ftp-fnOS/src/backend/internal/queue"
	"github.com/aydencharles/ps5-ftp-fnOS/src/backend/internal/store"
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
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":1003`) {
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

func TestMissingTaskReturnsResourceNotFound(t *testing.T) {
	handler, _, _ := extractionHandler(t)
	id := "00000000000000000000000000000000"
	requests := []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+id, nil),
		httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/"+id, nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+id+"/cancel", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+id+"/retry", nil),
	}

	for _, request := range requests {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":1002`) {
			t.Fatalf("%s %s status=%d body=%s", request.Method, request.URL.Path, recorder.Code, recorder.Body.String())
		}
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
	body, _ := json.Marshal(map[string]any{
		"type": "download", "profile_id": profile.ID,
		"sources":     []domain.SourceLocator{{RootID: "root", Path: "/data/game.exfat"}},
		"destination": "downloads/new-library", "conflict_policy": "smart",
	})
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Code int `json:"code"`
		Data struct {
			Task domain.Task `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	task := response.Data.Task
	if response.Code != codeSuccess || task.Type != "download" || task.Destination != "downloads/new-library" || len(task.Sources) != 1 || task.Sources[0].RootID != "root" || task.Sources[0].Path != "/data/game.exfat" {
		t.Fatalf("task=%+v", task)
	}
}
