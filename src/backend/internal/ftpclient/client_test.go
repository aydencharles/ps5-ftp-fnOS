package ftpclient

import "testing"

func TestJoinNormalizesAndRejectsTraversal(t *testing.T) {
	got, err := Join("/data", "homebrew", "game.exfat")
	if err != nil || got != "/data/homebrew/game.exfat" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if _, err = Join("/data", "../system"); err == nil {
		t.Fatal("traversal accepted")
	}
}

func TestMakeDirAllFromRejectsOutsideBase(t *testing.T) {
	c := &Client{}
	if err := c.MakeDirAllFrom("/data", "/system"); err == nil {
		t.Fatal("outside path accepted")
	}
}
