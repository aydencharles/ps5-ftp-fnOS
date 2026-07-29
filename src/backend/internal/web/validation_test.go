package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteEndpointsReturnStructuredValidationErrors(t *testing.T) {
	handler, _, _ := extractionHandler(t)
	tests := []struct {
		name      string
		method    string
		path      string
		body      string
		wantField string
	}{
		{
			name: "profile trims required name", method: http.MethodPost, path: "/api/v1/profiles",
			body:      `{"name":"   ","host":"192.168.1.2","port":2120,"username":"anonymous","password":"","base_path":"/","preset":"zftpd"}`,
			wantField: "name",
		},
		{
			name: "profile rejects unsupported preset", method: http.MethodPost, path: "/api/v1/profiles",
			body:      `{"name":"PS5","host":"192.168.1.2","port":2120,"username":"anonymous","password":"","base_path":"/","preset":"other"}`,
			wantField: "preset",
		},
		{
			name: "task validates nested sources", method: http.MethodPost, path: "/api/v1/tasks",
			body:      `{"type":"upload","profile_id":"00000000000000000000000000000000","sources":[{"root_id":"root","path":""}],"destination":"/","conflict_policy":"smart"}`,
			wantField: "sources[0].path",
		},
		{
			name: "task requires destination key", method: http.MethodPost, path: "/api/v1/tasks",
			body:      `{"type":"upload","profile_id":"00000000000000000000000000000000","sources":[{"root_id":"root","path":"game"}],"conflict_policy":"smart"}`,
			wantField: "destination",
		},
		{
			name: "operation requires rename destination", method: http.MethodPost, path: "/api/v1/ps5/00000000000000000000000000000000/operations",
			body:      `{"action":"rename","path":"/game"}`,
			wantField: "destination",
		},
		{
			name: "extraction requires destination root", method: http.MethodPost, path: "/api/v1/extraction-tasks",
			body:      `{"source":{"root_id":"root","path":"Game.7z.001"},"destination_parent":{"root_id":"","path":""},"password":""}`,
			wantField: "destination_parent.root_id",
		},
		{
			name: "settings bounds workers", method: http.MethodPut, path: "/api/v1/settings",
			body:      `{"transfer_workers":5}`,
			wantField: "transfer_workers",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body))
			request.Header.Set("Content-Type", "application/json")
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var response struct {
				Error  string            `json:"error"`
				Fields map[string]string `json:"fields"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Error == "" || response.Fields[test.wantField] == "" {
				t.Fatalf("expected field %q in response: %s", test.wantField, recorder.Body.String())
			}
		})
	}
}

func TestTaskRequestRejectsServerManagedFields(t *testing.T) {
	handler, _, _ := extractionHandler(t)
	body := `{"id":"client-id","type":"upload","profile_id":"00000000000000000000000000000000","sources":[{"root_id":"root","path":"game"}],"destination":"/","conflict_policy":"smart"}`
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(body)))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `unknown field \"id\"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPathIDIsValidatedBeforeHandler(t *testing.T) {
	handler, _, _ := extractionHandler(t)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/not-an-id", nil))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"id"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
