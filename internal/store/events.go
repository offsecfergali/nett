package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/alieddine/nett/internal/model"
)

// InsertEvent inserts e if an event with the same deterministic ID
// (hash(type+asset)) has not already been recorded for this project, and
// reports whether it was newly inserted. This is the loop-guard primitive the
// pivot engine (M21) builds on: a duplicate occurrence is dropped here,
// before it can schedule any work.
func (s *Store) InsertEvent(ctx context.Context, e *model.Event) (inserted bool, err error) {
	if e.ID == "" {
		e.ID = model.EventID(e.Type, e.AssetID)
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	if e.Status == "" {
		e.Status = "pending"
	}
	dataJSON, err := marshalJSON(e.Data)
	if err != nil {
		return false, fmt.Errorf("store: marshal event data: %w", err)
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO events (id, project_id, type, asset_id, source, data_json, depth, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, id) DO NOTHING`,
		e.ID, e.ProjectID, e.Type, e.AssetID, e.Source, dataJSON, e.Depth, e.Status, formatTime(e.CreatedAt))
	if err != nil {
		return false, fmt.Errorf("store: insert event %q: %w", e.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: insert event %q: %w", e.ID, err)
	}
	return n > 0, nil
}

// PendingEvents returns up to limit events with status "pending", oldest
// first.
func (s *Store) PendingEvents(ctx context.Context, projectID string, limit int) ([]*model.Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, type, asset_id, source, data_json, depth, status, created_at, processed_at
		FROM events WHERE project_id = ? AND status = 'pending'
		ORDER BY created_at ASC LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list pending events: %w", err)
	}
	defer rows.Close()

	var out []*model.Event
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// MarkEventStatus transitions an event to "processed" or "dropped" and
// records the processing time.
func (s *Store) MarkEventStatus(ctx context.Context, projectID, eventID, status string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE events SET status = ?, processed_at = ?
		WHERE project_id = ? AND id = ?`,
		status, formatTime(time.Now()), projectID, eventID)
	if err != nil {
		return fmt.Errorf("store: mark event %q %s: %w", eventID, status, err)
	}
	return nil
}

func scanEvent(row rowScanner) (*model.Event, error) {
	var e model.Event
	var dataJSON, createdAt, processedAt string
	if err := row.Scan(&e.ID, &e.ProjectID, &e.Type, &e.AssetID, &e.Source, &dataJSON, &e.Depth, &e.Status, &createdAt, &processedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("store: scan event: %w", err)
	}
	data, err := unmarshalJSON(dataJSON)
	if err != nil {
		return nil, fmt.Errorf("store: unmarshal event data: %w", err)
	}
	e.Data = data
	e.CreatedAt = parseTime(createdAt)
	e.ProcessedAt = parseTime(processedAt)
	return &e, nil
}

// UpsertModuleRun records or updates the execution state of one (module,
// target) pair, the primitive `nett resume` (M23) builds on.
func (s *Store) UpsertModuleRun(ctx context.Context, r *model.ModuleRun) error {
	cursorJSON, err := marshalJSON(r.Cursor)
	if err != nil {
		return fmt.Errorf("store: marshal module run cursor: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO module_runs (project_id, module, target, input_fp, cursor_json, status, error, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, module, target) DO UPDATE SET
			input_fp    = excluded.input_fp,
			cursor_json = excluded.cursor_json,
			status      = excluded.status,
			error       = excluded.error,
			started_at  = CASE WHEN module_runs.started_at = '' THEN excluded.started_at ELSE module_runs.started_at END,
			finished_at = excluded.finished_at`,
		r.ProjectID, r.Module, r.Target, r.InputFP, cursorJSON, r.Status, r.Error,
		formatTime(r.StartedAt), formatTime(r.FinishedAt))
	if err != nil {
		return fmt.Errorf("store: upsert module run %s/%s: %w", r.Module, r.Target, err)
	}
	return nil
}

// GetModuleRun returns the run record for (module, target) in project, or
// (nil, nil) if it has never run — the check `nett resume` uses to skip
// already-completed work.
func (s *Store) GetModuleRun(ctx context.Context, projectID, module, target string) (*model.ModuleRun, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, module, target, input_fp, cursor_json, status, error, started_at, finished_at
		FROM module_runs WHERE project_id = ? AND module = ? AND target = ?`, projectID, module, target)
	var r model.ModuleRun
	var cursorJSON, startedAt, finishedAt string
	if err := row.Scan(&r.ID, &r.ProjectID, &r.Module, &r.Target, &r.InputFP, &cursorJSON, &r.Status, &r.Error, &startedAt, &finishedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("store: get module run %s/%s: %w", module, target, err)
	}
	cursor, err := unmarshalJSON(cursorJSON)
	if err != nil {
		return nil, fmt.Errorf("store: unmarshal module run cursor: %w", err)
	}
	r.Cursor = cursor
	r.StartedAt = parseTime(startedAt)
	r.FinishedAt = parseTime(finishedAt)
	return &r, nil
}
