package ftpclient

import (
	"context"
	"errors"
	"net"
	"net/textproto"
	"syscall"
	"testing"

	ftp "github.com/jlaffaye/ftp"

	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/domain"
)

func TestDiagnoseProbeErrorClassifiesRecoverableFailures(t *testing.T) {
	profile := domain.Profile{Host: "192.168.1.50", Port: 2120, BasePath: "/data"}
	tests := []struct {
		name  string
		stage string
		err   error
		code  string
	}{
		{name: "dns", stage: "resolve", err: &net.DNSError{Name: "ps5.local", IsNotFound: true}, code: "dns_failed"},
		{name: "refused", stage: "connect", err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, code: "connection_refused"},
		{name: "timeout", stage: "connect", err: context.DeadlineExceeded, code: "timeout"},
		{name: "login", stage: "authenticate", err: &textproto.Error{Code: ftp.StatusNotLoggedIn, Msg: "not logged in"}, code: "authentication_failed"},
		{name: "path", stage: "directory", err: &textproto.Error{Code: ftp.StatusFileUnavailable, Msg: "not found"}, code: "base_path_unavailable"},
		{name: "passive", stage: "directory", err: &textproto.Error{Code: ftp.StatusCanNotOpenDataConnection, Msg: "cannot open data"}, code: "passive_mode_failed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			failure := diagnoseProbeError(test.stage, profile, test.err)
			if failure.Code != test.code {
				t.Fatalf("code=%q want=%q failure=%+v", failure.Code, test.code, failure)
			}
			if failure.Title == "" || failure.Message == "" || len(failure.Suggestions) == 0 || failure.Detail == "" {
				t.Fatalf("failure is missing actionable detail: %+v", failure)
			}
		})
	}
}

func TestDiagnoseProbeErrorKeepsUnknownStageSpecific(t *testing.T) {
	profile := domain.Profile{Host: "ps5.local", Port: 2121, BasePath: "/mnt/usb"}

	auth := diagnoseProbeError("authenticate", profile, errors.New("custom USER failure"))
	if auth.Code != "authentication_failed" {
		t.Fatalf("auth=%+v", auth)
	}
	directory := diagnoseProbeError("directory", profile, errors.New("custom LIST failure"))
	if directory.Code != "directory_check_failed" {
		t.Fatalf("directory=%+v", directory)
	}
}
