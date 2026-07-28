package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/domain"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"), filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestProfilePasswordEncryptedAndHidden(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	p, err := s.SaveProfile(ctx, domain.Profile{Name: "PS5", Host: "127.0.0.1", Port: 2121, Username: "anonymous", Password: "secret", BasePath: "/", Preset: "ftpsrv"})
	if err != nil {
		t.Fatal(err)
	}
	public, err := s.Profile(ctx, p.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if public.Password != "" {
		t.Fatal("public profile leaked password")
	}
	private, err := s.Profile(ctx, p.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if private.Password != "secret" {
		t.Fatalf("password=%q", private.Password)
	}
}

func TestInterruptInFlightKeepsQueued(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	p, _ := s.SaveProfile(ctx, domain.Profile{Name: "PS5", Host: "127.0.0.1", Port: 2121, BasePath: "/"})
	running, _ := s.CreateTask(ctx, domain.Task{ProfileID: p.ID, Sources: []domain.SourceLocator{{RootID: "r", Path: "a"}}, Destination: "/", ConflictPolicy: "smart"})
	_ = s.SetTaskState(ctx, running.ID, domain.TaskRunning, "")
	queued, _ := s.CreateTask(ctx, domain.Task{ProfileID: p.ID, Sources: []domain.SourceLocator{{RootID: "r", Path: "b"}}, Destination: "/", ConflictPolicy: "smart"})
	if err := s.InterruptInFlight(ctx); err != nil {
		t.Fatal(err)
	}
	r, _ := s.Task(ctx, running.ID)
	q, _ := s.Task(ctx, queued.ID)
	if r.State != domain.TaskInterrupted {
		t.Fatalf("running state=%s", r.State)
	}
	if q.State != domain.TaskQueued {
		t.Fatalf("queued state=%s", q.State)
	}
}

func TestNormalizeRemotePath(t *testing.T) {
	for _, bad := range []string{"/data/../system", "../data", "/a/../../b"} {
		if _, err := NormalizeRemotePath(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	got, err := NormalizeRemotePath("//data//homebrew/")
	if err != nil || got != "/data/homebrew" {
		t.Fatalf("got %q err=%v", got, err)
	}
}

func TestDeleteTaskOnlyRemovesHistoryAndCascadesDetails(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	p, _ := s.SaveProfile(ctx, domain.Profile{Name: "PS5", Host: "127.0.0.1", Port: 2121, BasePath: "/"})
	task, _ := s.CreateTask(ctx, domain.Task{ProfileID: p.ID, Sources: []domain.SourceLocator{{RootID: "r", Path: "game"}}, Destination: "/data", ConflictPolicy: "smart"})

	if err := s.DeleteTask(ctx, task.ID); err == nil {
		t.Fatal("queued task was deleted as history")
	}
	if _, err := s.Task(ctx, task.ID); err != nil {
		t.Fatalf("queued task disappeared: %v", err)
	}

	if err := s.ReplaceItems(ctx, task.ID, []domain.TaskItem{{RootID: "r", SourcePath: "game/eboot.bin", Destination: "/data/eboot.bin", Size: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Event(ctx, task.ID, "info", "done"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTaskState(ctx, task.ID, domain.TaskSucceeded, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Task(ctx, task.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("task still exists or returned wrong error: %v", err)
	}
	items, _ := s.Items(ctx, task.ID)
	events, _ := s.Events(ctx, task.ID)
	if len(items) != 0 || len(events) != 0 {
		t.Fatalf("details were not cascaded: items=%d events=%d", len(items), len(events))
	}
}
