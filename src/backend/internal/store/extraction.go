package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/aydencharles/ps5-ftp-fnOS/src/backend/internal/domain"
)

const extractionColumns = `id,source_root_id,source_path,destination_root_id,destination_parent,destination_path,delete_sources,state,total_bytes,extracted_bytes,speed_bytes,eta_seconds,current_file,total_items,completed_items,error,warning,retry_of,created_at,started_at,finished_at`

func scanExtractionTask(row interface{ Scan(...any) error }) (domain.ExtractionTask, error) {
	var task domain.ExtractionTask
	var created string
	var started, finished sql.NullString
	var eta sql.NullInt64
	err := row.Scan(
		&task.ID, &task.Source.RootID, &task.Source.Path,
		&task.Destination.RootID, &task.DestinationParent.Path, &task.Destination.Path,
		&task.DeleteSources, &task.State, &task.TotalBytes, &task.ExtractedBytes,
		&task.SpeedBytes, &eta, &task.CurrentFile, &task.TotalItems, &task.CompletedItems,
		&task.Error, &task.Warning, &task.RetryOf, &created, &started, &finished,
	)
	if err != nil {
		return task, err
	}
	task.DestinationParent.RootID = task.Destination.RootID
	task.CreatedAt = parseTime(created)
	task.StartedAt = nullableTime(started)
	task.FinishedAt = nullableTime(finished)
	if eta.Valid {
		value := eta.Int64
		task.ETASeconds = &value
	}
	return task, nil
}

func (s *Store) CreateExtractionTask(ctx context.Context, task domain.ExtractionTask) (domain.ExtractionTask, error) {
	if task.ID == "" {
		task.ID = newID()
	}
	if task.State == "" {
		task.State = domain.ExtractionQueued
	}
	ciphertext := ""
	var err error
	if task.Password != "" {
		ciphertext, err = s.encrypt(task.Password)
		if err != nil {
			return task, err
		}
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO extraction_tasks(
id,source_root_id,source_path,destination_root_id,destination_parent,destination_path,delete_sources,password_cipher,state,retry_of,created_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?)`, task.ID, task.Source.RootID, task.Source.Path, task.Destination.RootID,
		task.DestinationParent.Path, task.Destination.Path, task.DeleteSources, ciphertext, task.State, task.RetryOf, now())
	if err != nil {
		return task, err
	}
	return s.ExtractionTask(ctx, task.ID, false)
}

func (s *Store) ExtractionTask(ctx context.Context, id string, includeSecret bool) (domain.ExtractionTask, error) {
	task, err := scanExtractionTask(s.db.QueryRowContext(ctx, "SELECT "+extractionColumns+" FROM extraction_tasks WHERE id=?", id))
	if err != nil || !includeSecret {
		return task, err
	}
	var ciphertext string
	if err = s.db.QueryRowContext(ctx, "SELECT password_cipher FROM extraction_tasks WHERE id=?", id).Scan(&ciphertext); err != nil {
		return task, err
	}
	if ciphertext != "" {
		task.Password, err = s.decrypt(ciphertext)
	}
	return task, err
}

func (s *Store) ExtractionTasks(ctx context.Context) ([]domain.ExtractionTask, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+extractionColumns+" FROM extraction_tasks ORDER BY created_at DESC LIMIT 200")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := make([]domain.ExtractionTask, 0)
	for rows.Next() {
		task, scanErr := scanExtractionTask(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *Store) QueuedExtractionTasks(ctx context.Context) ([]domain.ExtractionTask, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+extractionColumns+" FROM extraction_tasks WHERE state=? ORDER BY created_at", domain.ExtractionQueued)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []domain.ExtractionTask
	for rows.Next() {
		task, scanErr := scanExtractionTask(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *Store) ClaimExtraction(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE extraction_tasks SET state=?,started_at=COALESCE(started_at,?) WHERE id=? AND state=?`, domain.ExtractionScanning, now(), id, domain.ExtractionQueued)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

func (s *Store) BeginExtraction(ctx context.Context, id string, totalItems int, totalBytes int64) (bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE extraction_tasks SET state=?,total_items=?,total_bytes=? WHERE id=? AND state=?`, domain.ExtractionExtracting, totalItems, totalBytes, id, domain.ExtractionScanning)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

func (s *Store) BeginExtractionCommit(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE extraction_tasks SET state=?,speed_bytes=0,eta_seconds=NULL,current_file='' WHERE id=? AND state=?`, domain.ExtractionCleaning, id, domain.ExtractionExtracting)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

func (s *Store) SetExtractionState(ctx context.Context, id, state, errMsg, warning string) error {
	var started, finished any
	if state == domain.ExtractionScanning {
		started = now()
	}
	if domain.ExtractionTerminal(state) {
		finished = now()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE extraction_tasks SET state=?,error=?,warning=?,
started_at=COALESCE(started_at,?),finished_at=COALESCE(?,finished_at),
password_cipher=CASE WHEN ? THEN '' ELSE password_cipher END WHERE id=?`,
		state, errMsg, warning, started, finished, domain.ExtractionTerminal(state), id)
	return err
}

func (s *Store) UpdateExtractionProgress(ctx context.Context, id string, bytes int64, completed int, speed float64, eta *int64, current string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE extraction_tasks SET extracted_bytes=?,completed_items=?,speed_bytes=?,eta_seconds=?,current_file=? WHERE id=?`, bytes, completed, speed, eta, current, id)
	return err
}

func (s *Store) CancelQueuedExtraction(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE extraction_tasks SET state=?,finished_at=?,password_cipher='' WHERE id=? AND state=?`, domain.ExtractionCanceled, now(), id, domain.ExtractionQueued)
	if err != nil {
		return false, err
	}
	count, _ := result.RowsAffected()
	return count > 0, nil
}

func (s *Store) MarkExtractionCanceling(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE extraction_tasks SET state=? WHERE id=? AND state IN (?,?)", domain.ExtractionCanceling, id, domain.ExtractionScanning, domain.ExtractionExtracting)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		var state string
		if err = s.db.QueryRowContext(ctx, "SELECT state FROM extraction_tasks WHERE id=?", id).Scan(&state); err != nil {
			return err
		}
		return fmt.Errorf("%w: extraction task in state %s cannot be canceled", ErrStateConflict, state)
	}
	return nil
}

func (s *Store) ExtractionIsCanceling(ctx context.Context, id string) bool {
	var state string
	return s.db.QueryRowContext(ctx, "SELECT state FROM extraction_tasks WHERE id=?", id).Scan(&state) == nil && state == domain.ExtractionCanceling
}

func (s *Store) DeleteExtractionTask(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM extraction_tasks WHERE id=? AND state IN (?,?,?,?)", id, domain.ExtractionSucceeded, domain.ExtractionFailed, domain.ExtractionCanceled, domain.ExtractionInterrupted)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		var state string
		if err = s.db.QueryRowContext(ctx, "SELECT state FROM extraction_tasks WHERE id=?", id).Scan(&state); err != nil {
			return err
		}
		return fmt.Errorf("%w: extraction task in state %s is not a history record", ErrStateConflict, state)
	}
	return nil
}

func (s *Store) InterruptExtractions(ctx context.Context) ([]domain.ExtractionTask, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+extractionColumns+" FROM extraction_tasks WHERE state IN (?,?,?,?)", domain.ExtractionScanning, domain.ExtractionExtracting, domain.ExtractionCleaning, domain.ExtractionCanceling)
	if err != nil {
		return nil, err
	}
	var tasks []domain.ExtractionTask
	for rows.Next() {
		task, scanErr := scanExtractionTask(rows)
		if scanErr != nil {
			rows.Close()
			return nil, scanErr
		}
		tasks = append(tasks, task)
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE extraction_tasks SET state=?,error='服务重启中断了解压任务',password_cipher='',finished_at=? WHERE state IN (?,?,?,?)`, domain.ExtractionInterrupted, now(), domain.ExtractionScanning, domain.ExtractionExtracting, domain.ExtractionCleaning, domain.ExtractionCanceling)
	return tasks, err
}

func (s *Store) ExtractionTaskCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM extraction_tasks").Scan(&count)
	return count, err
}
