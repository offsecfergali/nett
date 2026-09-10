package store

import (
	"context"
	"fmt"
	"time"

	"github.com/alieddine/nett/internal/model"
)

// AddProvenance appends one observation record for an asset or edge. Unlike
// assets/edges, provenance is never merged — every observation from every
// source is kept so a researcher can answer "why does nett believe this
// exists?" in full.
func (s *Store) AddProvenance(ctx context.Context, p *model.Provenance) error {
	if p.ObservedAt.IsZero() {
		p.ObservedAt = time.Now()
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO provenance (project_id, subject_id, subject_kind, module, source, evidence, confidence, parent_id, observed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ProjectID, p.SubjectID, p.SubjectKind, p.Module, p.Source, p.Evidence, p.Confidence, p.ParentID, formatTime(p.ObservedAt))
	if err != nil {
		return fmt.Errorf("store: add provenance for %q: %w", p.SubjectID, err)
	}
	if id, err := res.LastInsertId(); err == nil {
		p.ID = id
	}
	return nil
}

// ListProvenance returns every observation recorded for subjectID, oldest
// first.
func (s *Store) ListProvenance(ctx context.Context, projectID, subjectID string) ([]*model.Provenance, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, subject_id, subject_kind, module, source, evidence, confidence, parent_id, observed_at
		FROM provenance WHERE project_id = ? AND subject_id = ?
		ORDER BY observed_at ASC, id ASC`, projectID, subjectID)
	if err != nil {
		return nil, fmt.Errorf("store: list provenance for %q: %w", subjectID, err)
	}
	defer rows.Close()

	var out []*model.Provenance
	for rows.Next() {
		var p model.Provenance
		var observedAt string
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.SubjectID, &p.SubjectKind, &p.Module, &p.Source, &p.Evidence, &p.Confidence, &p.ParentID, &observedAt); err != nil {
			return nil, fmt.Errorf("store: scan provenance: %w", err)
		}
		p.ObservedAt = parseTime(observedAt)
		out = append(out, &p)
	}
	return out, rows.Err()
}
