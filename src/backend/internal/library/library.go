package library

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/aydencharles/ps5-ftp-fnOS/src/backend/internal/domain"
	"github.com/aydencharles/ps5-ftp-fnOS/src/backend/internal/store"
)

type Library struct {
	store *store.Store
	roots map[string]domain.LibraryRoot
}

var volumeRE = regexp.MustCompile(`^/vol([0-9]+)$`)

var gameImageExtensions = map[string]struct{}{
	".exfat":  {},
	".ffpfs":  {},
	".ffpfsc": {},
	".phu":    {},
}

func New(ctx context.Context, s *store.Store) (*Library, error) {
	l := &Library{store: s, roots: map[string]domain.LibraryRoot{}}
	if err := l.Discover(ctx); err != nil {
		return nil, err
	}
	return l, nil
}

// NewStatic builds a Library from explicitly supplied roots. It is used by
// tests and by diagnostics that must not inspect host mount metadata.
func NewStatic(s *store.Store, roots []domain.LibraryRoot) *Library {
	l := &Library{store: s, roots: make(map[string]domain.LibraryRoot, len(roots))}
	for _, root := range roots {
		l.roots[root.ID] = root
	}
	return l
}
func rootID(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:8])
}

func (l *Library) Discover(ctx context.Context) error {
	seen := map[string]bool{}
	if f, e := os.Open("/proc/self/mountinfo"); e == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) > 4 && volumeRE.MatchString(fields[4]) {
				seen[fields[4]] = true
			}
		}
		f.Close()
	}
	matches, _ := filepath.Glob("/vol*")
	for _, p := range matches {
		if volumeRE.MatchString(p) {
			if st, e := os.Stat(p); e == nil && st.IsDir() {
				seen[p] = true
			}
		}
	}
	if len(seen) == 0 {
		wd, _ := os.Getwd()
		seen[wd] = true
	}
	for volume := range seen {
		volLabel := "本机目录"
		if m := volumeRE.FindStringSubmatch(volume); len(m) == 2 {
			volLabel = "存储空间 " + m[1]
		}
		candidates := []string{volume}
		entries, _ := os.ReadDir(volume)
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			p := filepath.Join(volume, e.Name())
			if _, err := fmt.Sscanf(e.Name(), "%d", new(int)); err == nil {
				children, _ := os.ReadDir(p)
				for _, c := range children {
					if c.IsDir() && !strings.HasPrefix(c.Name(), ".") {
						candidates = append(candidates, filepath.Join(p, c.Name()))
					}
				}
			} else {
				candidates = append(candidates, p)
			}
		}
		for _, p := range candidates {
			label := volLabel
			kind := "volume"
			if p != volume {
				label += " / " + filepath.Base(p)
				kind = "share"
			}
			r := domain.LibraryRoot{ID: rootID(p), Label: label, Path: p, Kind: kind}
			if err := l.store.UpsertRoot(ctx, r); err != nil {
				return err
			}
		}
	}
	roots, err := l.store.Roots(ctx)
	if err != nil {
		return err
	}
	l.roots = map[string]domain.LibraryRoot{}
	for _, r := range roots {
		l.roots[r.ID] = r
	}
	return nil
}
func (l *Library) Roots() []domain.LibraryRoot {
	out := make([]domain.LibraryRoot, 0, len(l.roots))
	for _, r := range l.roots {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

func cleanRelative(p string) (string, error) {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "/")
	clean := filepath.Clean(filepath.FromSlash(p))
	if clean == "." {
		return "", nil
	}
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes library root")
	}
	return clean, nil
}
func (l *Library) Resolve(rootID, relative string) (string, error) {
	r, ok := l.roots[rootID]
	if !ok {
		return "", errors.New("unknown library root")
	}
	rel, err := cleanRelative(relative)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(r.Path, rel)
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	rootResolved, err := filepath.EvalSymlinks(r.Path)
	if err != nil {
		return "", err
	}
	within, err := filepath.Rel(rootResolved, resolved)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes library root")
	}
	return resolved, nil
}

// ResolveForWrite resolves a locator that may not exist yet. It checks the
// closest existing ancestor after following symlinks, so a task can create a
// new file or directory without gaining a route outside its Library Root.
func (l *Library) ResolveForWrite(rootID, relative string) (string, error) {
	r, ok := l.roots[rootID]
	if !ok {
		return "", errors.New("unknown library root")
	}
	rel, err := cleanRelative(relative)
	if err != nil {
		return "", err
	}
	rootResolved, err := filepath.EvalSymlinks(r.Path)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(rootResolved, rel)
	probe := candidate
	for {
		if _, err = os.Lstat(probe); err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		next := filepath.Dir(probe)
		if next == probe {
			return "", errors.New("path escapes library root")
		}
		probe = next
	}
	resolvedProbe, err := filepath.EvalSymlinks(probe)
	if err != nil {
		return "", err
	}
	within, err := filepath.Rel(rootResolved, resolvedProbe)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes library root")
	}
	if info, statErr := os.Lstat(candidate); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		resolved, resolveErr := filepath.EvalSymlinks(candidate)
		if resolveErr != nil {
			return "", resolveErr
		}
		within, resolveErr = filepath.Rel(rootResolved, resolved)
		if resolveErr != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
			return "", errors.New("path escapes library root")
		}
	}
	return candidate, nil
}

func (l *Library) Entries(rootID, relative, query string, hidden bool) ([]domain.Entry, error) {
	abs, err := l.Resolve(rootID, relative)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(query)
	out := make([]domain.Entry, 0, len(entries))
	appendEntry := func(e os.DirEntry, entryAbs, entryRel string) {
		name := e.Name()
		if !hidden && strings.HasPrefix(name, ".") {
			return
		}
		if query != "" && !strings.Contains(strings.ToLower(name), query) {
			return
		}
		info, err := e.Info()
		if err != nil {
			return
		}
		item := domain.Entry{Name: name, Path: filepath.ToSlash(entryRel), IsDir: e.IsDir(), Size: info.Size(), ModifiedAt: info.ModTime()}
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(entryAbs, "eboot.bin")); err == nil {
				item.GameKind = "game-directory"
			}
		} else {
			ext := strings.ToLower(filepath.Ext(name))
			if _, ok := gameImageExtensions[ext]; ok {
				item.GameKind = "game-image"
			}
		}
		out = append(out, item)
	}

	root := l.roots[rootID]
	for _, e := range entries {
		// fnOS stores user shares below numeric UID directories. At the storage
		// root, expose those shares directly so the Interface never makes users
		// navigate through implementation details such as /vol2/1000.
		if relative == "" && root.Kind == "volume" && e.IsDir() {
			if _, err := fmt.Sscanf(e.Name(), "%d", new(int)); err == nil {
				uidPath := filepath.Join(abs, e.Name())
				children, readErr := os.ReadDir(uidPath)
				if readErr != nil {
					continue
				}
				for _, child := range children {
					childAbs := filepath.Join(uidPath, child.Name())
					childRel := filepath.Join(e.Name(), child.Name())
					appendEntry(child, childAbs, childRel)
				}
				continue
			}
		}
		entryAbs := filepath.Join(abs, e.Name())
		entryRel := filepath.Join(filepath.FromSlash(relative), e.Name())
		appendEntry(e, entryAbs, entryRel)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}
