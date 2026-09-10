// Package model defines nett's core domain types — the in-memory shape of the
// asset graph persisted by internal/store. See docs/DATA_MODEL.md for the
// full contract: this package and that document must stay in sync.
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// AssetType identifies the kind of node in the asset graph.
type AssetType string

const (
	AssetDomain        AssetType = "domain"
	AssetHostname      AssetType = "hostname"
	AssetIP            AssetType = "ip"
	AssetCIDR          AssetType = "cidr"
	AssetASN           AssetType = "asn"
	AssetURL           AssetType = "url"
	AssetEndpoint      AssetType = "endpoint"
	AssetParameter     AssetType = "parameter"
	AssetCertificate   AssetType = "certificate"
	AssetJSFile        AssetType = "jsfile"
	AssetRepository    AssetType = "repository"
	AssetCloudProvider AssetType = "cloud_provider"
	AssetService       AssetType = "service"
	AssetPort          AssetType = "port"
)

// RelType identifies the kind of a directed edge between two assets.
type RelType string

const (
	RelHasSubdomain RelType = "HAS_SUBDOMAIN"
	RelResolvesTo   RelType = "RESOLVES_TO"
	RelAnnouncedBy  RelType = "ANNOUNCED_BY"
	RelContains     RelType = "CONTAINS"
	RelPTR          RelType = "PTR"
	RelPresented    RelType = "PRESENTED"
	RelSAN          RelType = "SAN"
	RelServes       RelType = "SERVES"
	RelExposes      RelType = "EXPOSES"
	RelAccepts      RelType = "ACCEPTS"
	RelReferences   RelType = "REFERENCES"
	RelReveals      RelType = "REVEALS"
	RelMentions     RelType = "MENTIONS"
	RelLinkedRepo   RelType = "LINKED_REPO"
	RelHostedOn     RelType = "HOSTED_ON"
	RelOnPort       RelType = "ON_PORT"
)

// Asset is one node in the asset graph.
type Asset struct {
	ID         string
	ProjectID  string
	Type       AssetType
	Key        string
	Value      map[string]any
	InScope    bool
	Historical bool
	Confidence float64
	FirstSeen  time.Time
	LastSeen   time.Time
}

// Edge is one directed, typed relationship between two assets.
type Edge struct {
	ID         string
	ProjectID  string
	SrcID      string
	DstID      string
	Rel        RelType
	Confidence float64
	FirstSeen  time.Time
	LastSeen   time.Time
}

// Provenance records why a single asset or edge is believed to exist. Many
// rows may exist per subject, one per observation/source.
type Provenance struct {
	ID          int64
	ProjectID   string
	SubjectID   string
	SubjectKind string // "asset" | "edge"
	Module      string
	Source      string
	Evidence    string
	Confidence  float64
	ParentID    string
	ObservedAt  time.Time
}

// Event is a typed occurrence on the project's event log, consumed by the
// pivot engine (M21).
type Event struct {
	ID          string
	ProjectID   string
	Type        string
	AssetID     string
	Source      string
	Data        map[string]any
	Depth       int
	Status      string // "pending" | "processed" | "dropped"
	CreatedAt   time.Time
	ProcessedAt time.Time
}

// ModuleRun is the execution record for one (module, target) pair, used to
// support resume/incremental recon (M23).
type ModuleRun struct {
	ID         int64
	ProjectID  string
	Module     string
	Target     string
	InputFP    string
	Cursor     map[string]any
	Status     string // "pending" | "running" | "done" | "error"
	Error      string
	StartedAt  time.Time
	FinishedAt time.Time
}

// Project is one reconnaissance project: the unit of resume and diff.
type Project struct {
	ID         string
	Name       string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	ConfigJSON string
	Status     string // "active" | "paused" | "complete"
}

// AssetID returns the deterministic identity of an asset: a hex SHA-256 of
// its type and canonical key. The same asset discovered by two modules always
// collapses to the same ID.
func AssetID(t AssetType, canonicalKey string) string {
	return hashHex(string(t), canonicalKey)
}

// EdgeID returns the deterministic identity of an edge.
func EdgeID(srcID string, rel RelType, dstID string) string {
	return hashHex(srcID, string(rel), dstID)
}

// EventID returns the deterministic dedup key for an event: a hash of its
// type and subject asset, so the same occurrence is never scheduled twice.
func EventID(eventType, assetID string) string {
	return hashHex(eventType, assetID)
}

func hashHex(parts ...string) string {
	h := sha256.New()
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte{0})
		}
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}
