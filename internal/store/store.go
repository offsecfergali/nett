// Package store implements nett's persistence layer: the SQLite-backed asset
// graph described in docs/DATA_MODEL.md. It is the only package that talks to
// the database; every other package goes through the *Store DAO.
//
// The driver is the pure-Go modernc.org/sqlite, deliberately chosen so the
// binary needs no cgo and no system SQLite library — consistent with the
// "clean container, native Go only" requirement the rest of the framework
// follows. SQLite is single-writer, so the pool is capped at one open
// connection: this makes every write serialize safely without a bespoke
// writer-goroutine abstraction that nothing yet needs.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/offsecfergali/nett/internal/model"
)

// Store is a handle to one project's-worth (or many projects') SQLite asset
// graph. It is safe for concurrent use.
type Store struct {
	db *sql.DB
}

// Open opens (creating if necessary) the SQLite database at path and applies
// the schema. path may be ":memory:" for an ephemeral, test-only database.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open %q: %w", path, err)
	}
	// SQLite allows exactly one writer at a time; serializing at the pool
	// level turns concurrent writers into a queue instead of "database is
	// locked" errors.
	db.SetMaxOpenConns(1)

	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA busy_timeout=5000;",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("store: %s: %w", pragma, err)
		}
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

const timeFmt = time.RFC3339Nano

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(timeFmt)
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(timeFmt, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func marshalJSON(v map[string]any) (string, error) {
	if v == nil {
		v = map[string]any{}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalJSON(s string) (map[string]any, error) {
	if s == "" {
		return map[string]any{}, nil
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, err
	}
	if v == nil {
		v = map[string]any{}
	}
	return v, nil
}

// mergeValue merges new on top of existing: scalar and object keys in new
// overwrite existing, but a []any in both is unioned (deduplicated by its
// JSON representation) rather than replaced — re-observation is additive, per
// docs/DATA_MODEL.md §4 ("attributes are additive ... never destroying
// earlier evidence").
func mergeValue(existing, next map[string]any) map[string]any {
	out := make(map[string]any, len(existing)+len(next))
	for k, v := range existing {
		out[k] = v
	}
	for k, v := range next {
		if newList, ok := v.([]any); ok {
			if oldList, ok := out[k].([]any); ok {
				out[k] = unionAny(oldList, newList)
				continue
			}
		}
		out[k] = v
	}
	return out
}

func unionAny(a, b []any) []any {
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]any, 0, len(a)+len(b))
	add := func(items []any) {
		for _, it := range items {
			b, err := json.Marshal(it)
			key := string(b)
			if err != nil {
				key = fmt.Sprintf("%v", it)
			}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, it)
		}
	}
	add(a)
	add(b)
	return out
}

// CreateProject creates a project if it does not already exist; it is a
// no-op (not an error) if the project is already present, so `nett scan
// --project acme` is safely idempotent across resumed runs.
func (s *Store) CreateProject(ctx context.Context, id, name, configJSON string) error {
	now := formatTime(time.Now())
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO projects (id, name, created_at, updated_at, config_json, status)
		VALUES (?, ?, ?, ?, ?, 'active')
		ON CONFLICT(id) DO NOTHING`,
		id, name, now, now, configJSON)
	if err != nil {
		return fmt.Errorf("store: create project %q: %w", id, err)
	}
	return nil
}

// GetProject returns the project with the given id, or (nil, nil) if absent.
func (s *Store) GetProject(ctx context.Context, id string) (*model.Project, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, created_at, updated_at, config_json, status
		FROM projects WHERE id = ?`, id)
	var p model.Project
	var created, updated string
	if err := row.Scan(&p.ID, &p.Name, &created, &updated, &p.ConfigJSON, &p.Status); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("store: get project %q: %w", id, err)
	}
	p.CreatedAt = parseTime(created)
	p.UpdatedAt = parseTime(updated)
	return &p, nil
}

// UpsertAsset inserts a new asset or merges into an existing one with the
// same (project, id): value_json is additively merged, confidence takes the
// max of old and new, first_seen is preserved, and last_seen advances.
func (s *Store) UpsertAsset(ctx context.Context, a *model.Asset) error {
	if a.ID == "" {
		a.ID = model.AssetID(a.Type, a.Key)
	}
	now := time.Now()
	if a.FirstSeen.IsZero() {
		a.FirstSeen = now
	}
	if a.LastSeen.IsZero() {
		a.LastSeen = now
	}

	existing, err := s.GetAsset(ctx, a.ProjectID, a.ID)
	if err != nil {
		return err
	}

	value := a.Value
	confidence := a.Confidence
	firstSeen := a.FirstSeen
	inScope := a.InScope
	historical := a.Historical
	if existing != nil {
		value = mergeValue(existing.Value, a.Value)
		if existing.Confidence > confidence {
			confidence = existing.Confidence
		}
		firstSeen = existing.FirstSeen
		// in_scope/historical are verdicts fixed at first insert.
		inScope = existing.InScope
		historical = existing.Historical
	}

	valueJSON, err := marshalJSON(value)
	if err != nil {
		return fmt.Errorf("store: marshal asset value: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO assets (id, project_id, type, key, value_json, in_scope, historical, confidence, first_seen, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, id) DO UPDATE SET
			value_json = excluded.value_json,
			confidence = excluded.confidence,
			last_seen  = excluded.last_seen`,
		a.ID, a.ProjectID, string(a.Type), a.Key, valueJSON,
		boolToInt(inScope), boolToInt(historical), confidence,
		formatTime(firstSeen), formatTime(a.LastSeen))
	if err != nil {
		return fmt.Errorf("store: upsert asset %q: %w", a.ID, err)
	}
	return nil
}

// GetAsset returns the asset with the given id in project, or (nil, nil) if
// it does not exist.
func (s *Store) GetAsset(ctx context.Context, projectID, id string) (*model.Asset, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, type, key, value_json, in_scope, historical, confidence, first_seen, last_seen
		FROM assets WHERE project_id = ? AND id = ?`, projectID, id)
	return scanAsset(row)
}

// ListAssets returns every asset in project, optionally filtered to one
// AssetType (pass "" for every type), ordered by first_seen.
func (s *Store) ListAssets(ctx context.Context, projectID string, assetType model.AssetType) ([]*model.Asset, error) {
	query := `
		SELECT id, project_id, type, key, value_json, in_scope, historical, confidence, first_seen, last_seen
		FROM assets WHERE project_id = ?`
	args := []any{projectID}
	if assetType != "" {
		query += " AND type = ?"
		args = append(args, string(assetType))
	}
	query += " ORDER BY first_seen ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list assets: %w", err)
	}
	defer rows.Close()

	var out []*model.Asset
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanAsset(row rowScanner) (*model.Asset, error) {
	var a model.Asset
	var typ, valueJSON, firstSeen, lastSeen string
	var inScope, historical int
	if err := row.Scan(&a.ID, &a.ProjectID, &typ, &a.Key, &valueJSON, &inScope, &historical, &a.Confidence, &firstSeen, &lastSeen); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("store: scan asset: %w", err)
	}
	value, err := unmarshalJSON(valueJSON)
	if err != nil {
		return nil, fmt.Errorf("store: unmarshal asset value: %w", err)
	}
	a.Type = model.AssetType(typ)
	a.Value = value
	a.InScope = inScope != 0
	a.Historical = historical != 0
	a.FirstSeen = parseTime(firstSeen)
	a.LastSeen = parseTime(lastSeen)
	return &a, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
