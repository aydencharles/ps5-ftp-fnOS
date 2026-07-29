package archive

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var sampleVolumes = []string{
	"N3q8ryccAAQAT6hMUwEAAAAAAAByAAAAAAAAAKQad8jgAioBS10AMJwKRgNofKHEGlcPS1zStaxjmRxQSqhBHxCVYgMtWcqyJolLEVijgbq7YouBmnoGyh9mJmXJDs55FZc9JtCAollYzOTzhLylFj3wSmbS+gmTXUC8t1TpQgvNB9Bs+Q72jZYVm6+VxqadjPrQ+ezzlBJ4YMIj00nTMhP7Z6YyAM+4iEXNy1EUyM8/nxLbe+HHMiK+5o1KDvZhKSMCWvDkek/+oHEsvWkPziRw6Y6arMt3e6y94w==",
	"n8O0TxuYVKd26KL91jGCcO6Xm4ZKHkp6NlHKE/s1+P1xvnG6tzC7tnp8zbJRIBiq40EFYoI9ht6KytMp1U4BGo3QTkDM1mOjVQLyH9vN8wdf1RH1yFjLNEGtJ4URdVZAEQ+VQpeLX5lgr+E0P+CsqKywFbO2EaI38HfbcGfePx2YG3xbiRBKsa+/Tg/VpHS1+i+xZ6glAAEEBgABCYFTAAcLAQABISEBAAyCKwAICgETT6a9AAAFARkKAAAAAAAAAAAAABEnAHAAYQBjAGsAYQBnAGkAbgBnAC8AbQ==",
	"AGEAbgBpAGYAZQBzAHQAAAAZBAAAAAAUCgEA006KOyUf3QEVBgEAIICkgQAA",
}

func writeSampleArchive(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	first := filepath.Join(directory, "sample.7z.001")
	for index, encoded := range sampleVolumes {
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Join(directory, "sample.7z."+[]string{"001", "002", "003"}[index])
		if err = os.WriteFile(name, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return first
}

func TestOpenAndExtractSplitArchive(t *testing.T) {
	first := writeSampleArchive(t)
	plan, err := Open(first, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Volumes) != 3 || plan.TotalBytes == 0 {
		t.Fatalf("volumes=%d bytes=%d", len(plan.Volumes), plan.TotalBytes)
	}
	stage := filepath.Join(filepath.Dir(first), StageName("test"))
	var last Progress
	if err = Extract(context.Background(), plan, stage, func(progress Progress) { last = progress }); err != nil {
		t.Fatal(err)
	}
	if err = plan.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(stage, "packaging", "manifest"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "appname") || last.Bytes != plan.TotalBytes || last.Completed != len(plan.Entries) {
		t.Fatalf("content/progress mismatch: %+v", last)
	}
	destination := filepath.Join(filepath.Dir(first), "sample")
	if err = Publish(stage, destination); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(destination); err != nil {
		t.Fatal(err)
	}
}

func TestMissingVolumeFailsWithoutOutput(t *testing.T) {
	first := writeSampleArchive(t)
	if err := os.Remove(strings.TrimSuffix(first, "001") + "003"); err != nil {
		t.Fatal(err)
	}
	if plan, err := Open(first, ""); err == nil {
		_ = plan.Close()
		t.Fatal("incomplete split archive opened successfully")
	}
}

func TestRejectsUnsafeEntryNames(t *testing.T) {
	for _, name := range []string{"../outside", "/absolute", "C:/windows", "safe/../outside", "//server/share"} {
		if _, err := normalizeEntryName(name); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	if got, err := normalizeEntryName("folder/file.bin"); err != nil || got != "folder/file.bin" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestPublishDoesNotReplaceDestination(t *testing.T) {
	directory := t.TempDir()
	stage := filepath.Join(directory, StageName("task"))
	destination := filepath.Join(directory, "output")
	if err := os.Mkdir(stage, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Publish(stage, destination); !errors.Is(err, os.ErrExist) {
		t.Fatalf("err=%v", err)
	}
}

func TestDestinationNames(t *testing.T) {
	for input, expected := range map[string]string{"Game.7z.001": "Game", "archive.7Z": "archive"} {
		if got, err := DestinationName(input); err != nil || got != expected {
			t.Fatalf("%s => %q, %v", input, got, err)
		}
	}
	if _, err := DestinationName("archive.7z.002"); err == nil {
		t.Fatal("accepted a non-first volume")
	}
}
