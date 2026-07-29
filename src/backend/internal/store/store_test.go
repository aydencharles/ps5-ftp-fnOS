package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
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

func TestExtractionTaskIsIndependentAndPasswordIsCleared(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	profile, err := s.SaveProfile(ctx, domain.Profile{Name: "PS5", Host: "127.0.0.1", Port: 2121, BasePath: "/"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.CreateExtractionTask(ctx, domain.ExtractionTask{
		Source:            domain.SourceLocator{RootID: "root", Path: "Game.7z.001"},
		DestinationParent: domain.SourceLocator{RootID: "root", Path: "downloads"},
		Destination:       domain.SourceLocator{RootID: "root", Path: "downloads/Game"},
		Password:          "archive-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.Password != "" {
		t.Fatal("public task leaked password")
	}
	var ciphertext string
	if err = s.db.QueryRowContext(ctx, "SELECT password_cipher FROM extraction_tasks WHERE id=?", task.ID).Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if ciphertext == "" || strings.Contains(ciphertext, "archive-secret") {
		t.Fatalf("password was not encrypted: %q", ciphertext)
	}
	private, err := s.ExtractionTask(ctx, task.ID, true)
	if err != nil || private.Password != "archive-secret" {
		t.Fatalf("password=%q err=%v", private.Password, err)
	}
	if err = s.DeleteProfile(ctx, profile.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExtractionTask(ctx, task.ID, false); err != nil {
		t.Fatalf("profile deletion removed extraction: %v", err)
	}
	if err = s.SetExtractionState(ctx, task.ID, domain.ExtractionFailed, "bad archive", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRowContext(ctx, "SELECT password_cipher FROM extraction_tasks WHERE id=?", task.ID).Scan(&ciphertext); err != nil || ciphertext != "" {
		t.Fatalf("terminal password=%q err=%v", ciphertext, err)
	}
}

func TestExtractionClaimDoesNotReviveCanceledTask(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	task, err := s.CreateExtractionTask(ctx, domain.ExtractionTask{
		Source:            domain.SourceLocator{RootID: "root", Path: "Game.7z.001"},
		DestinationParent: domain.SourceLocator{RootID: "root", Path: ""},
		Destination:       domain.SourceLocator{RootID: "root", Path: "Game"},
	})
	if err != nil {
		t.Fatal(err)
	}
	canceled, err := s.CancelQueuedExtraction(ctx, task.ID)
	if err != nil || !canceled {
		t.Fatalf("cancel queued: canceled=%v err=%v", canceled, err)
	}
	claimed, err := s.ClaimExtraction(ctx, task.ID)
	if err != nil || claimed {
		t.Fatalf("claim canceled: claimed=%v err=%v", claimed, err)
	}
	stored, err := s.ExtractionTask(ctx, task.ID, false)
	if err != nil || stored.State != domain.ExtractionCanceled {
		t.Fatalf("state=%q err=%v", stored.State, err)
	}
}

func TestVersionTwoMigrationPreservesTransferData(t *testing.T) {
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "migration.db")
	keyPath := filepath.Join(directory, "key")
	ctx := context.Background()
	s, err := Open(databasePath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := s.SaveProfile(ctx, domain.Profile{Name: "PS5", Host: "127.0.0.1", Port: 2121, BasePath: "/"})
	if err != nil {
		t.Fatal(err)
	}
	transfer, err := s.CreateTask(ctx, domain.Task{ProfileID: profile.ID, Sources: []domain.SourceLocator{{RootID: "root", Path: "game"}}, Destination: "/data", ConflictPolicy: "smart"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.ExecContext(ctx, "DROP TABLE extraction_tasks"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.ExecContext(ctx, "UPDATE schema_version SET version=1"); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = Open(databasePath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	stored, err := s.Task(ctx, transfer.ID)
	if err != nil || stored.ProfileID != profile.ID || stored.Destination != "/data" {
		t.Fatalf("transfer changed during migration: %+v err=%v", stored, err)
	}
	var version int
	if err = s.db.QueryRowContext(ctx, "SELECT version FROM schema_version").Scan(&version); err != nil || version != 2 {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
	if count, countErr := s.ExtractionTaskCount(ctx); countErr != nil || count != 0 {
		t.Fatalf("extraction table count=%d err=%v", count, countErr)
	}
}
