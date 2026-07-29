package extractqueue

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	archiveengine "github.com/chenpy/ps5-ftp-fnos/src/backend/internal/archive"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/domain"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/library"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/store"
)

type execution struct {
	cancel context.CancelFunc
}

type Manager struct {
	store   *store.Store
	library *library.Library
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	mu      sync.Mutex
	active  map[string]*execution
	running bool
}

func New(s *store.Store, l *library.Library) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{store: s, library: l, ctx: ctx, cancel: cancel, active: make(map[string]*execution)}
}

func (m *Manager) Start() {
	m.wg.Add(1)
	go m.loop()
}

func (m *Manager) Close() {
	m.cancel()
	m.mu.Lock()
	for _, active := range m.active {
		active.cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
}

func (m *Manager) loop() {
	defer m.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.dispatch()
		}
	}
}

func (m *Manager) dispatch() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	tasks, err := m.store.QueuedExtractionTasks(m.ctx)
	if err != nil || len(tasks) == 0 {
		return
	}
	task := tasks[0]
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	taskCtx, taskCancel := context.WithCancel(m.ctx)
	m.running = true
	m.active[task.ID] = &execution{cancel: taskCancel}
	m.mu.Unlock()
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer func() {
			m.mu.Lock()
			delete(m.active, task.ID)
			m.running = false
			m.mu.Unlock()
		}()
		m.execute(taskCtx, taskCancel, task.ID)
	}()
}

func (m *Manager) Cancel(ctx context.Context, id string) error {
	if canceled, err := m.store.CancelQueuedExtraction(ctx, id); err != nil {
		return err
	} else if canceled {
		return nil
	}
	if err := m.store.MarkExtractionCanceling(ctx, id); err != nil {
		return err
	}
	m.mu.Lock()
	active := m.active[id]
	m.mu.Unlock()
	if active == nil {
		return errors.New("解压任务尚未开始或已不可取消")
	}
	active.cancel()
	return nil
}

func (m *Manager) CleanupInterrupted(tasks []domain.ExtractionTask) {
	for _, task := range tasks {
		parent, err := m.library.Resolve(task.DestinationParent.RootID, task.DestinationParent.Path)
		if err != nil {
			continue
		}
		_ = archiveengine.CleanupStage(filepath.Join(parent, archiveengine.StageName(task.ID)))
	}
}

type liveProgress struct {
	bytes     atomic.Int64
	completed atomic.Int64
	mu        sync.Mutex
	current   string
}

func (p *liveProgress) update(value archiveengine.Progress) {
	p.bytes.Store(value.Bytes)
	p.completed.Store(int64(value.Completed))
	p.mu.Lock()
	p.current = value.CurrentFile
	p.mu.Unlock()
}

func (p *liveProgress) snapshot() (int64, int, string) {
	p.mu.Lock()
	current := p.current
	p.mu.Unlock()
	return p.bytes.Load(), int(p.completed.Load()), current
}

func (m *Manager) report(ctx context.Context, id string, total int64, progress *liveProgress, done <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastAt := time.Now()
	var lastBytes int64
	var smoothed float64
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			bytes, completed, current := progress.snapshot()
			elapsed := now.Sub(lastAt).Seconds()
			instant := float64(bytes-lastBytes) / elapsed
			if smoothed == 0 {
				smoothed = instant
			} else {
				smoothed = smoothed*0.7 + instant*0.3
			}
			var eta *int64
			if smoothed > 0 && total > bytes {
				value := int64(float64(total-bytes) / smoothed)
				eta = &value
			}
			_ = m.store.UpdateExtractionProgress(context.Background(), id, bytes, completed, smoothed, eta, current)
			lastAt, lastBytes = now, bytes
		}
	}
}

func extractionError(err error) error {
	if archiveengine.IsEncryptedError(err) {
		return errors.New("密码错误或压缩包损坏")
	}
	message := err.Error()
	if strings.Contains(message, "checksum") || strings.Contains(message, "not a valid 7-zip") || strings.Contains(message, "unexpected EOF") {
		return fmt.Errorf("压缩包损坏或分卷不完整：%w", err)
	}
	return err
}

func (m *Manager) execute(ctx context.Context, cancel context.CancelFunc, id string) {
	defer cancel()

	claimed, err := m.store.ClaimExtraction(ctx, id)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return
		}
		m.finish(id, domain.ExtractionFailed, err, "")
		return
	}
	if !claimed {
		return
	}
	task, err := m.store.ExtractionTask(ctx, id, true)
	if err != nil {
		m.failOrStop(ctx, id, err)
		return
	}
	source, err := m.library.Resolve(task.Source.RootID, task.Source.Path)
	if err != nil {
		m.failOrStop(ctx, id, err)
		return
	}
	parent, err := m.library.Resolve(task.DestinationParent.RootID, task.DestinationParent.Path)
	if err != nil {
		m.failOrStop(ctx, id, err)
		return
	}
	if info, statErr := os.Stat(parent); statErr != nil || !info.IsDir() {
		if statErr == nil {
			statErr = errors.New("解压目标不是文件夹")
		}
		m.failOrStop(ctx, id, statErr)
		return
	}
	destination, err := m.library.ResolveForWrite(task.Destination.RootID, task.Destination.Path)
	if err != nil {
		m.failOrStop(ctx, id, err)
		return
	}
	if _, statErr := os.Lstat(destination); statErr == nil {
		m.failOrStop(ctx, id, errors.New("目标文件夹已存在，不会覆盖"))
		return
	} else if !errors.Is(statErr, os.ErrNotExist) {
		m.failOrStop(ctx, id, statErr)
		return
	}
	stage := filepath.Join(parent, archiveengine.StageName(id))
	_ = archiveengine.CleanupStage(stage)
	published := false
	defer func() {
		if !published {
			_ = archiveengine.CleanupStage(stage)
		}
	}()
	if err = ctx.Err(); err != nil {
		m.failOrStop(ctx, id, err)
		return
	}
	plan, err := archiveengine.Open(source, task.Password)
	if err != nil {
		m.failOrStop(ctx, id, extractionError(err))
		return
	}
	volumes := append([]archiveengine.Volume(nil), plan.Volumes...)
	if err = archiveengine.CheckSpace(parent, plan.TotalBytes); err != nil {
		_ = plan.Close()
		m.failOrStop(ctx, id, err)
		return
	}
	if err = ctx.Err(); err != nil {
		_ = plan.Close()
		m.failOrStop(ctx, id, err)
		return
	}
	begun, err := m.store.BeginExtraction(context.Background(), id, len(plan.Entries), plan.TotalBytes)
	if err != nil {
		_ = plan.Close()
		m.finish(id, domain.ExtractionFailed, err, "")
		return
	}
	if !begun {
		_ = plan.Close()
		m.failOrStop(ctx, id, errors.New("解压任务状态已改变"))
		return
	}
	progress := &liveProgress{}
	reportDone := make(chan struct{})
	reportStopped := make(chan struct{})
	go func() {
		defer close(reportStopped)
		m.report(ctx, id, plan.TotalBytes, progress, reportDone)
	}()
	err = archiveengine.Extract(ctx, plan, stage, progress.update)
	closeErr := plan.Close()
	close(reportDone)
	<-reportStopped
	bytes, completed, current := progress.snapshot()
	_ = m.store.UpdateExtractionProgress(context.Background(), id, bytes, completed, 0, nil, current)
	if err == nil {
		err = closeErr
	}
	if err != nil {
		m.failOrStop(ctx, id, extractionError(err))
		return
	}
	if err = ctx.Err(); err != nil {
		m.failOrStop(ctx, id, err)
		return
	}
	committing, err := m.store.BeginExtractionCommit(context.Background(), id)
	if err != nil {
		m.finish(id, domain.ExtractionFailed, err, "")
		return
	}
	if !committing {
		m.failOrStop(ctx, id, errors.New("解压任务状态已改变"))
		return
	}
	if err = archiveengine.Publish(stage, destination); err != nil {
		if errors.Is(err, os.ErrExist) {
			err = errors.New("目标文件夹已存在，不会覆盖")
		}
		m.finish(id, domain.ExtractionFailed, err, "")
		return
	}
	published = true
	warning := ""
	if task.DeleteSources {
		if failed := archiveengine.DeleteVolumes(volumes); len(failed) > 0 {
			names := make([]string, 0, len(failed))
			for _, failedPath := range failed {
				names = append(names, filepath.Base(failedPath))
			}
			warning = "解压成功，但以下源分卷未能删除：" + strings.Join(names, "、")
		}
	}
	_ = m.store.UpdateExtractionProgress(context.Background(), id, plan.TotalBytes, len(plan.Entries), 0, nil, "")
	m.finish(id, domain.ExtractionSucceeded, nil, warning)
}

func (m *Manager) failOrStop(ctx context.Context, id string, err error) {
	if m.store.ExtractionIsCanceling(context.Background(), id) {
		m.finish(id, domain.ExtractionCanceled, errors.New("解压任务已取消"), "")
		return
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		m.finish(id, domain.ExtractionInterrupted, ctx.Err(), "")
		return
	}
	m.finish(id, domain.ExtractionFailed, err, "")
}

func (m *Manager) finish(id, state string, err error, warning string) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	_ = m.store.SetExtractionState(context.Background(), id, state, message, warning)
}
