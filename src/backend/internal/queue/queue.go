package queue

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/domain"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/ftpclient"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/library"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/store"
)

type execution struct {
	cancel  context.CancelFunc
	mu      sync.Mutex
	clients map[*ftpclient.Client]struct{}
}

func (e *execution) add(c *ftpclient.Client)    { e.mu.Lock(); e.clients[c] = struct{}{}; e.mu.Unlock() }
func (e *execution) remove(c *ftpclient.Client) { e.mu.Lock(); delete(e.clients, c); e.mu.Unlock() }
func (e *execution) stop() {
	e.cancel()
	e.mu.Lock()
	for c := range e.clients {
		_ = c.Close()
	}
	e.mu.Unlock()
}

type Manager struct {
	store    *store.Store
	library  *library.Library
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mu       sync.Mutex
	active   map[string]*execution
	profiles map[string]bool
}

func New(s *store.Store, l *library.Library) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{store: s, library: l, ctx: ctx, cancel: cancel, active: map[string]*execution{}, profiles: map[string]bool{}}
}
func (m *Manager) Start() { m.wg.Add(1); go m.loop() }
func (m *Manager) Close() {
	m.cancel()
	m.mu.Lock()
	for _, e := range m.active {
		e.stop()
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
	tasks, err := m.store.QueuedTasks(m.ctx)
	if err != nil {
		return
	}
	for _, t := range tasks {
		m.mu.Lock()
		busy := m.profiles[t.ProfileID]
		if !busy {
			m.profiles[t.ProfileID] = true
		}
		m.mu.Unlock()
		if busy {
			continue
		}
		m.wg.Add(1)
		go func(task domain.Task) {
			defer m.wg.Done()
			defer func() { m.mu.Lock(); delete(m.profiles, task.ProfileID); delete(m.active, task.ID); m.mu.Unlock() }()
			m.execute(task)
		}(t)
	}
}

func (m *Manager) Cancel(ctx context.Context, id string) error {
	if ok, err := m.store.CancelQueued(ctx, id); err != nil {
		return err
	} else if ok {
		return nil
	}
	if err := m.store.MarkCanceling(ctx, id); err != nil {
		return err
	}
	m.mu.Lock()
	e := m.active[id]
	m.mu.Unlock()
	if e == nil {
		return errors.New("task is not running")
	}
	e.stop()
	return nil
}

func (m *Manager) execute(task domain.Task) {
	ctx, cancel := context.WithCancel(m.ctx)
	exec := &execution{cancel: cancel, clients: map[*ftpclient.Client]struct{}{}}
	m.mu.Lock()
	m.active[task.ID] = exec
	m.mu.Unlock()
	defer cancel()
	if task.Type == "delete" {
		m.executeRemoteDelete(ctx, exec, task)
		return
	}
	if task.Type == "download" {
		m.executeDownload(ctx, exec, task)
		return
	}
	_ = m.store.SetTaskState(ctx, task.ID, domain.TaskScanning, "")
	_ = m.store.Event(ctx, task.ID, "info", "开始扫描源文件")
	items, total, err := m.scan(task)
	if err != nil {
		m.finish(ctx, task.ID, domain.TaskFailed, err)
		return
	}
	if err = m.store.ReplaceItems(ctx, task.ID, items); err != nil {
		m.finish(ctx, task.ID, domain.TaskFailed, err)
		return
	}
	_ = m.store.SetTaskPlan(ctx, task.ID, len(items), total)
	if err = m.store.SetTaskState(ctx, task.ID, domain.TaskRunning, ""); err != nil {
		return
	}
	_ = m.store.Event(ctx, task.ID, "info", fmt.Sprintf("扫描完成：%d 项，%d 字节", len(items), total))
	profile, err := m.store.Profile(ctx, task.ProfileID, true)
	if err != nil {
		m.finish(ctx, task.ID, domain.TaskFailed, err)
		return
	}
	workers := m.store.Workers(ctx)
	dirClient, err := m.dial(ctx, exec, profile)
	if err != nil {
		m.finish(ctx, task.ID, domain.TaskFailed, err)
		return
	}
	for _, item := range items {
		if !item.IsDir {
			continue
		}
		if err = dirClient.MakeDirAllFrom(profile.BasePath, item.Destination); err != nil {
			exec.remove(dirClient)
			_ = dirClient.Close()
			m.finish(ctx, task.ID, domain.TaskFailed, err)
			return
		}
		_ = m.store.SetItem(ctx, item.ID, "succeeded", 0, 1, "")
		_ = m.store.IncrementTaskResult(ctx, task.ID, 0, false)
	}
	exec.remove(dirClient)
	_ = dirClient.Close()
	files := make(chan domain.TaskItem)
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex
	var progress atomic.Int64
	var currentMu sync.Mutex
	current := ""
	progress.Store(0)
	reportDone := make(chan struct{})
	go m.report(ctx, task.ID, total, &progress, &currentMu, &current, reportDone)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range files {
				if ctx.Err() != nil {
					return
				}
				currentMu.Lock()
				current = item.SourcePath
				currentMu.Unlock()
				skipped, e := m.transferItem(ctx, exec, profile, task, item, &progress)
				if e != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = e
						cancel()
					}
					errMu.Unlock()
					return
				}
				_ = m.store.IncrementTaskResult(context.Background(), task.ID, item.Size, skipped)
			}
		}()
	}
sendLoop:
	for _, item := range items {
		if item.IsDir {
			continue
		}
		select {
		case files <- item:
		case <-ctx.Done():
			break sendLoop
		}
	}
	close(files)
	wg.Wait()
	close(reportDone)
	currentMu.Lock()
	lastCurrent := current
	currentMu.Unlock()
	_ = m.store.UpdateTaskProgress(context.Background(), task.ID, progress.Load(), 0, nil, lastCurrent)
	if errors.Is(ctx.Err(), context.Canceled) {
		if m.store.IsCanceling(context.Background(), task.ID) {
			m.finish(context.Background(), task.ID, domain.TaskCanceled, errors.New("任务已取消"))
		} else if firstErr != nil {
			m.finish(context.Background(), task.ID, domain.TaskFailed, firstErr)
		} else {
			m.finish(context.Background(), task.ID, domain.TaskInterrupted, ctx.Err())
		}
		return
	}
	if firstErr != nil {
		m.finish(ctx, task.ID, domain.TaskFailed, firstErr)
		return
	}
	_ = m.store.UpdateTaskProgress(ctx, task.ID, total, 0, nil, "")
	m.finish(ctx, task.ID, domain.TaskSucceeded, nil)
}

func (m *Manager) executeDownload(ctx context.Context, exec *execution, task domain.Task) {
	_ = m.store.SetTaskState(ctx, task.ID, domain.TaskScanning, "")
	_ = m.store.Event(ctx, task.ID, "info", "开始扫描 PS5 源文件")
	profile, err := m.store.Profile(ctx, task.ProfileID, true)
	if err != nil {
		m.finish(ctx, task.ID, domain.TaskFailed, err)
		return
	}
	scanner, err := m.dial(ctx, exec, profile)
	if err != nil {
		m.finish(ctx, task.ID, domain.TaskFailed, err)
		return
	}
	items, total, err := m.scanRemoteDownload(ctx, scanner, task)
	exec.remove(scanner)
	_ = scanner.Close()
	if err != nil {
		m.finish(ctx, task.ID, domain.TaskFailed, err)
		return
	}
	if err = m.store.ReplaceItems(ctx, task.ID, items); err != nil {
		m.finish(ctx, task.ID, domain.TaskFailed, err)
		return
	}
	_ = m.store.SetTaskPlan(ctx, task.ID, len(items), total)
	if err = m.store.SetTaskState(ctx, task.ID, domain.TaskRunning, ""); err != nil {
		return
	}
	_ = m.store.Event(ctx, task.ID, "info", fmt.Sprintf("扫描完成：%d 项，%d 字节", len(items), total))
	for _, item := range items {
		if !item.IsDir {
			continue
		}
		destination, resolveErr := m.library.ResolveForWrite(item.RootID, item.Destination)
		if resolveErr == nil {
			resolveErr = os.MkdirAll(destination, 0o755)
		}
		if resolveErr != nil {
			m.finish(ctx, task.ID, domain.TaskFailed, resolveErr)
			return
		}
		_ = m.store.SetItem(ctx, item.ID, "succeeded", 0, 1, "")
		_ = m.store.IncrementTaskResult(ctx, task.ID, 0, false)
	}
	files := make(chan domain.TaskItem)
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex
	var progress atomic.Int64
	var currentMu sync.Mutex
	current := ""
	reportDone := make(chan struct{})
	go m.report(ctx, task.ID, total, &progress, &currentMu, &current, reportDone)
	for w := 0; w < m.store.Workers(ctx); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range files {
				if ctx.Err() != nil {
					return
				}
				currentMu.Lock()
				current = item.SourcePath
				currentMu.Unlock()
				skipped, transferErr := m.downloadItem(ctx, exec, profile, task, item, &progress)
				if transferErr != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = transferErr
						exec.cancel()
					}
					errMu.Unlock()
					return
				}
				_ = m.store.IncrementTaskResult(context.Background(), task.ID, item.Size, skipped)
			}
		}()
	}
sendDownloadLoop:
	for _, item := range items {
		if item.IsDir {
			continue
		}
		select {
		case files <- item:
		case <-ctx.Done():
			break sendDownloadLoop
		}
	}
	close(files)
	wg.Wait()
	close(reportDone)
	currentMu.Lock()
	lastCurrent := current
	currentMu.Unlock()
	_ = m.store.UpdateTaskProgress(context.Background(), task.ID, progress.Load(), 0, nil, lastCurrent)
	if errors.Is(ctx.Err(), context.Canceled) {
		if m.store.IsCanceling(context.Background(), task.ID) {
			m.finish(context.Background(), task.ID, domain.TaskCanceled, errors.New("任务已取消"))
		} else if firstErr != nil {
			m.finish(context.Background(), task.ID, domain.TaskFailed, firstErr)
		} else {
			m.finish(context.Background(), task.ID, domain.TaskInterrupted, ctx.Err())
		}
		return
	}
	if firstErr != nil {
		m.finish(ctx, task.ID, domain.TaskFailed, firstErr)
		return
	}
	_ = m.store.UpdateTaskProgress(ctx, task.ID, total, 0, nil, "")
	m.finish(ctx, task.ID, domain.TaskSucceeded, nil)
}

func (m *Manager) scanRemoteDownload(ctx context.Context, client *ftpclient.Client, task domain.Task) ([]domain.TaskItem, int64, error) {
	sources := dedupe(append([]domain.SourceLocator(nil), task.Sources...))
	items := make([]domain.TaskItem, 0)
	var total int64
	var walk func(rootID, remote, destination string) error
	walk = func(rootID, remote, destination string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry, err := remoteEntry(client, remote)
		if err != nil {
			return err
		}
		items = append(items, domain.TaskItem{TaskID: task.ID, RootID: rootID, SourcePath: remote, Destination: destination, IsDir: entry.IsDir, Size: entry.Size, ModUnixNano: entry.ModifiedAt.UnixNano()})
		if !entry.IsDir {
			total += entry.Size
			return nil
		}
		children, err := client.List(remote)
		if err != nil {
			return err
		}
		for _, child := range children {
			if err = walk(rootID, child.Path, path.Join(destination, child.Name)); err != nil {
				return err
			}
		}
		return nil
	}
	for _, source := range sources {
		if err := walk(source.RootID, source.Path, path.Join(task.Destination, path.Base(source.Path))); err != nil {
			return nil, 0, err
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		if items[i].IsDir {
			return strings.Count(items[i].Destination, "/") < strings.Count(items[j].Destination, "/")
		}
		return items[i].Destination < items[j].Destination
	})
	return items, total, nil
}

func remoteEntry(client *ftpclient.Client, remote string) (domain.Entry, error) {
	entries, err := client.List(path.Dir(remote))
	if err != nil {
		return domain.Entry{}, err
	}
	for _, entry := range entries {
		if entry.Path == remote {
			return entry, nil
		}
	}
	return domain.Entry{}, fmt.Errorf("远端文件不存在: %s", remote)
}

func (m *Manager) executeRemoteDelete(ctx context.Context, exec *execution, task domain.Task) {
	_ = m.store.SetTaskState(ctx, task.ID, domain.TaskScanning, "")
	_ = m.store.Event(ctx, task.ID, "warning", "开始扫描永久删除目录")
	profile, err := m.store.Profile(ctx, task.ProfileID, true)
	if err != nil {
		m.finish(ctx, task.ID, domain.TaskFailed, err)
		return
	}
	client, err := m.dial(ctx, exec, profile)
	if err != nil {
		m.finish(ctx, task.ID, domain.TaskFailed, err)
		return
	}
	defer func() { exec.remove(client); _ = client.Close() }()
	items, total, err := collectRemoteDeleteItems(ctx, client, task.ID, task.Destination)
	if err != nil {
		m.finish(ctx, task.ID, domain.TaskFailed, err)
		return
	}
	if err = m.store.ReplaceItems(ctx, task.ID, items); err != nil {
		m.finish(ctx, task.ID, domain.TaskFailed, err)
		return
	}
	_ = m.store.SetTaskPlan(ctx, task.ID, len(items), total)
	_ = m.store.SetTaskState(ctx, task.ID, domain.TaskRunning, "")
	var transferred int64
	for _, item := range items {
		if ctx.Err() != nil {
			m.finish(context.Background(), task.ID, domain.TaskCanceled, errors.New("删除任务已取消，已删除内容无法恢复"))
			return
		}
		_ = m.store.UpdateTaskProgress(ctx, task.ID, transferred, 0, nil, item.SourcePath)
		if item.IsDir {
			err = client.RemoveDir(item.SourcePath, false)
		} else {
			err = client.Delete(item.SourcePath)
		}
		if err != nil {
			if m.store.IsCanceling(context.Background(), task.ID) {
				m.finish(context.Background(), task.ID, domain.TaskCanceled, errors.New("删除任务已取消，已删除内容无法恢复"))
			} else {
				m.finish(ctx, task.ID, domain.TaskFailed, err)
			}
			return
		}
		transferred += item.Size
		_ = m.store.SetItem(ctx, item.ID, "succeeded", item.Size, 1, "")
		_ = m.store.IncrementTaskResult(ctx, task.ID, item.Size, false)
	}
	_ = m.store.UpdateTaskProgress(ctx, task.ID, total, 0, nil, "")
	m.finish(ctx, task.ID, domain.TaskSucceeded, nil)
}

func collectRemoteDeleteItems(ctx context.Context, client *ftpclient.Client, taskID, root string) ([]domain.TaskItem, int64, error) {
	var items []domain.TaskItem
	var total int64
	var walk func(string) error
	walk = func(current string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, err := client.List(current)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir {
				if err = walk(entry.Path); err != nil {
					return err
				}
			} else {
				total += entry.Size
			}
			items = append(items, domain.TaskItem{TaskID: taskID, SourcePath: entry.Path, Destination: entry.Path, IsDir: entry.IsDir, Size: entry.Size, ModUnixNano: entry.ModifiedAt.UnixNano()})
		}
		return nil
	}
	if err := walk(root); err != nil {
		return nil, 0, err
	}
	items = append(items, domain.TaskItem{TaskID: taskID, SourcePath: root, Destination: root, IsDir: true})
	sort.SliceStable(items, func(i, j int) bool {
		di, dj := strings.Count(items[i].SourcePath, "/"), strings.Count(items[j].SourcePath, "/")
		if di != dj {
			return di > dj
		}
		if items[i].IsDir != items[j].IsDir {
			return !items[i].IsDir
		}
		return items[i].SourcePath > items[j].SourcePath
	})
	return items, total, nil
}

func (m *Manager) finish(ctx context.Context, id, state string, err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	_ = m.store.SetTaskState(ctx, id, state, msg)
	level := "info"
	if state == domain.TaskFailed {
		level = "error"
	}
	_ = m.store.Event(ctx, id, level, "任务状态："+state+func() string {
		if msg != "" {
			return " - " + msg
		}
		return ""
	}())
}

func (m *Manager) scan(task domain.Task) ([]domain.TaskItem, int64, error) {
	sources := dedupe(task.Sources)
	var out []domain.TaskItem
	var total int64
	for _, src := range sources {
		abs, err := m.library.Resolve(src.RootID, src.Path)
		if err != nil {
			return nil, 0, err
		}
		info, err := os.Stat(abs)
		if err != nil {
			return nil, 0, err
		}
		baseDest := path.Join(task.Destination, filepath.Base(abs))
		if !info.IsDir() {
			out = append(out, domain.TaskItem{RootID: src.RootID, SourcePath: src.Path, Destination: baseDest, Size: info.Size(), ModUnixNano: info.ModTime().UnixNano()})
			total += info.Size()
			continue
		}
		err = filepath.WalkDir(abs, func(p string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, _ := filepath.Rel(abs, p)
			dest := baseDest
			if rel != "." {
				dest = path.Join(baseDest, filepath.ToSlash(rel))
			}
			st, e := d.Info()
			if e != nil {
				return e
			}
			relativeToRoot, _ := filepath.Rel(mustRoot(m.library, src.RootID), p)
			item := domain.TaskItem{RootID: src.RootID, SourcePath: filepath.ToSlash(relativeToRoot), Destination: dest, IsDir: d.IsDir(), ModUnixNano: st.ModTime().UnixNano()}
			if !d.IsDir() {
				item.Size = st.Size()
				total += st.Size()
			}
			out = append(out, item)
			return nil
		})
		if err != nil {
			return nil, 0, err
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return out[i].Destination < out[j].Destination
	})
	return out, total, nil
}

// mustRoot obtains the stable absolute root by resolving its empty locator.
func mustRoot(l *library.Library, id string) string {
	p, err := l.Resolve(id, "")
	if err != nil {
		return string(filepath.Separator)
	}
	return p
}
func dedupe(in []domain.SourceLocator) []domain.SourceLocator {
	sort.Slice(in, func(i, j int) bool {
		if in[i].RootID != in[j].RootID {
			return in[i].RootID < in[j].RootID
		}
		return in[i].Path < in[j].Path
	})
	var out []domain.SourceLocator
	for _, s := range in {
		skip := false
		for _, p := range out {
			if p.RootID == s.RootID && (s.Path == p.Path || strings.HasPrefix(s.Path, p.Path+"/")) {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, s)
		}
	}
	return out
}

func (m *Manager) dial(ctx context.Context, e *execution, p domain.Profile) (*ftpclient.Client, error) {
	c, err := ftpclient.Dial(ctx, p)
	if err == nil {
		e.add(c)
	}
	return c, err
}

func (m *Manager) downloadItem(ctx context.Context, e *execution, profile domain.Profile, task domain.Task, item domain.TaskItem, progress *atomic.Int64) (bool, error) {
	destination, err := m.library.ResolveForWrite(item.RootID, item.Destination)
	if err != nil {
		return false, err
	}
	if err = os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return false, err
	}
	info, statErr := os.Stat(destination)
	exists := statErr == nil
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return false, statErr
	}
	if exists && info.IsDir() {
		return false, fmt.Errorf("本地目标是目录: %s", item.Destination)
	}
	if exists && task.ConflictPolicy == "fail" {
		return false, fmt.Errorf("目标已存在: %s", item.Destination)
	}
	if exists && task.ConflictPolicy == "smart" && info.Size() == item.Size {
		_ = m.store.SetItem(ctx, item.ID, "skipped", item.Size, 0, "")
		progress.Add(item.Size)
		return true, nil
	}
	tmp := filepath.Join(filepath.Dir(destination), "."+filepath.Base(destination)+".ps5ftp-"+task.ID[:8]+".part")
	backup := filepath.Join(filepath.Dir(destination), "."+filepath.Base(destination)+".ps5ftp-"+task.ID[:8]+".bak")
	defer func() {
		if ctx.Err() != nil {
			_ = os.Remove(tmp)
		}
	}()
	var downloaded int64
	attempts := 0
	for attempts < 3 {
		attempts++
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		offset := int64(0)
		if partial, partialErr := os.Stat(tmp); partialErr == nil {
			switch {
			case partial.Size() > item.Size:
				if err = os.Remove(tmp); err != nil {
					return false, err
				}
			case partial.Size() > 0:
				offset = partial.Size()
			}
		}
		if downloaded > 0 {
			progress.Add(-downloaded)
		}
		downloaded = offset
		progress.Add(offset)
		if offset == item.Size {
			break
		}
		file, openErr := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY, 0o600)
		if openErr != nil {
			return false, openErr
		}
		if offset == 0 {
			openErr = file.Truncate(0)
		} else {
			_, openErr = file.Seek(offset, io.SeekStart)
		}
		if openErr != nil {
			_ = file.Close()
			return false, openErr
		}
		client, dialErr := m.dial(ctx, e, profile)
		if dialErr != nil {
			_ = file.Close()
			err = dialErr
		} else {
			writer := &ftpclient.ContextWriter{Context: ctx, Writer: file, OnWrite: func(n int) { progress.Add(int64(n)); downloaded += int64(n) }}
			err = client.DownloadTo(item.SourcePath, writer, uint64(offset))
			if err == nil {
				err = file.Sync()
			}
			e.remove(client)
			_ = client.Close()
			_ = file.Close()
		}
		_ = m.store.SetItem(context.Background(), item.ID, "transferring", downloaded, attempts, errorString(err))
		if err == nil && downloaded == item.Size {
			break
		}
		if err == nil {
			err = fmt.Errorf("下载未完成 %s: %d/%d", item.SourcePath, downloaded, item.Size)
		}
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(time.Duration(attempts) * time.Second):
		}
	}
	if downloaded != item.Size {
		return false, fmt.Errorf("下载未完成 %s: %d/%d", item.SourcePath, downloaded, item.Size)
	}
	if partial, statErr := os.Stat(tmp); statErr != nil || partial.Size() != item.Size {
		if statErr != nil {
			return false, statErr
		}
		return false, fmt.Errorf("本地大小校验失败 %s: 期望 %d，实际 %d", item.Destination, item.Size, partial.Size())
	}
	control, dialErr := m.dial(ctx, e, profile)
	if dialErr != nil {
		return false, fmt.Errorf("远端大小校验失败 %s: 重新连接 FTP 失败: %w", item.SourcePath, dialErr)
	}
	_, err = verifyRemoteSize(ctx, item.Size, func() (int64, error) {
		return control.Size(item.SourcePath)
	}, 350*time.Millisecond)
	e.remove(control)
	_ = control.Close()
	if err != nil {
		return false, fmt.Errorf("远端大小校验失败 %s: %w", item.SourcePath, err)
	}
	if exists {
		_ = os.Remove(backup)
		if err = os.Rename(destination, backup); err != nil {
			return false, err
		}
		if err = os.Rename(tmp, destination); err != nil {
			_ = os.Rename(backup, destination)
			return false, err
		}
		_ = os.Remove(backup)
	} else if err = os.Rename(tmp, destination); err != nil {
		return false, err
	}
	_ = m.store.SetItem(ctx, item.ID, "succeeded", item.Size, attempts, "")
	return false, nil
}

func (m *Manager) transferItem(ctx context.Context, e *execution, p domain.Profile, t domain.Task, item domain.TaskItem, progress *atomic.Int64) (bool, error) {
	abs, err := m.library.Resolve(item.RootID, item.SourcePath)
	if err != nil {
		return false, err
	}
	parent := path.Dir(item.Destination)
	control, err := m.dial(ctx, e, p)
	if err != nil {
		return false, err
	}
	defer func() { e.remove(control); _ = control.Close() }()
	if err = control.MakeDirAllFrom(p.BasePath, parent); err != nil {
		return false, err
	}
	exists, size, err := control.Exists(item.Destination)
	if err != nil {
		return false, err
	}
	if exists && t.ConflictPolicy == "fail" {
		return false, fmt.Errorf("目标已存在: %s", item.Destination)
	}
	if exists && t.ConflictPolicy == "smart" && size == item.Size {
		_ = m.store.SetItem(ctx, item.ID, "skipped", item.Size, 0, "")
		progress.Add(item.Size)
		return true, nil
	}
	tmp := path.Join(parent, "."+path.Base(item.Destination)+".ps5ftp-"+t.ID[:8]+".part")
	backup := path.Join(parent, "."+path.Base(item.Destination)+".ps5ftp-"+t.ID[:8]+".bak")
	defer func() {
		if ctx.Err() != nil {
			cleanup, er := ftpclient.Dial(context.Background(), p)
			if er == nil {
				_ = cleanup.Delete(tmp)
				_ = cleanup.Close()
			}
		}
	}()
	var uploaded int64
	attempts := 0
	for attempts < 3 {
		attempts++
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		remoteOffset := int64(0)
		if ok, n, _ := control.Exists(tmp); ok && n > 0 && n < item.Size {
			remoteOffset = n
		}
		if remoteOffset > item.Size {
			_ = control.Delete(tmp)
			remoteOffset = 0
		}
		f, er := os.Open(abs)
		if er != nil {
			return false, er
		}
		if remoteOffset > 0 {
			_, er = f.Seek(remoteOffset, io.SeekStart)
			if er != nil {
				f.Close()
				return false, er
			}
		}
		before := progress.Load()
		if uploaded > 0 {
			progress.Add(-uploaded)
		}
		uploaded = remoteOffset
		progress.Add(remoteOffset)
		reader := &ftpclient.ContextReader{Context: ctx, Reader: f, OnRead: func(n int) { progress.Add(int64(n)); uploaded += int64(n) }}
		uploader, dialErr := m.dial(ctx, e, p)
		if dialErr != nil {
			f.Close()
			er = dialErr
		} else {
			er = uploader.UploadFrom(tmp, reader, uint64(remoteOffset))
			e.remove(uploader)
			_ = uploader.Close()
			f.Close()
		}
		_ = m.store.SetItem(context.Background(), item.ID, "transferring", uploaded, attempts, errorString(er))
		if er == nil {
			break
		}
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		_ = before
		time.Sleep(time.Duration(attempts) * time.Second)
	}
	if uploaded != item.Size {
		return false, fmt.Errorf("上传未完成 %s: %d/%d", item.SourcePath, uploaded, item.Size)
	}
	// The control connection has been idle while the dedicated data connection
	// uploads the file. PS5 FTP servers can close that idle connection during a
	// large transfer, so reconnect before validation and the final rename.
	e.remove(control)
	_ = control.Close()
	freshControl, dialErr := m.dial(ctx, e, p)
	if dialErr != nil {
		return false, fmt.Errorf("远端大小校验失败 %s: 重新连接 FTP 失败: %w", item.Destination, dialErr)
	}
	control = freshControl
	if _, err = verifyRemoteSize(ctx, item.Size, func() (int64, error) {
		return control.Size(tmp)
	}, 350*time.Millisecond); err != nil {
		return false, fmt.Errorf("远端大小校验失败 %s: %w", item.Destination, err)
	}
	after, err := os.Stat(abs)
	if err != nil || after.Size() != item.Size || after.ModTime().UnixNano() != item.ModUnixNano {
		return false, fmt.Errorf("源文件在传输期间发生变化: %s", item.SourcePath)
	}
	if exists {
		_ = control.Delete(backup)
		if err = control.Rename(item.Destination, backup); err != nil {
			return false, err
		}
		if err = control.Rename(tmp, item.Destination); err != nil {
			_ = control.Rename(backup, item.Destination)
			return false, err
		}
		_ = control.Delete(backup)
	} else if err = control.Rename(tmp, item.Destination); err != nil {
		return false, err
	}
	_ = m.store.SetItem(ctx, item.ID, "succeeded", item.Size, attempts, "")
	return false, nil
}

func verifyRemoteSize(ctx context.Context, expected int64, size func() (int64, error), retryDelay time.Duration) (int64, error) {
	var remoteSize int64
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		remoteSize, lastErr = size()
		if lastErr == nil && remoteSize == expected {
			return remoteSize, nil
		}
		if attempt == 3 {
			break
		}
		timer := time.NewTimer(time.Duration(attempt) * retryDelay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return remoteSize, ctx.Err()
		case <-timer.C:
		}
	}
	if lastErr != nil {
		return remoteSize, fmt.Errorf("FTP SIZE 命令失败（本地 %d 字节）: %w", expected, lastErr)
	}
	return remoteSize, fmt.Errorf("大小不一致（本地 %d 字节，远端 %d 字节）", expected, remoteSize)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
func (m *Manager) report(ctx context.Context, id string, total int64, bytes *atomic.Int64, currentMu *sync.Mutex, current *string, done <-chan struct{}) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	last := int64(0)
	speed := float64(0)
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			n := bytes.Load()
			delta := n - last
			last = n
			instant := float64(delta) * 2
			if speed == 0 {
				speed = instant
			} else {
				speed = .7*speed + .3*instant
			}
			var eta *int64
			if speed > 1 && total > n {
				v := int64(float64(total-n) / speed)
				eta = &v
			}
			currentMu.Lock()
			name := *current
			currentMu.Unlock()
			_ = m.store.UpdateTaskProgress(context.Background(), id, n, speed, eta, name)
		}
	}
}
