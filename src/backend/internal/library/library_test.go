package library

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aydencharles/ps5-ftp-fnOS/src/backend/internal/domain"
)

func TestResolveRejectsTraversalAndEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	l := NewStatic(nil, []domain.LibraryRoot{{ID: "root", Path: root, Label: "Root"}})
	if _, err := l.Resolve("root", "../secret"); err == nil {
		t.Fatal("traversal accepted")
	}
	if _, err := l.Resolve("root", "escape/secret"); err == nil {
		t.Fatal("escaping symlink accepted")
	}
}

func TestResolveForWriteAllowsNewDescendantsButRejectsEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "downloads")
	if err := os.Mkdir(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	l := NewStatic(nil, []domain.LibraryRoot{{ID: "root", Path: root, Label: "Root"}})
	got, err := l.ResolveForWrite("root", "downloads/game.exfat")
	resolvedRoot, resolveErr := filepath.EvalSymlinks(root)
	if resolveErr != nil {
		t.Fatal(resolveErr)
	}
	if err != nil || got != filepath.Join(resolvedRoot, "downloads", "game.exfat") {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if _, err := l.ResolveForWrite("root", "escape/game.exfat"); err == nil {
		t.Fatal("escaping symlink accepted for write")
	}
}

func TestEntriesRecognizeGames(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "PPSA00001")
	if err := os.Mkdir(game, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(game, "eboot.bin"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, image := range []string{"game.exfat", "game.ffpfs", "game.ffpfsc", "game.phu"} {
		if err := os.WriteFile(filepath.Join(root, image), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	l := NewStatic(nil, []domain.LibraryRoot{{ID: "root", Path: root, Label: "Root"}})
	entries, err := l.Entries("root", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]string{}
	for _, e := range entries {
		kinds[e.Name] = e.GameKind
	}
	if kinds["PPSA00001"] != "game-directory" {
		t.Fatalf("kinds=%v", kinds)
	}
	for _, image := range []string{"game.exfat", "game.ffpfs", "game.ffpfsc", "game.phu"} {
		if kinds[image] != "game-image" {
			t.Fatalf("%s kind=%q, all=%v", image, kinds[image], kinds)
		}
	}
}

func TestEntriesFlattenVolumeUIDDirectories(t *testing.T) {
	root := t.TempDir()
	share := filepath.Join(root, "1000", "PS5 游戏")
	if err := os.MkdirAll(share, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(share, "eboot.bin"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	l := NewStatic(nil, []domain.LibraryRoot{{ID: "volume", Path: root, Label: "存储空间 2", Kind: "volume"}})

	entries, err := l.Entries("volume", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries=%v", entries)
	}
	if entries[0].Name != "PS5 游戏" || entries[0].Path != "1000/PS5 游戏" || entries[0].GameKind != "game-directory" {
		t.Fatalf("flattened entry=%+v", entries[0])
	}
}
