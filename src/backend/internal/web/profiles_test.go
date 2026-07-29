package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
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

func TestDeletedProfileReturnsStableBusinessError(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "app.db"), filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	l := library.NewStatic(s, nil)
	handler := New(s, l, queue.New(s, l), extractqueue.New(s, l), dir).Handler()
	profile, err := s.SaveProfile(context.Background(), domain.Profile{Name: "PS5", Host: "127.0.0.1", Port: 2121, BasePath: "/"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteProfile(context.Background(), profile.ID); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/ps5/"+profile.ID+"/entries", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err = json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != codeProfileNotFound || response.Message != messageProfileNotFound || string(response.Data) != "null" {
		t.Fatalf("response=%s", recorder.Body.String())
	}
	if strings.Contains(strings.ToLower(recorder.Body.String()), "sql") {
		t.Fatalf("response leaked database details: %s", recorder.Body.String())
	}
}

func TestProfileListUsesSuccessEnvelope(t *testing.T) {
	handler, _, _ := extractionHandler(t)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/profiles", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Profiles []domain.Profile `json:"profiles"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != codeSuccess || response.Message != messageSuccess || response.Data.Profiles == nil {
		t.Fatalf("response=%s", recorder.Body.String())
	}
}

func TestDeletedProfileCannotBeUpdatedDeletedAgainOrUsedByTask(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "app.db"), filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	l := library.NewStatic(s, nil)
	handler := New(s, l, queue.New(s, l), extractqueue.New(s, l), dir).Handler()
	profile, err := s.SaveProfile(context.Background(), domain.Profile{Name: "PS5", Host: "127.0.0.1", Port: 2121, BasePath: "/", Preset: "ftpsrv"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteProfile(context.Background(), profile.ID); err != nil {
		t.Fatal(err)
	}

	requests := []*http.Request{
		httptest.NewRequest(http.MethodPut, "/api/v1/profiles/"+profile.ID, strings.NewReader(`{"name":"PS5","host":"127.0.0.1","port":2121,"username":"","password":"","base_path":"/","preset":"ftpsrv"}`)),
		httptest.NewRequest(http.MethodDelete, "/api/v1/profiles/"+profile.ID, nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(`{"type":"upload","profile_id":"`+profile.ID+`","sources":[{"root_id":"root","path":"game"}],"destination":"/","conflict_policy":"smart"}`)),
	}
	for _, request := range requests {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":1001`) || strings.Contains(strings.ToLower(recorder.Body.String()), "sql") {
			t.Fatalf("%s %s status=%d body=%s", request.Method, request.URL.Path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestInternalErrorsAreSanitized(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "app.db"), filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	l := library.NewStatic(s, nil)
	handler := New(s, l, queue.New(s, l), extractqueue.New(s, l), dir).Handler()
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/profiles", bytes.NewReader(nil)))
	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), `"code":9000`) || strings.Contains(strings.ToLower(recorder.Body.String()), "database") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPS5ConnectionFailureUsesStableBusinessError(t *testing.T) {
	handler, s, _ := extractionHandler(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			_ = connection.Close()
		}
	}()
	port := listener.Addr().(*net.TCPAddr).Port
	profile, err := s.SaveProfile(context.Background(), domain.Profile{Name: "PS5", Host: "127.0.0.1", Port: port, BasePath: "/"})
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/ps5/"+profile.ID+"/entries", nil))

	body := recorder.Body.String()
	if recorder.Code != http.StatusBadRequest || !strings.Contains(body, `"code":2001`) || !strings.Contains(body, `"message":"`+messagePS5OperationFailed+`"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, body)
	}
	if strings.Contains(strings.ToLower(body), "eof") || strings.Contains(body, "127.0.0.1") {
		t.Fatalf("response leaked connection details: %s", body)
	}
}
