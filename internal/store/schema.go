package store

// schema is applied on every Open via CREATE TABLE/INDEX IF NOT EXISTS, so
// opening an existing database is idempotent and safe. See docs/DATA_MODEL.md
// for the authoritative description of every column.
const schema = `
CREATE TABLE IF NOT EXISTS projects (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	created_at  TEXT NOT NULL,
	updated_at  TEXT NOT NULL,
	config_json TEXT NOT NULL DEFAULT '{}',
	status      TEXT NOT NULL DEFAULT 'active'
);

CREATE TABLE IF NOT EXISTS assets (
	id          TEXT NOT NULL,
	project_id  TEXT NOT NULL REFERENCES projects(id),
	type        TEXT NOT NULL,
	key         TEXT NOT NULL,
	value_json  TEXT NOT NULL DEFAULT '{}',
	in_scope    INTEGER NOT NULL DEFAULT 0,
	historical  INTEGER NOT NULL DEFAULT 0,
	confidence  REAL NOT NULL DEFAULT 0,
	first_seen  TEXT NOT NULL,
	last_seen   TEXT NOT NULL,
	PRIMARY KEY (project_id, id)
);
CREATE INDEX IF NOT EXISTS idx_assets_type ON assets(project_id, type);
CREATE INDEX IF NOT EXISTS idx_assets_key  ON assets(project_id, key);

CREATE TABLE IF NOT EXISTS edges (
	id          TEXT NOT NULL,
	project_id  TEXT NOT NULL REFERENCES projects(id),
	src_id      TEXT NOT NULL,
	dst_id      TEXT NOT NULL,
	rel         TEXT NOT NULL,
	confidence  REAL NOT NULL DEFAULT 0,
	first_seen  TEXT NOT NULL,
	last_seen   TEXT NOT NULL,
	PRIMARY KEY (project_id, id)
);
CREATE INDEX IF NOT EXISTS idx_edges_src ON edges(project_id, src_id, rel);
CREATE INDEX IF NOT EXISTS idx_edges_dst ON edges(project_id, dst_id, rel);

CREATE TABLE IF NOT EXISTS provenance (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id   TEXT NOT NULL,
	subject_id   TEXT NOT NULL,
	subject_kind TEXT NOT NULL,
	module       TEXT NOT NULL,
	source       TEXT NOT NULL,
	evidence     TEXT NOT NULL DEFAULT '',
	confidence   REAL NOT NULL DEFAULT 0,
	parent_id    TEXT NOT NULL DEFAULT '',
	observed_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_provenance_subject ON provenance(project_id, subject_id);

CREATE TABLE IF NOT EXISTS events (
	id           TEXT NOT NULL,
	project_id   TEXT NOT NULL,
	type         TEXT NOT NULL,
	asset_id     TEXT NOT NULL,
	source       TEXT NOT NULL,
	data_json    TEXT NOT NULL DEFAULT '{}',
	depth        INTEGER NOT NULL DEFAULT 0,
	status       TEXT NOT NULL DEFAULT 'pending',
	created_at   TEXT NOT NULL,
	processed_at TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (project_id, id)
);
CREATE INDEX IF NOT EXISTS idx_events_status ON events(project_id, status);

CREATE TABLE IF NOT EXISTS module_runs (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id   TEXT NOT NULL,
	module       TEXT NOT NULL,
	target       TEXT NOT NULL,
	input_fp     TEXT NOT NULL DEFAULT '',
	cursor_json  TEXT NOT NULL DEFAULT '{}',
	status       TEXT NOT NULL DEFAULT 'pending',
	error        TEXT NOT NULL DEFAULT '',
	started_at   TEXT NOT NULL DEFAULT '',
	finished_at  TEXT NOT NULL DEFAULT '',
	UNIQUE(project_id, module, target)
);

CREATE TABLE IF NOT EXISTS classifications (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id  TEXT NOT NULL,
	asset_id    TEXT NOT NULL,
	label       TEXT NOT NULL,
	confidence  REAL NOT NULL DEFAULT 0,
	evidence    TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS scores (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id   TEXT NOT NULL,
	asset_id     TEXT NOT NULL,
	score        INTEGER NOT NULL DEFAULT 0,
	priority     TEXT NOT NULL DEFAULT 'low',
	reasons_json TEXT NOT NULL DEFAULT '[]',
	computed_at  TEXT NOT NULL,
	UNIQUE(project_id, asset_id)
);
`
