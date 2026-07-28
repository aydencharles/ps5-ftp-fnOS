package queue

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/domain"
)

func TestDedupeNestedSources(t *testing.T) {
	in := []domain.SourceLocator{{RootID: "r", Path: "games/a/file"}, {RootID: "r", Path: "games/a"}, {RootID: "r", Path: "games/b"}}
	out := dedupe(in)
	if len(out) != 2 || out[0].Path != "games/a" || out[1].Path != "games/b" {
		t.Fatalf("out=%v", out)
	}
}

func TestVerifyRemoteSizeRetriesTransientSizeError(t *testing.T) {
	calls := 0
	size, err := verifyRemoteSize(context.Background(), 1<<33, func() (int64, error) {
		calls++
		if calls == 1 {
			return 0, errors.New("control connection closed")
		}
		return 1 << 33, nil
	}, 0)
	if err != nil || size != 1<<33 || calls != 2 {
		t.Fatalf("size=%d calls=%d err=%v", size, calls, err)
	}
}

func TestVerifyRemoteSizeReportsExpectedAndActualSizes(t *testing.T) {
	_, err := verifyRemoteSize(context.Background(), 73732718592, func() (int64, error) {
		return 73700000000, nil
	}, 0)
	if err == nil || !strings.Contains(err.Error(), "本地 73732718592") || !strings.Contains(err.Error(), "远端 73700000000") {
		t.Fatalf("err=%v", err)
	}
}

func TestVerifyRemoteSizeReportsFTPError(t *testing.T) {
	_, err := verifyRemoteSize(context.Background(), 1024, func() (int64, error) {
		return 0, errors.New("421 idle timeout")
	}, time.Nanosecond)
	if err == nil || !strings.Contains(err.Error(), "FTP SIZE 命令失败") || !strings.Contains(err.Error(), "421 idle timeout") {
		t.Fatalf("err=%v", err)
	}
}
