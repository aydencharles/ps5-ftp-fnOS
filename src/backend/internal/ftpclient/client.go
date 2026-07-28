package ftpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	ftp "github.com/jlaffaye/ftp"

	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/domain"
	"github.com/chenpy/ps5-ftp-fnos/src/backend/internal/store"
)

type Client struct {
	mu   sync.Mutex
	conn *ftp.ServerConn
}

func Dial(ctx context.Context, p domain.Profile) (*Client, error) {
	addr := net.JoinHostPort(p.Host, fmt.Sprint(p.Port))
	conn, err := ftp.Dial(addr, ftp.DialWithTimeout(12*time.Second), ftp.DialWithContext(ctx), ftp.DialWithDisabledEPSV(false))
	if err != nil {
		return nil, err
	}
	user := p.Username
	if user == "" {
		user = "anonymous"
	}
	if err = conn.Login(user, p.Password); err != nil {
		_ = conn.Quit()
		return nil, err
	}
	return &Client{conn: conn}, nil
}
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	conn := c.conn
	c.conn = nil
	return conn.Quit()
}
func (c *Client) abort() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		_ = c.conn.Quit()
		c.conn = nil
	}
}
func (c *Client) withConn(fn func(*ftp.ServerConn) error) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return errors.New("FTP connection is closed")
	}
	return fn(conn)
}

func normalize(v string) (string, error) { return store.NormalizeRemotePath(v) }
func Join(base string, parts ...string) (string, error) {
	cleanBase, err := normalize(base)
	if err != nil {
		return "", err
	}
	for _, part := range parts {
		for _, segment := range strings.Split(strings.ReplaceAll(part, "\\", "/"), "/") {
			if segment == ".." {
				return "", errors.New("remote path traversal is not allowed")
			}
		}
	}
	return normalize(path.Join(append([]string{cleanBase}, parts...)...))
}

func (c *Client) List(p string) ([]domain.Entry, error) {
	p, err := normalize(p)
	if err != nil {
		return nil, err
	}
	var entries []*ftp.Entry
	err = c.withConn(func(conn *ftp.ServerConn) error { var e error; entries, e = conn.List(p); return e })
	if err != nil {
		return nil, err
	}
	out := make([]domain.Entry, 0, len(entries))
	for _, e := range entries {
		if e.Name == "." || e.Name == ".." {
			continue
		}
		child, _ := Join(p, e.Name)
		out = append(out, domain.Entry{Name: e.Name, Path: child, IsDir: e.Type == ftp.EntryTypeFolder, Size: int64(e.Size), ModifiedAt: e.Time})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}
func (c *Client) Size(p string) (int64, error) {
	p, err := normalize(p)
	if err != nil {
		return 0, err
	}
	var size int64
	err = c.withConn(func(conn *ftp.ServerConn) error { var e error; size, e = conn.FileSize(p); return e })
	return size, err
}
func (c *Client) Exists(p string) (bool, int64, error) {
	size, err := c.Size(p)
	if err == nil {
		return true, size, nil
	}
	var code *textproto.Error
	if errors.As(err, &code) && code.Code == ftp.StatusFileUnavailable {
		return false, 0, nil
	}
	return false, 0, err
}
func (c *Client) MakeDir(p string) error {
	p, err := normalize(p)
	if err != nil {
		return err
	}
	return c.withConn(func(conn *ftp.ServerConn) error { return conn.MakeDir(p) })
}
func (c *Client) MakeDirAll(p string) error {
	return c.MakeDirAllFrom("/", p)
}

// MakeDirAllFrom creates only the portion below an already-existing profile
// base. Some FTP servers expose a virtual root whose PWD is not "/" and reject
// probes of host-path ancestors above that root.
func (c *Client) MakeDirAllFrom(base, p string) error {
	base, err := normalize(base)
	if err != nil {
		return err
	}
	p, err = normalize(p)
	if err != nil {
		return err
	}
	if base != "/" && p != base && !strings.HasPrefix(p, base+"/") {
		return errors.New("directory is outside profile base path")
	}
	if p == base {
		return nil
	}
	relative := strings.TrimPrefix(strings.TrimPrefix(p, base), "/")
	cur := strings.TrimSuffix(base, "/")
	for _, part := range strings.Split(relative, "/") {
		cur += "/" + part
		err = c.MakeDir(cur)
		if err != nil {
			var code *textproto.Error
			if !(errors.As(err, &code) && code.Code == ftp.StatusFileUnavailable) {
				return err
			}
			if _, listErr := c.List(cur); listErr != nil {
				return err
			}
		}
	}
	return nil
}
func (c *Client) Rename(from, to string) error {
	from, err := normalize(from)
	if err != nil {
		return err
	}
	to, err = normalize(to)
	if err != nil {
		return err
	}
	return c.withConn(func(conn *ftp.ServerConn) error { return conn.Rename(from, to) })
}
func (c *Client) Delete(p string) error {
	p, err := normalize(p)
	if err != nil {
		return err
	}
	return c.withConn(func(conn *ftp.ServerConn) error { return conn.Delete(p) })
}
func (c *Client) RemoveDir(p string, recursive bool) error {
	p, err := normalize(p)
	if err != nil {
		return err
	}
	return c.withConn(func(conn *ftp.ServerConn) error {
		if recursive {
			return conn.RemoveDirRecur(p)
		}
		return conn.RemoveDir(p)
	})
}
func (c *Client) UploadFrom(remote string, r io.Reader, offset uint64) error {
	remote, err := normalize(remote)
	if err != nil {
		return err
	}
	return c.withConn(func(conn *ftp.ServerConn) error { return conn.StorFrom(remote, r, offset) })
}

func (c *Client) DownloadTo(remote string, w io.Writer, offset uint64) error {
	remote, err := normalize(remote)
	if err != nil {
		return err
	}
	return c.withConn(func(conn *ftp.ServerConn) error {
		response, err := conn.RetrFrom(remote, offset)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(w, response)
		closeErr := response.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}
func (c *Client) Test() map[string]any {
	result := map[string]any{"ok": true, "passive": true, "mlsd_or_list": false, "rest_stor": true}
	if _, err := c.List("/"); err != nil {
		result["ok"] = false
		result["error"] = err.Error()
	} else {
		result["mlsd_or_list"] = true
	}
	return result
}

type ContextReader struct {
	Context context.Context
	Reader  io.Reader
	OnRead  func(int)
}

type ContextWriter struct {
	Context context.Context
	Writer  io.Writer
	OnWrite func(int)
}

func (w *ContextWriter) Write(p []byte) (int, error) {
	select {
	case <-w.Context.Done():
		return 0, w.Context.Err()
	default:
	}
	n, err := w.Writer.Write(p)
	if n > 0 && w.OnWrite != nil {
		w.OnWrite(n)
	}
	return n, err
}

func (r *ContextReader) Read(p []byte) (int, error) {
	select {
	case <-r.Context.Done():
		return 0, r.Context.Err()
	default:
	}
	n, err := r.Reader.Read(p)
	if n > 0 && r.OnRead != nil {
		r.OnRead(n)
	}
	return n, err
}
