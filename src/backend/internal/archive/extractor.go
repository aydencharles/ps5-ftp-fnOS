package archive

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/bodgit/sevenzip"
)

type Entry struct {
	File *sevenzip.File
	Path string
	Mode fs.FileMode
	Size int64
}

type Volume struct {
	Path string
	Info os.FileInfo
}

type Plan struct {
	Reader     *sevenzip.ReadCloser
	Entries    []Entry
	Volumes    []Volume
	TotalBytes int64
}

type Progress struct {
	Bytes       int64
	Completed   int
	CurrentFile string
}

func SupportedName(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".7z") || strings.HasSuffix(lower, ".7z.001")
}

func DestinationName(name string) (string, error) {
	lower := strings.ToLower(name)
	cut := len(name)
	switch {
	case strings.HasSuffix(lower, ".7z.001"):
		cut -= len(".7z.001")
	case strings.HasSuffix(lower, ".7z"):
		cut -= len(".7z")
	default:
		return "", errors.New("只支持 .7z 或首卷 .7z.001")
	}
	name = strings.TrimSpace(name[:cut])
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\\\x00`) {
		return "", errors.New("无法从压缩包名称生成目标文件夹")
	}
	return name, nil
}

func StageName(taskID string) string {
	return ".ps5-ftp-manager-extract-" + taskID
}

func normalizeEntryName(raw string) (string, error) {
	if strings.ContainsRune(raw, 0) {
		return "", errors.New("文件名包含 NUL")
	}
	normalized := strings.ReplaceAll(raw, `\`, "/")
	normalized = strings.TrimSuffix(normalized, "/")
	if normalized == "" || normalized == "." || strings.HasPrefix(normalized, "/") || strings.HasPrefix(normalized, "//") {
		return "", fmt.Errorf("非法归档路径 %q", raw)
	}
	if len(normalized) >= 2 && normalized[1] == ':' {
		return "", fmt.Errorf("归档包含绝对路径 %q", raw)
	}
	cleaned := path.Clean(normalized)
	if cleaned != normalized || !fs.ValidPath(cleaned) {
		return "", fmt.Errorf("归档路径越界或不规范 %q", raw)
	}
	return cleaned, nil
}

func Open(source, password string) (*Plan, error) {
	reader, err := sevenzip.OpenReaderWithPassword(source, password)
	if err != nil {
		return nil, err
	}
	plan := &Plan{Reader: reader}
	defer func() {
		if err != nil {
			_ = reader.Close()
		}
	}()
	seen := make(map[string]struct{}, len(reader.File))
	for _, file := range reader.File {
		entryPath, pathErr := normalizeEntryName(file.Name)
		if pathErr != nil {
			err = pathErr
			return nil, err
		}
		if _, exists := seen[entryPath]; exists {
			err = fmt.Errorf("归档包含重复路径 %q", entryPath)
			return nil, err
		}
		seen[entryPath] = struct{}{}
		mode := file.Mode()
		if !mode.IsDir() && mode.Type() != 0 {
			err = fmt.Errorf("归档包含不支持的特殊文件 %q", entryPath)
			return nil, err
		}
		if file.UncompressedSize > math.MaxInt64 {
			err = fmt.Errorf("文件过大 %q", entryPath)
			return nil, err
		}
		size := int64(file.UncompressedSize)
		if !mode.IsDir() {
			if plan.TotalBytes > math.MaxInt64-size {
				err = errors.New("归档解压总大小超过系统限制")
				return nil, err
			}
			plan.TotalBytes += size
		}
		plan.Entries = append(plan.Entries, Entry{File: file, Path: entryPath, Mode: mode, Size: size})
	}
	for _, volumePath := range reader.Volumes() {
		info, statErr := os.Stat(volumePath)
		if statErr != nil {
			err = statErr
			return nil, err
		}
		plan.Volumes = append(plan.Volumes, Volume{Path: volumePath, Info: info})
	}
	return plan, nil
}

func (p *Plan) Close() error {
	if p == nil || p.Reader == nil {
		return nil
	}
	return p.Reader.Close()
}

func CheckSpace(directory string, required int64) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(directory, &stat); err != nil {
		return err
	}
	blocks, blockSize := uint64(stat.Bavail), uint64(stat.Bsize)
	available := int64(math.MaxInt64)
	if blockSize == 0 || blocks <= uint64(math.MaxInt64)/blockSize {
		available = int64(blocks * blockSize)
	}
	if required > available {
		return fmt.Errorf("目标存储空间不足：需要 %d 字节，可用 %d 字节", required, available)
	}
	return nil
}

type progressWriter struct {
	ctx      context.Context
	w        io.Writer
	written  *int64
	callback func(Progress)
	progress Progress
}

func (w *progressWriter) Write(data []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := w.w.Write(data)
	*w.written += int64(n)
	w.progress.Bytes = *w.written
	w.callback(w.progress)
	return n, err
}

func safeFileMode(mode fs.FileMode) fs.FileMode {
	return 0o644 | (mode.Perm() & 0o111)
}

func Extract(ctx context.Context, plan *Plan, stage string, callback func(Progress)) error {
	if callback == nil {
		callback = func(Progress) {}
	}
	if err := os.Mkdir(stage, 0o700); err != nil {
		return err
	}
	var written int64
	completed := 0
	var directories []Entry
	for _, entry := range plan.Entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		target := filepath.Join(stage, filepath.FromSlash(entry.Path))
		relative, err := filepath.Rel(stage, target)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("归档路径越过临时目录 %q", entry.Path)
		}
		if entry.Mode.IsDir() {
			if err = os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			directories = append(directories, entry)
			completed++
			callback(Progress{Bytes: written, Completed: completed, CurrentFile: entry.Path})
			continue
		}
		if err = os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		input, err := entry.File.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, safeFileMode(entry.Mode))
		if err != nil {
			_ = input.Close()
			return err
		}
		hash := crc32.NewIEEE()
		writer := &progressWriter{ctx: ctx, w: io.MultiWriter(output, hash), written: &written, callback: callback, progress: Progress{Completed: completed, CurrentFile: entry.Path}}
		copied, copyErr := io.CopyBuffer(writer, input, make([]byte, 256*1024))
		closeInputErr := input.Close()
		closeOutputErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeInputErr != nil {
			return closeInputErr
		}
		if closeOutputErr != nil {
			return closeOutputErr
		}
		if copied != entry.Size {
			return fmt.Errorf("解压大小校验失败 %s：%d/%d", entry.Path, copied, entry.Size)
		}
		if hash.Sum32() != entry.File.CRC32 {
			return fmt.Errorf("CRC 校验失败 %s", entry.Path)
		}
		if !entry.File.Modified.IsZero() {
			_ = os.Chtimes(target, entry.File.Modified, entry.File.Modified)
		}
		completed++
		callback(Progress{Bytes: written, Completed: completed, CurrentFile: entry.Path})
	}
	for index := len(directories) - 1; index >= 0; index-- {
		target := filepath.Join(stage, filepath.FromSlash(directories[index].Path))
		_ = os.Chmod(target, 0o755)
		if !directories[index].File.Modified.IsZero() {
			_ = os.Chtimes(target, directories[index].File.Modified, directories[index].File.Modified)
		}
	}
	return nil
}

func DeleteVolumes(volumes []Volume) []string {
	var failed []string
	for _, volume := range volumes {
		current, err := os.Stat(volume.Path)
		if err != nil || !os.SameFile(volume.Info, current) || current.Size() != volume.Info.Size() || !current.ModTime().Equal(volume.Info.ModTime()) {
			failed = append(failed, volume.Path)
			continue
		}
		if err = os.Remove(volume.Path); err != nil {
			failed = append(failed, volume.Path)
		}
	}
	return failed
}

func IsEncryptedError(err error) bool {
	var readErr *sevenzip.ReadError
	return errors.As(err, &readErr) && readErr.Encrypted
}

func CleanupStage(stage string) error {
	if filepath.Base(stage) != StageName(strings.TrimPrefix(filepath.Base(stage), ".ps5-ftp-manager-extract-")) {
		return errors.New("refusing to remove unexpected extraction path")
	}
	if err := os.RemoveAll(stage); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
