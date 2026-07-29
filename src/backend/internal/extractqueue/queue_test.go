package extractqueue

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	archiveengine "github.com/chenpy/ps5-ftp-fnos/src/backend/internal/archive"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/domain"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/library"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/store"
)

var queueSampleVolumes = []string{
	"N3q8ryccAAQAT6hMUwEAAAAAAAByAAAAAAAAAKQad8jgAioBS10AMJwKRgNofKHEGlcPS1zStaxjmRxQSqhBHxCVYgMtWcqyJolLEVijgbq7YouBmnoGyh9mJmXJDs55FZc9JtCAollYzOTzhLylFj3wSmbS+gmTXUC8t1TpQgvNB9Bs+Q72jZYVm6+VxqadjPrQ+ezzlBJ4YMIj00nTMhP7Z6YyAM+4iEXNy1EUyM8/nxLbe+HHMiK+5o1KDvZhKSMCWvDkek/+oHEsvWkPziRw6Y6arMt3e6y94w==",
	"n8O0TxuYVKd26KL91jGCcO6Xm4ZKHkp6NlHKE/s1+P1xvnG6tzC7tnp8zbJRIBiq40EFYoI9ht6KytMp1U4BGo3QTkDM1mOjVQLyH9vN8wdf1RH1yFjLNEGtJ4URdVZAEQ+VQpeLX5lgr+E0P+CsqKywFbO2EaI38HfbcGfePx2YG3xbiRBKsa+/Tg/VpHS1+i+xZ6glAAEEBgABCYFTAAcLAQABISEBAAyCKwAICgETT6a9AAAFARkKAAAAAAAAAAAAABEnAHAAYQBjAGsAYQBnAGkAbgBnAC8AbQ==",
	"AGEAbgBpAGYAZQBzAHQAAAAZBAAAAAAUCgEA006KOyUf3QEVBgEAIICkgQAA",
}

func writeQueueArchive(t *testing.T, root, name string) []string {
	t.Helper()
	paths := make([]string, 0, len(queueSampleVolumes))
	for index, encoded := range queueSampleVolumes {
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, name+".7z."+[]string{"001", "002", "003"}[index])
		if err = os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	return paths
}

func waitForExtraction(t *testing.T, s *store.Store, id string) domain.ExtractionTask {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		task, err := s.ExtractionTask(context.Background(), id, false)
		if err != nil {
			t.Fatal(err)
		}
		if domain.ExtractionTerminal(task.State) {
			return task
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("extraction did not reach a terminal state")
	return domain.ExtractionTask{}
}

func TestManagerExtractsSplitArchiveAndAppliesSourcePolicy(t *testing.T) {
	for _, deleteSources := range []bool{false, true} {
		deleteSources := deleteSources
		t.Run(map[bool]string{false: "preserve", true: "delete"}[deleteSources], func(t *testing.T) {
			root := t.TempDir()
			name := map[bool]string{false: "keep", true: "remove"}[deleteSources]
			volumes := writeQueueArchive(t, root, name)
			s, err := store.Open(filepath.Join(root, "tasks.db"), filepath.Join(root, "key"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			lib := library.NewStatic(s, []domain.LibraryRoot{{ID: "root", Label: "Root", Path: root, Kind: "volume"}})
			task, err := s.CreateExtractionTask(context.Background(), domain.ExtractionTask{
				Source:            domain.SourceLocator{RootID: "root", Path: name + ".7z.001"},
				DestinationParent: domain.SourceLocator{RootID: "root", Path: ""},
				Destination:       domain.SourceLocator{RootID: "root", Path: name},
				DeleteSources:     deleteSources,
			})
			if err != nil {
				t.Fatal(err)
			}

			manager := New(s, lib)
			manager.dispatch()
			finished := waitForExtraction(t, s, task.ID)
			manager.Close()

			if finished.State != domain.ExtractionSucceeded || finished.TotalBytes == 0 || finished.ExtractedBytes != finished.TotalBytes {
				t.Fatalf("unexpected result: %+v", finished)
			}
			if _, err = os.Stat(filepath.Join(root, name, "packaging", "manifest")); err != nil {
				t.Fatalf("published output missing: %v", err)
			}
			if _, err = os.Stat(filepath.Join(root, archiveengine.StageName(task.ID))); !os.IsNotExist(err) {
				t.Fatalf("staging directory was not removed: %v", err)
			}
			for _, volume := range volumes {
				_, statErr := os.Stat(volume)
				if deleteSources && !os.IsNotExist(statErr) {
					t.Fatalf("source volume still exists: %s", volume)
				}
				if !deleteSources && statErr != nil {
					t.Fatalf("source volume was removed: %s: %v", volume, statErr)
				}
			}
		})
	}
}
