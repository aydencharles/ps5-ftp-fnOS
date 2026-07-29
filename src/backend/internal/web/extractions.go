package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	archiveengine "github.com/chenpy/ps5-ftp-fnos/src/backend/internal/archive"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/domain"
)

type extractionRequest struct {
	Source            domain.SourceLocator `json:"source"`
	DestinationParent domain.SourceLocator `json:"destination_parent"`
	Password          string               `json:"password"`
	DeleteSources     bool                 `json:"delete_sources"`
}

func normalizedLocalPath(value string) string {
	value = strings.ReplaceAll(value, `\`, "/")
	return strings.TrimPrefix(path.Clean("/"+value), "/")
}

func (s *Server) validateExtractionRequest(request extractionRequest) (domain.ExtractionTask, error) {
	request.Source.Path = normalizedLocalPath(request.Source.Path)
	request.DestinationParent.Path = normalizedLocalPath(request.DestinationParent.Path)
	if request.Source.RootID == "" || request.DestinationParent.RootID == "" {
		return domain.ExtractionTask{}, errors.New("来源和目标存储位置不能为空")
	}
	if !archiveengine.SupportedName(path.Base(request.Source.Path)) {
		return domain.ExtractionTask{}, errors.New("只支持普通 .7z 或首卷 .7z.001")
	}
	source, err := s.library.Resolve(request.Source.RootID, request.Source.Path)
	if err != nil {
		return domain.ExtractionTask{}, err
	}
	info, err := os.Stat(source)
	if err != nil {
		return domain.ExtractionTask{}, err
	}
	if !info.Mode().IsRegular() {
		return domain.ExtractionTask{}, errors.New("解压来源必须是普通文件")
	}
	parent, err := s.library.Resolve(request.DestinationParent.RootID, request.DestinationParent.Path)
	if err != nil {
		return domain.ExtractionTask{}, err
	}
	parentInfo, err := os.Stat(parent)
	if err != nil || !parentInfo.IsDir() {
		return domain.ExtractionTask{}, errors.New("目标位置必须是已存在的文件夹")
	}
	name, err := archiveengine.DestinationName(path.Base(request.Source.Path))
	if err != nil {
		return domain.ExtractionTask{}, err
	}
	destination := path.Join(request.DestinationParent.Path, name)
	destinationAbs, err := s.library.ResolveForWrite(request.DestinationParent.RootID, destination)
	if err != nil {
		return domain.ExtractionTask{}, err
	}
	if _, err = os.Lstat(destinationAbs); err == nil {
		return domain.ExtractionTask{}, errors.New("目标文件夹已存在，不会覆盖")
	} else if !errors.Is(err, os.ErrNotExist) {
		return domain.ExtractionTask{}, err
	}
	return domain.ExtractionTask{
		Source:            request.Source,
		DestinationParent: request.DestinationParent,
		Destination:       domain.SourceLocator{RootID: request.DestinationParent.RootID, Path: destination},
		DeleteSources:     request.DeleteSources,
		Password:          request.Password,
		State:             domain.ExtractionQueued,
	}, nil
}

func (s *Server) listExtractionTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.store.ExtractionTasks(r.Context())
	if err != nil {
		fail(w, 500, err)
		return
	}
	jsonResponse(w, 200, map[string]any{"ok": true, "tasks": tasks})
}

func (s *Server) createExtractionTask(w http.ResponseWriter, r *http.Request) {
	var request extractionRequest
	if err := decode(r, &request); err != nil {
		fail(w, 400, err)
		return
	}
	task, err := s.validateExtractionRequest(request)
	if err != nil {
		fail(w, 400, err)
		return
	}
	saved, err := s.store.CreateExtractionTask(r.Context(), task)
	if err != nil {
		fail(w, 500, err)
		return
	}
	jsonResponse(w, http.StatusCreated, map[string]any{"ok": true, "task": saved})
}

func (s *Server) getExtractionTask(w http.ResponseWriter, r *http.Request) {
	task, err := s.store.ExtractionTask(r.Context(), r.PathValue("id"), false)
	if err != nil {
		fail(w, 404, err)
		return
	}
	jsonResponse(w, 200, map[string]any{"ok": true, "task": task})
}

func (s *Server) deleteExtractionTask(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteExtractionTask(r.Context(), r.PathValue("id")); err != nil {
		fail(w, 409, err)
		return
	}
	jsonResponse(w, 200, map[string]any{"ok": true})
}

func (s *Server) cancelExtractionTask(w http.ResponseWriter, r *http.Request) {
	if s.extract == nil {
		fail(w, 503, errors.New("解压队列不可用"))
		return
	}
	if err := s.extract.Cancel(r.Context(), r.PathValue("id")); err != nil {
		fail(w, 409, err)
		return
	}
	jsonResponse(w, 200, map[string]any{"ok": true})
}

func (s *Server) retryExtractionTask(w http.ResponseWriter, r *http.Request) {
	old, err := s.store.ExtractionTask(r.Context(), r.PathValue("id"), false)
	if err != nil {
		fail(w, 404, err)
		return
	}
	if !domain.ExtractionTerminal(old.State) {
		fail(w, 409, errors.New("活动中的解压任务不能重试"))
		return
	}
	var request struct {
		Password string `json:"password"`
	}
	if r.ContentLength != 0 {
		if err = decode(r, &request); err != nil {
			fail(w, 400, err)
			return
		}
	}
	validated, err := s.validateExtractionRequest(extractionRequest{
		Source: old.Source, DestinationParent: old.DestinationParent,
		Password: request.Password, DeleteSources: old.DeleteSources,
	})
	if err != nil {
		fail(w, 400, err)
		return
	}
	validated.RetryOf = old.ID
	saved, err := s.store.CreateExtractionTask(r.Context(), validated)
	if err != nil {
		fail(w, 500, err)
		return
	}
	jsonResponse(w, http.StatusCreated, map[string]any{"ok": true, "task": saved})
}

func (s *Server) extractionEvents(w http.ResponseWriter, r *http.Request) {
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
		tasks, err := s.store.ExtractionTasks(r.Context())
		if err != nil {
			return err
		}
		data, _ := json.Marshal(map[string]any{"tasks": tasks})
		_, err = fmt.Fprintf(w, "event: extraction-tasks\ndata: %s\n\n", data)
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
