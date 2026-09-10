package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/offsecfergali/nett/internal/model"
)

// UpsertEdge inserts a new edge or, if one already exists between the same
// (src, rel, dst) in this project, advances its last_seen and keeps the max
// confidence — a relationship discovered by two modules keeps one edge row
// (each observation is still recorded separately via AddProvenance).
func (s *Store) UpsertEdge(ctx context.Context, e *model.Edge) error {
	if e.ID == "" {
		e.ID = model.EdgeID(e.SrcID, e.Rel, e.DstID)
	}
	now := time.Now()
	if e.FirstSeen.IsZero() {
		e.FirstSeen = now
	}
	if e.LastSeen.IsZero() {
		e.LastSeen = now
	}

	existing, err := s.getEdge(ctx, e.ProjectID, e.ID)
	if err != nil {
		return err
	}
	confidence := e.Confidence
	firstSeen := e.FirstSeen
	if existing != nil {
		if existing.Confidence > confidence {
			confidence = existing.Confidence
		}
		firstSeen = existing.FirstSeen
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO edges (id, project_id, src_id, dst_id, rel, confidence, first_seen, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, id) DO UPDATE SET
			confidence = excluded.confidence,
			last_seen  = excluded.last_seen`,
		e.ID, e.ProjectID, e.SrcID, e.DstID, string(e.Rel), confidence,
		formatTime(firstSeen), formatTime(e.LastSeen))
	if err != nil {
		return fmt.Errorf("store: upsert edge %q: %w", e.ID, err)
	}
	return nil
}

func (s *Store) getEdge(ctx context.Context, projectID, id string) (*model.Edge, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, src_id, dst_id, rel, confidence, first_seen, last_seen
		FROM edges WHERE project_id = ? AND id = ?`, projectID, id)
	return scanEdge(row)
}

// ListEdgesFrom returns every edge whose src_id is srcID, optionally
// filtered to one relationship type (pass "" for every type).
func (s *Store) ListEdgesFrom(ctx context.Context, projectID, srcID string, rel model.RelType) ([]*model.Edge, error) {
	query := `SELECT id, project_id, src_id, dst_id, rel, confidence, first_seen, last_seen
		FROM edges WHERE project_id = ? AND src_id = ?`
	args := []any{projectID, srcID}
	if rel != "" {
		query += " AND rel = ?"
		args = append(args, string(rel))
	}
	return s.queryEdges(ctx, query, args...)
}

// ListEdgesTo returns every edge whose dst_id is dstID, optionally filtered
// to one relationship type (pass "" for every type).
func (s *Store) ListEdgesTo(ctx context.Context, projectID, dstID string, rel model.RelType) ([]*model.Edge, error) {
	query := `SELECT id, project_id, src_id, dst_id, rel, confidence, first_seen, last_seen
		FROM edges WHERE project_id = ? AND dst_id = ?`
	args := []any{projectID, dstID}
	if rel != "" {
		query += " AND rel = ?"
		args = append(args, string(rel))
	}
	return s.queryEdges(ctx, query, args...)
}

func (s *Store) queryEdges(ctx context.Context, query string, args ...any) ([]*model.Edge, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list edges: %w", err)
	}
	defer rows.Close()

	var out []*model.Edge
	for rows.Next() {
		e, err := scanEdge(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanEdge(row rowScanner) (*model.Edge, error) {
	var e model.Edge
	var rel, firstSeen, lastSeen string
	if err := row.Scan(&e.ID, &e.ProjectID, &e.SrcID, &e.DstID, &rel, &e.Confidence, &firstSeen, &lastSeen); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("store: scan edge: %w", err)
	}
	e.Rel = model.RelType(rel)
	e.FirstSeen = parseTime(firstSeen)
	e.LastSeen = parseTime(lastSeen)
	return &e, nil
}
