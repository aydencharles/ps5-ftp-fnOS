package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"

	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/domain"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/extractqueue"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/ftpclient"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/library"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/queue"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/store"
)

type Server struct {
	store     *store.Store
	library   *library.Library
	queue     *queue.Manager
	extract   *extractqueue.Manager
	validator *validator.Validate
	uiDir     string
	mux       *http.ServeMux
}

func New(s *store.Store, l *library.Library, q *queue.Manager, extract *extractqueue.Manager, uiDir string) *Server {
	v := &Server{store: s, library: l, queue: q, extract: extract, validator: newRequestValidator(), uiDir: uiDir, mux: http.NewServeMux()}
	v.routes()
	return v
}
func (s *Server) Handler() http.Handler { return securityHeaders(s.mux) }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.health)
	s.mux.HandleFunc("GET /api/v1/bootstrap", s.bootstrap)
	s.mux.HandleFunc("GET /api/v1/profiles", s.listProfiles)
	s.mux.HandleFunc("POST /api/v1/profiles", s.saveProfile)
	s.mux.HandleFunc("PUT /api/v1/profiles/{id}", s.withValidID(s.saveProfile))
	s.mux.HandleFunc("DELETE /api/v1/profiles/{id}", s.withValidID(s.deleteProfile))
	s.mux.HandleFunc("POST /api/v1/profiles/{id}/test", s.withValidID(s.testProfile))
	s.mux.HandleFunc("GET /api/v1/library/roots", s.roots)
	s.mux.HandleFunc("GET /api/v1/library/entries", s.localEntries)
	s.mux.HandleFunc("GET /api/v1/ps5/{id}/entries", s.withValidID(s.remoteEntries))
	s.mux.HandleFunc("POST /api/v1/ps5/{id}/operations", s.withValidID(s.remoteOperation))
	s.mux.HandleFunc("GET /api/v1/tasks", s.listTasks)
	s.mux.HandleFunc("POST /api/v1/tasks", s.createTask)
	s.mux.HandleFunc("GET /api/v1/tasks/{id}", s.withValidID(s.getTask))
	s.mux.HandleFunc("DELETE /api/v1/tasks/{id}", s.withValidID(s.deleteTask))
	s.mux.HandleFunc("POST /api/v1/tasks/{id}/cancel", s.withValidID(s.cancelTask))
	s.mux.HandleFunc("POST /api/v1/tasks/{id}/retry", s.withValidID(s.retryTask))
	s.mux.HandleFunc("GET /api/v1/events", s.events)
	s.mux.HandleFunc("GET /api/v1/extraction-tasks", s.listExtractionTasks)
	s.mux.HandleFunc("POST /api/v1/extraction-tasks", s.createExtractionTask)
	s.mux.HandleFunc("GET /api/v1/extraction-tasks/{id}", s.withValidID(s.getExtractionTask))
	s.mux.HandleFunc("DELETE /api/v1/extraction-tasks/{id}", s.withValidID(s.deleteExtractionTask))
	s.mux.HandleFunc("POST /api/v1/extraction-tasks/{id}/cancel", s.withValidID(s.cancelExtractionTask))
	s.mux.HandleFunc("POST /api/v1/extraction-tasks/{id}/retry", s.withValidID(s.retryExtractionTask))
	s.mux.HandleFunc("GET /api/v1/extraction-events", s.extractionEvents)
	s.mux.HandleFunc("GET /api/v1/settings", s.settings)
	s.mux.HandleFunc("PUT /api/v1/settings", s.updateSettings)
	s.mux.Handle("/", spaHandler(s.uiDir))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		// fnOS launches desktop applications in an iframe served from its own
		// management port. That parent is cross-origin with this service port.
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors *")
		next.ServeHTTP(w, r)
	})
}
func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, err error) {
	var invalid *requestValidationError
	if errors.As(err, &invalid) {
		jsonResponse(w, status, map[string]any{"ok": false, "error": invalid.Error(), "fields": invalid.fields})
		return
	}
	jsonResponse(w, status, map[string]any{"ok": false, "error": err.Error()})
}
func decode(r *http.Request, v any) error {
	d := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if err := d.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("请求体只能包含一个 JSON 对象")
		}
		return err
	}
	return nil
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		fail(w, 503, err)
		return
	}
	jsonResponse(w, 200, map[string]any{"ok": true, "database": "ready", "time": time.Now().UTC()})
}
func (s *Server) bootstrap(w http.ResponseWriter, r *http.Request) {
	profiles, err := s.store.Profiles(r.Context())
	if err != nil {
		fail(w, 500, err)
		return
	}
	tasks, _ := s.store.Tasks(r.Context())
	extractions, _ := s.store.ExtractionTasks(r.Context())
	jsonResponse(w, 200, map[string]any{"ok": true, "name": "PS5 FTP Manager", "version": "0.2.0", "profiles": profiles, "library_roots": s.library.Roots(), "tasks": tasks, "extraction_tasks": extractions, "settings": map[string]any{"transfer_workers": s.store.Workers(r.Context())}})
}
func (s *Server) listProfiles(w http.ResponseWriter, r *http.Request) {
	v, err := s.store.Profiles(r.Context())
	if err != nil {
		fail(w, 500, err)
		return
	}
	jsonResponse(w, 200, map[string]any{"ok": true, "profiles": v})
}

func validateProfile(p *domain.Profile) error {
	base, err := store.NormalizeRemotePath(p.BasePath)
	if err != nil {
		return err
	}
	p.BasePath = base
	return nil
}
func (s *Server) saveProfile(w http.ResponseWriter, r *http.Request) {
	var body profileRequest
	if err := decode(r, &body); err != nil {
		fail(w, 400, err)
		return
	}
	body.normalize()
	if err := s.validateRequest(body); err != nil {
		fail(w, 400, err)
		return
	}
	p := body.profile(r.PathValue("id"))
	if err := validateProfile(&p); err != nil {
		fail(w, 400, err)
		return
	}
	saved, err := s.store.SaveProfile(r.Context(), p)
	if err != nil {
		fail(w, 500, err)
		return
	}
	jsonResponse(w, 200, map[string]any{"ok": true, "profile": saved})
}
func (s *Server) deleteProfile(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteProfile(r.Context(), r.PathValue("id")); err != nil {
		fail(w, 409, err)
		return
	}
	jsonResponse(w, 200, map[string]any{"ok": true})
}
func (s *Server) testProfile(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.Profile(r.Context(), r.PathValue("id"), true)
	if err != nil {
		fail(w, 404, err)
		return
	}
	jsonResponse(w, 200, ftpclient.Probe(r.Context(), p))
}

func (s *Server) roots(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, 200, map[string]any{"ok": true, "roots": s.library.Roots()})
}
func (s *Server) localEntries(w http.ResponseWriter, r *http.Request) {
	hidden := r.URL.Query().Get("hidden") == "1"
	entries, err := s.library.Entries(r.URL.Query().Get("root_id"), r.URL.Query().Get("path"), r.URL.Query().Get("query"), hidden)
	if err != nil {
		fail(w, 400, err)
		return
	}
	jsonResponse(w, 200, map[string]any{"ok": true, "entries": entries, "path": r.URL.Query().Get("path")})
}

func remotePathFor(p domain.Profile, requested string) (string, error) {
	if requested == "" {
		requested = p.BasePath
	}
	requested, err := store.NormalizeRemotePath(requested)
	if err != nil {
		return "", err
	}
	base, err := store.NormalizeRemotePath(p.BasePath)
	if err != nil {
		return "", err
	}
	if base != "/" && requested != base && !strings.HasPrefix(requested, base+"/") {
		return "", errors.New("path is outside profile base path")
	}
	return requested, nil
}
func (s *Server) withFTP(r *http.Request, fn func(domain.Profile, *ftpclient.Client) error) error {
	p, err := s.store.Profile(r.Context(), r.PathValue("id"), true)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	c, err := ftpclient.Dial(ctx, p)
	if err != nil {
		return err
	}
	defer c.Close()
	return fn(p, c)
}
func (s *Server) remoteEntries(w http.ResponseWriter, r *http.Request) {
	var entries []domain.Entry
	var current string
	err := s.withFTP(r, func(p domain.Profile, c *ftpclient.Client) error {
		var err error
		current, err = remotePathFor(p, r.URL.Query().Get("path"))
		if err != nil {
			return err
		}
		entries, err = c.List(current)
		if err != nil {
			return err
		}
		query := strings.ToLower(r.URL.Query().Get("query"))
		if query != "" {
			filtered := entries[:0]
			for _, e := range entries {
				if strings.Contains(strings.ToLower(e.Name), query) {
					filtered = append(filtered, e)
				}
			}
			entries = filtered
		}
		sortBy := r.URL.Query().Get("sort")
		if sortBy == "size" {
			sort.SliceStable(entries, func(i, j int) bool { return entries[i].Size < entries[j].Size })
		}
		return nil
	})
	if err != nil {
		fail(w, 502, err)
		return
	}
	jsonResponse(w, 200, map[string]any{"ok": true, "entries": entries, "path": current})
}

func (s *Server) remoteOperation(w http.ResponseWriter, r *http.Request) {
	var body operationRequest
	if err := decode(r, &body); err != nil {
		fail(w, 400, err)
		return
	}
	body.normalize()
	if err := s.validateRequest(body); err != nil {
		fail(w, 400, err)
		return
	}
	if body.Action == "delete" && body.IsDir && body.Recursive {
		profile, err := s.store.Profile(r.Context(), r.PathValue("id"), false)
		if err != nil {
			fail(w, 404, err)
			return
		}
		src, err := remotePathFor(profile, body.Path)
		if err != nil {
			fail(w, 400, err)
			return
		}
		if body.ConfirmName != path.Base(src) {
			fail(w, 400, errors.New("confirmation name does not match"))
			return
		}
		task, err := s.store.CreateTask(r.Context(), domain.Task{Type: "delete", ProfileID: profile.ID, Destination: src, ConflictPolicy: "smart"})
		if err != nil {
			fail(w, 500, err)
			return
		}
		jsonResponse(w, http.StatusAccepted, map[string]any{"ok": true, "task": task})
		return
	}
	err := s.withFTP(r, func(p domain.Profile, c *ftpclient.Client) error {
		src, err := remotePathFor(p, body.Path)
		if err != nil {
			return err
		}
		switch body.Action {
		case "mkdir":
			return c.MakeDir(src)
		case "rename", "move":
			dst, err := remotePathFor(p, body.Destination)
			if err != nil {
				return err
			}
			return c.Rename(src, dst)
		case "delete":
			if body.Recursive && body.ConfirmName != path.Base(src) {
				return errors.New("confirmation name does not match")
			}
			if body.IsDir {
				return c.RemoveDir(src, body.Recursive)
			}
			return c.Delete(src)
		default:
			return errors.New("unsupported operation")
		}
	})
	if err != nil {
		fail(w, 400, err)
		return
	}
	jsonResponse(w, 200, map[string]any{"ok": true})
}

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.store.Tasks(r.Context())
	if err != nil {
		fail(w, 500, err)
		return
	}
	jsonResponse(w, 200, map[string]any{"ok": true, "tasks": tasks})
}
func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	var body taskRequest
	if err := decode(r, &body); err != nil {
		fail(w, 400, err)
		return
	}
	body.normalize()
	if err := s.validateRequest(body); err != nil {
		fail(w, 400, err)
		return
	}
	t := body.task()
	if _, err := s.store.Profile(r.Context(), t.ProfileID, false); err != nil {
		fail(w, 400, errors.New("unknown profile"))
		return
	}
	profile, _ := s.store.Profile(r.Context(), t.ProfileID, false)
	switch t.Type {
	case "", "upload":
		t.Type = "upload"
		dest, err := store.NormalizeRemotePath(t.Destination)
		if err != nil {
			fail(w, 400, err)
			return
		}
		t.Destination = dest
		for _, src := range t.Sources {
			if _, resolveErr := s.library.Resolve(src.RootID, src.Path); resolveErr != nil {
				fail(w, 400, resolveErr)
				return
			}
		}
	case "download":
		rootID := t.Sources[0].RootID
		if rootID == "" {
			fail(w, 400, errors.New("download destination root is required"))
			return
		}
		// Downloads may create the selected path's descendants. ResolveForWrite
		// retains the Library Root boundary checks while permitting that target
		// directory to be created by the queued task.
		if _, resolveErr := s.library.ResolveForWrite(rootID, t.Destination); resolveErr != nil {
			fail(w, 400, resolveErr)
			return
		}
		for i := range t.Sources {
			if t.Sources[i].RootID != rootID {
				fail(w, 400, errors.New("all download sources must use the same destination root"))
				return
			}
			remote, remoteErr := remotePathFor(profile, t.Sources[i].Path)
			if remoteErr != nil {
				fail(w, 400, remoteErr)
				return
			}
			t.Sources[i].Path = remote
		}
	default:
		fail(w, 400, errors.New("unsupported task type"))
		return
	}
	saved, err := s.store.CreateTask(r.Context(), t)
	if err != nil {
		fail(w, 500, err)
		return
	}
	jsonResponse(w, 201, map[string]any{"ok": true, "task": saved})
}
func (s *Server) getTask(w http.ResponseWriter, r *http.Request) {
	t, err := s.store.Task(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, 404, err)
		return
	}
	items, _ := s.store.Items(r.Context(), t.ID)
	events, _ := s.store.Events(r.Context(), t.ID)
	jsonResponse(w, 200, map[string]any{"ok": true, "task": t, "items": items, "events": events})
}
func (s *Server) deleteTask(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteTask(r.Context(), r.PathValue("id")); err != nil {
		fail(w, 409, err)
		return
	}
	jsonResponse(w, 200, map[string]any{"ok": true})
}
func (s *Server) cancelTask(w http.ResponseWriter, r *http.Request) {
	if err := s.queue.Cancel(r.Context(), r.PathValue("id")); err != nil {
		fail(w, 409, err)
		return
	}
	jsonResponse(w, 200, map[string]any{"ok": true})
}
func (s *Server) retryTask(w http.ResponseWriter, r *http.Request) {
	old, err := s.store.Task(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, 404, err)
		return
	}
	if old.State == domain.TaskQueued || old.State == domain.TaskScanning || old.State == domain.TaskRunning || old.State == domain.TaskCanceling {
		fail(w, 409, errors.New("active task cannot be retried"))
		return
	}
	old.ID = ""
	old.State = domain.TaskQueued
	old.RetryOf = r.PathValue("id")
	old.Error = ""
	old.TotalBytes = 0
	old.TransferredBytes = 0
	saved, err := s.store.CreateTask(r.Context(), old)
	if err != nil {
		fail(w, 500, err)
		return
	}
	jsonResponse(w, 201, map[string]any{"ok": true, "task": saved})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		fail(w, 500, errors.New("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	send := func() error {
		tasks, err := s.store.Tasks(r.Context())
		if err != nil {
			return err
		}
		b, _ := json.Marshal(map[string]any{"tasks": tasks})
		_, err = fmt.Fprintf(w, "event: tasks\ndata: %s\n\n", b)
		flusher.Flush()
		return err
	}
	_ = send()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if send() != nil {
				return
			}
		}
	}
}
func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, 200, map[string]any{"ok": true, "transfer_workers": s.store.Workers(r.Context())})
}
func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var body settingsRequest
	if err := decode(r, &body); err != nil {
		fail(w, 400, err)
		return
	}
	if err := s.validateRequest(body); err != nil {
		fail(w, 400, err)
		return
	}
	if err := s.store.SetWorkers(r.Context(), body.TransferWorkers); err != nil {
		fail(w, 400, err)
		return
	}
	s.settings(w, r)
}

type spa struct {
	root  string
	files http.Handler
}

func spaHandler(root string) http.Handler {
	if root == "" {
		root = "."
	}
	return &spa{root: root, files: http.FileServer(http.Dir(root))}
}
func (h *spa) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	clean := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if clean == "." {
		clean = "index.html"
	}
	target := filepath.Join(h.root, clean)
	if rel, err := filepath.Rel(h.root, target); err != nil || strings.HasPrefix(rel, "..") {
		http.NotFound(w, r)
		return
	}
	if st, err := os.Stat(target); err == nil && !st.IsDir() {
		h.files.ServeHTTP(w, r)
		return
	}
	index := filepath.Join(h.root, "index.html")
	if _, err := os.Stat(index); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			jsonResponse(w, 503, map[string]any{"ok": false, "error": "UI build not found"})
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/"
	h.files.ServeHTTP(w, r2)
}
