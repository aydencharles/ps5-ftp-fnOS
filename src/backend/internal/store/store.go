package store

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/aydencharles/ps5-ftp-fnOS/src/backend/internal/domain"
)

var (
	ErrProfileNotFound = errors.New("profile not found")
	ErrStateConflict   = errors.New("resource state conflict")
)

type Store struct {
	db   *sql.DB
	aead cipher.AEAD
}

func Open(dbPath, keyPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000"} {
		if _, err = db.Exec(pragma); err != nil {
			db.Close()
			return nil, err
		}
	}
	key, err := loadKey(keyPath)
	if err != nil {
		db.Close()
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		db.Close()
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db, aead: aead}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func loadKey(path string) ([]byte, error) {
	key, err := os.ReadFile(path)
	if err == nil && len(key) == 32 {
		return key, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key = make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS schema_version(version INTEGER NOT NULL);
INSERT INTO schema_version(version) SELECT 1 WHERE NOT EXISTS(SELECT 1 FROM schema_version);
CREATE TABLE IF NOT EXISTS profiles(
 id TEXT PRIMARY KEY, name TEXT NOT NULL, host TEXT NOT NULL, port INTEGER NOT NULL,
 username TEXT NOT NULL, password_cipher TEXT NOT NULL, base_path TEXT NOT NULL, preset TEXT NOT NULL,
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS library_roots(
 id TEXT PRIMARY KEY, label TEXT NOT NULL, path TEXT NOT NULL UNIQUE,
 favorite INTEGER NOT NULL DEFAULT 0, kind TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS tasks(
 id TEXT PRIMARY KEY, type TEXT NOT NULL, profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
 sources_json TEXT NOT NULL, destination TEXT NOT NULL, conflict_policy TEXT NOT NULL, state TEXT NOT NULL,
 total_bytes INTEGER NOT NULL DEFAULT 0, transferred_bytes INTEGER NOT NULL DEFAULT 0,
 speed_bytes REAL NOT NULL DEFAULT 0, eta_seconds INTEGER, current_file TEXT NOT NULL DEFAULT '',
 total_items INTEGER NOT NULL DEFAULT 0, completed_items INTEGER NOT NULL DEFAULT 0,
 skipped_items INTEGER NOT NULL DEFAULT 0, error TEXT NOT NULL DEFAULT '', retry_of TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL, started_at TEXT, finished_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_tasks_profile_state ON tasks(profile_id,state,created_at);
CREATE TABLE IF NOT EXISTS task_items(
 id TEXT PRIMARY KEY, task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
 root_id TEXT NOT NULL, source_path TEXT NOT NULL, destination TEXT NOT NULL,
 is_dir INTEGER NOT NULL, size INTEGER NOT NULL, mod_unix_nano INTEGER NOT NULL,
 state TEXT NOT NULL, transferred INTEGER NOT NULL DEFAULT 0, attempts INTEGER NOT NULL DEFAULT 0,
 error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_task_items_task ON task_items(task_id,state);
CREATE TABLE IF NOT EXISTS task_events(
 id INTEGER PRIMARY KEY AUTOINCREMENT, task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
 level TEXT NOT NULL, message TEXT NOT NULL, created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS settings(key TEXT PRIMARY KEY, value TEXT NOT NULL);
INSERT OR IGNORE INTO settings(key,value) VALUES('transfer_workers','2');`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	const extractionSchema = `
CREATE TABLE IF NOT EXISTS extraction_tasks(
 id TEXT PRIMARY KEY,
 source_root_id TEXT NOT NULL, source_path TEXT NOT NULL,
 destination_root_id TEXT NOT NULL, destination_parent TEXT NOT NULL, destination_path TEXT NOT NULL,
 delete_sources INTEGER NOT NULL DEFAULT 0, password_cipher TEXT NOT NULL DEFAULT '',
 state TEXT NOT NULL, total_bytes INTEGER NOT NULL DEFAULT 0, extracted_bytes INTEGER NOT NULL DEFAULT 0,
 speed_bytes REAL NOT NULL DEFAULT 0, eta_seconds INTEGER, current_file TEXT NOT NULL DEFAULT '',
 total_items INTEGER NOT NULL DEFAULT 0, completed_items INTEGER NOT NULL DEFAULT 0,
 error TEXT NOT NULL DEFAULT '', warning TEXT NOT NULL DEFAULT '', retry_of TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL, started_at TEXT, finished_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_extraction_tasks_state_created ON extraction_tasks(state,created_at);
UPDATE schema_version SET version=2 WHERE version<2;`
	_, err := s.db.Exec(extractionSchema)
	return err
}

func (s *Store) Close() error                   { return s.db.Close() }
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func now() string                  { return time.Now().UTC().Format(time.RFC3339Nano) }
func parseTime(v string) time.Time { t, _ := time.Parse(time.RFC3339Nano, v); return t }
func nullableTime(v sql.NullString) *time.Time {
	if !v.Valid {
		return nil
	}
	t := parseTime(v.String)
	return &t
}

func (s *Store) encrypt(plain string) (string, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(s.aead.Seal(nonce, nonce, []byte(plain), nil)), nil
}
func (s *Store) decrypt(encoded string) (string, error) {
	data, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	n := s.aead.NonceSize()
	if len(data) < n {
		return "", errors.New("invalid encrypted password")
	}
	plain, err := s.aead.Open(nil, data[:n], data[n:], nil)
	return string(plain), err
}

func (s *Store) SaveProfile(ctx context.Context, p domain.Profile) (domain.Profile, error) {
	creating := p.ID == ""
	if creating {
		p.ID = newID()
	}
	if p.BasePath == "" {
		p.BasePath = "/"
	}
	var existingCipher string
	if !creating {
		err := s.db.QueryRowContext(ctx, "SELECT password_cipher FROM profiles WHERE id=?", p.ID).Scan(&existingCipher)
		if errors.Is(err, sql.ErrNoRows) {
			return p, ErrProfileNotFound
		}
		if err != nil {
			return p, err
		}
	}
	ciphertext := existingCipher
	if p.Password != "" || ciphertext == "" {
		var err error
		ciphertext, err = s.encrypt(p.Password)
		if err != nil {
			return p, err
		}
	}
	t := now()
	if creating {
		_, err := s.db.ExecContext(ctx, `INSERT INTO profiles(id,name,host,port,username,password_cipher,base_path,preset,created_at,updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?)`, p.ID, p.Name, p.Host, p.Port, p.Username, ciphertext, p.BasePath, p.Preset, t, t)
		if err != nil {
			return p, err
		}
		return s.Profile(ctx, p.ID, false)
	}
	result, err := s.db.ExecContext(ctx, `UPDATE profiles SET name=?,host=?,port=?,username=?,password_cipher=?,base_path=?,preset=?,updated_at=? WHERE id=?`,
		p.Name, p.Host, p.Port, p.Username, ciphertext, p.BasePath, p.Preset, t, p.ID)
	if err != nil {
		return p, err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return p, err
	}
	if updated == 0 {
		return p, ErrProfileNotFound
	}
	return s.Profile(ctx, p.ID, false)
}

func scanProfile(row interface{ Scan(...any) error }, includeSecret bool, s *Store) (domain.Profile, error) {
	var p domain.Profile
	var cipherText, created, updated string
	err := row.Scan(&p.ID, &p.Name, &p.Host, &p.Port, &p.Username, &cipherText, &p.BasePath, &p.Preset, &created, &updated)
	if err != nil {
		return p, err
	}
	p.CreatedAt, p.UpdatedAt = parseTime(created), parseTime(updated)
	if includeSecret {
		p.Password, err = s.decrypt(cipherText)
	}
	return p, err
}

func (s *Store) Profile(ctx context.Context, id string, includeSecret bool) (domain.Profile, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id,name,host,port,username,password_cipher,base_path,preset,created_at,updated_at FROM profiles WHERE id=?`, id)
	p, err := scanProfile(row, includeSecret, s)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrProfileNotFound
	}
	return p, err
}

func (s *Store) Profiles(ctx context.Context) ([]domain.Profile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,host,port,username,password_cipher,base_path,preset,created_at,updated_at FROM profiles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Profile, 0)
	for rows.Next() {
		p, err := scanProfile(rows, false, s)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) DeleteProfile(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM profiles WHERE id=?", id)
	if err != nil {
		return err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if deleted == 0 {
		return ErrProfileNotFound
	}
	return nil
}

func (s *Store) UpsertRoot(ctx context.Context, r domain.LibraryRoot) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO library_roots(id,label,path,favorite,kind,updated_at) VALUES(?,?,?,?,?,?)
ON CONFLICT(path) DO UPDATE SET label=excluded.label,kind=excluded.kind,updated_at=excluded.updated_at`, r.ID, r.Label, r.Path, r.Favorite, r.Kind, now())
	return err
}

func (s *Store) Roots(ctx context.Context) ([]domain.LibraryRoot, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,label,path,favorite,kind FROM library_roots ORDER BY favorite DESC,label")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.LibraryRoot, 0)
	for rows.Next() {
		var r domain.LibraryRoot
		if err := rows.Scan(&r.ID, &r.Label, &r.Path, &r.Favorite, &r.Kind); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateTask(ctx context.Context, t domain.Task) (domain.Task, error) {
	if t.ID == "" {
		t.ID = newID()
	}
	if t.Type == "" {
		t.Type = "upload"
	}
	if t.State == "" {
		t.State = domain.TaskQueued
	}
	if t.ConflictPolicy == "" {
		t.ConflictPolicy = "smart"
	}
	b, _ := json.Marshal(t.Sources)
	created := now()
	_, err := s.db.ExecContext(ctx, `INSERT INTO tasks(id,type,profile_id,sources_json,destination,conflict_policy,state,retry_of,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, t.ID, t.Type, t.ProfileID, string(b), t.Destination, t.ConflictPolicy, t.State, t.RetryOf, created)
	if err != nil {
		return t, err
	}
	return s.Task(ctx, t.ID)
}

func scanTask(row interface{ Scan(...any) error }) (domain.Task, error) {
	var t domain.Task
	var src, created string
	var started, finished sql.NullString
	var eta sql.NullInt64
	err := row.Scan(&t.ID, &t.Type, &t.ProfileID, &src, &t.Destination, &t.ConflictPolicy, &t.State, &t.TotalBytes, &t.TransferredBytes, &t.SpeedBytes, &eta, &t.CurrentFile, &t.TotalItems, &t.CompletedItems, &t.SkippedItems, &t.Error, &t.RetryOf, &created, &started, &finished)
	if err != nil {
		return t, err
	}
	_ = json.Unmarshal([]byte(src), &t.Sources)
	t.CreatedAt = parseTime(created)
	t.StartedAt = nullableTime(started)
	t.FinishedAt = nullableTime(finished)
	if eta.Valid {
		v := eta.Int64
		t.ETASeconds = &v
	}
	return t, nil
}

const taskColumns = `id,type,profile_id,sources_json,destination,conflict_policy,state,total_bytes,transferred_bytes,speed_bytes,eta_seconds,current_file,total_items,completed_items,skipped_items,error,retry_of,created_at,started_at,finished_at`

func (s *Store) Task(ctx context.Context, id string) (domain.Task, error) {
	return scanTask(s.db.QueryRowContext(ctx, "SELECT "+taskColumns+" FROM tasks WHERE id=?", id))
}
func (s *Store) Tasks(ctx context.Context) ([]domain.Task, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+taskColumns+" FROM tasks ORDER BY created_at DESC LIMIT 200")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Task, 0)
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (s *Store) QueuedTasks(ctx context.Context) ([]domain.Task, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+taskColumns+" FROM tasks WHERE state=? ORDER BY created_at", domain.TaskQueued)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Task, 0)
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) InterruptInFlight(ctx context.Context) error {
	t := now()
	_, err := s.db.ExecContext(ctx, `UPDATE tasks SET state=?,error='服务重启中断了任务',finished_at=? WHERE state IN (?,?,?)`, domain.TaskInterrupted, t, domain.TaskScanning, domain.TaskRunning, domain.TaskCanceling)
	return err
}
func (s *Store) SetTaskState(ctx context.Context, id, state, errMsg string) error {
	var started, finished any
	if state == domain.TaskScanning {
		started = now()
	}
	if state == domain.TaskSucceeded || state == domain.TaskFailed || state == domain.TaskCanceled || state == domain.TaskInterrupted {
		finished = now()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE tasks SET state=?,error=?,started_at=COALESCE(started_at,?),finished_at=COALESCE(?,finished_at) WHERE id=?`, state, errMsg, started, finished, id)
	return err
}
func (s *Store) SetTaskPlan(ctx context.Context, id string, total int, totalBytes int64) error {
	_, e := s.db.ExecContext(ctx, "UPDATE tasks SET total_items=?,total_bytes=? WHERE id=?", total, totalBytes, id)
	return e
}
func (s *Store) UpdateTaskProgress(ctx context.Context, id string, bytes int64, speed float64, eta *int64, current string) error {
	_, e := s.db.ExecContext(ctx, "UPDATE tasks SET transferred_bytes=?,speed_bytes=?,eta_seconds=?,current_file=? WHERE id=?", bytes, speed, eta, current, id)
	return e
}
func (s *Store) IncrementTaskResult(ctx context.Context, id string, bytes int64, skipped bool) error {
	q := "UPDATE tasks SET transferred_bytes=transferred_bytes+?,completed_items=completed_items+1 WHERE id=?"
	if skipped {
		q = "UPDATE tasks SET transferred_bytes=transferred_bytes+?,completed_items=completed_items+1,skipped_items=skipped_items+1 WHERE id=?"
	}
	_, e := s.db.ExecContext(ctx, q, bytes, id)
	return e
}

func (s *Store) ReplaceItems(ctx context.Context, taskID string, items []domain.TaskItem) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "DELETE FROM task_items WHERE task_id=?", taskID); e != nil {
		return e
	}
	stmt, e := tx.PrepareContext(ctx, `INSERT INTO task_items(id,task_id,root_id,source_path,destination,is_dir,size,mod_unix_nano,state) VALUES(?,?,?,?,?,?,?,?,?)`)
	if e != nil {
		return e
	}
	defer stmt.Close()
	for i := range items {
		if items[i].ID == "" {
			items[i].ID = newID()
		}
		if _, e = stmt.ExecContext(ctx, items[i].ID, taskID, items[i].RootID, items[i].SourcePath, items[i].Destination, items[i].IsDir, items[i].Size, items[i].ModUnixNano, "pending"); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (s *Store) Items(ctx context.Context, taskID string) ([]domain.TaskItem, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT id,task_id,root_id,source_path,destination,is_dir,size,mod_unix_nano,state,transferred,attempts,error FROM task_items WHERE task_id=? ORDER BY is_dir DESC,destination`, taskID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]domain.TaskItem, 0)
	for rows.Next() {
		var i domain.TaskItem
		if e = rows.Scan(&i.ID, &i.TaskID, &i.RootID, &i.SourcePath, &i.Destination, &i.IsDir, &i.Size, &i.ModUnixNano, &i.State, &i.Transferred, &i.Attempts, &i.Error); e != nil {
			return nil, e
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
func (s *Store) SetItem(ctx context.Context, id, state string, transferred int64, attempts int, errMsg string) error {
	_, e := s.db.ExecContext(ctx, "UPDATE task_items SET state=?,transferred=?,attempts=?,error=? WHERE id=?", state, transferred, attempts, errMsg, id)
	return e
}
func (s *Store) Event(ctx context.Context, taskID, level, message string) error {
	_, e := s.db.ExecContext(ctx, "INSERT INTO task_events(task_id,level,message,created_at) VALUES(?,?,?,?)", taskID, level, message, now())
	return e
}
func (s *Store) Events(ctx context.Context, taskID string) ([]map[string]any, error) {
	rows, e := s.db.QueryContext(ctx, "SELECT id,level,message,created_at FROM task_events WHERE task_id=? ORDER BY id DESC LIMIT 500", taskID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var l, m, t string
		if e = rows.Scan(&id, &l, &m, &t); e != nil {
			return nil, e
		}
		out = append(out, map[string]any{"id": id, "level": l, "message": m, "created_at": t})
	}
	return out, rows.Err()
}

func (s *Store) Workers(ctx context.Context) int {
	var v string
	if s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='transfer_workers'").Scan(&v) != nil {
		return 2
	}
	if v < "1" || v > "4" {
		return 2
	}
	return int(v[0] - '0')
}
func (s *Store) SetWorkers(ctx context.Context, n int) error {
	if n < 1 || n > 4 {
		return errors.New("workers must be 1-4")
	}
	_, e := s.db.ExecContext(ctx, "INSERT INTO settings(key,value) VALUES('transfer_workers',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", fmt.Sprint(n))
	return e
}

func (s *Store) CancelQueued(ctx context.Context, id string) (bool, error) {
	res, e := s.db.ExecContext(ctx, "UPDATE tasks SET state=?,finished_at=? WHERE id=? AND state=?", domain.TaskCanceled, now(), id, domain.TaskQueued)
	if e != nil {
		return false, e
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
func (s *Store) IsCanceling(ctx context.Context, id string) bool {
	var state string
	_ = s.db.QueryRowContext(ctx, "SELECT state FROM tasks WHERE id=?", id).Scan(&state)
	return state == domain.TaskCanceling
}
func (s *Store) MarkCanceling(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE tasks SET state=? WHERE id=? AND state IN (?,?)", domain.TaskCanceling, id, domain.TaskScanning, domain.TaskRunning)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil || updated > 0 {
		return err
	}
	var state string
	if err = s.db.QueryRowContext(ctx, "SELECT state FROM tasks WHERE id=?", id).Scan(&state); err != nil {
		return err
	}
	return fmt.Errorf("%w: task in state %s cannot be canceled", ErrStateConflict, state)
}
func (s *Store) DeleteTask(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM tasks WHERE id=? AND state IN (?,?,?,?)", id, domain.TaskSucceeded, domain.TaskFailed, domain.TaskCanceled, domain.TaskInterrupted)
	if err != nil {
		return err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if deleted > 0 {
		return nil
	}
	var state string
	if err = s.db.QueryRowContext(ctx, "SELECT state FROM tasks WHERE id=?", id).Scan(&state); err != nil {
		return err
	}
	return fmt.Errorf("%w: task in state %s is not a history record", ErrStateConflict, state)
}

func NormalizeRemotePath(v string) (string, error) {
	v = strings.ReplaceAll(v, "\\", "/")
	if v == "" {
		return "/", nil
	}
	parts := strings.Split(v, "/")
	clean := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" || p == "." {
			continue
		}
		if p == ".." {
			return "", errors.New("remote path traversal is not allowed")
		}
		clean = append(clean, p)
	}
	return "/" + strings.Join(clean, "/"), nil
}
