package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/alieddine/nett/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	s1.Close()
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open (re-applying schema): %v", err)
	}
	s2.Close()
}

func TestCreateProjectIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.CreateProject(ctx, "acme", "Acme Corp", "{}"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if err := s.CreateProject(ctx, "acme", "Acme Corp (renamed, ignored)", "{}"); err != nil {
		t.Fatalf("CreateProject (repeat): %v", err)
	}
	p, err := s.GetProject(ctx, "acme")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if p == nil {
		t.Fatal("GetProject returned nil")
	}
	if p.Name != "Acme Corp" {
		t.Errorf("Name = %q, want original name preserved on repeat create", p.Name)
	}
}

func TestGetProjectMissing(t *testing.T) {
	s := newTestStore(t)
	p, err := s.GetProject(context.Background(), "nope")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if p != nil {
		t.Errorf("GetProject(missing) = %+v, want nil", p)
	}
}

func TestUpsertAssetMergesAdditively(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.CreateProject(ctx, "p1", "P1", "{}"); err != nil {
		t.Fatal(err)
	}

	a := &model.Asset{
		ProjectID:  "p1",
		Type:       model.AssetHostname,
		Key:        "api.example.com",
		Value:      map[string]any{"fqdn": "api.example.com", "urls": []any{"https://api.example.com/"}},
		Confidence: 0.5,
	}
	if err := s.UpsertAsset(ctx, a); err != nil {
		t.Fatalf("UpsertAsset (insert): %v", err)
	}
	id := a.ID
	firstSeen := a.FirstSeen

	a2 := &model.Asset{
		ProjectID:  "p1",
		Type:       model.AssetHostname,
		Key:        "api.example.com",
		Value:      map[string]any{"resolved": true, "urls": []any{"https://api.example.com/v2"}},
		Confidence: 0.9,
	}
	if err := s.UpsertAsset(ctx, a2); err != nil {
		t.Fatalf("UpsertAsset (merge): %v", err)
	}
	if a2.ID != id {
		t.Fatalf("deterministic ID changed: %q vs %q", a2.ID, id)
	}

	got, err := s.GetAsset(ctx, "p1", id)
	if err != nil {
		t.Fatalf("GetAsset: %v", err)
	}
	if got == nil {
		t.Fatal("GetAsset returned nil after upsert")
	}
	if got.Confidence != 0.9 {
		t.Errorf("confidence = %v, want max(0.5,0.9)=0.9", got.Confidence)
	}
	if !got.FirstSeen.Equal(firstSeen) {
		t.Errorf("first_seen changed on re-observation: %v vs %v", got.FirstSeen, firstSeen)
	}
	if got.Value["fqdn"] != "api.example.com" {
		t.Errorf("earlier field %q lost on merge: %+v", "fqdn", got.Value)
	}
	if got.Value["resolved"] != true {
		t.Errorf("new field %q missing after merge: %+v", "resolved", got.Value)
	}
	urls, ok := got.Value["urls"].([]any)
	if !ok || len(urls) != 2 {
		t.Errorf("urls should be unioned to 2 entries, got %+v", got.Value["urls"])
	}
}

func TestUpsertAssetPreservesScopeVerdict(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	s.CreateProject(ctx, "p1", "P1", "{}")

	a := &model.Asset{ProjectID: "p1", Type: model.AssetHostname, Key: "h.example.com", InScope: true}
	if err := s.UpsertAsset(ctx, a); err != nil {
		t.Fatal(err)
	}
	a2 := &model.Asset{ProjectID: "p1", Type: model.AssetHostname, Key: "h.example.com", InScope: false}
	if err := s.UpsertAsset(ctx, a2); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetAsset(ctx, "p1", a.ID)
	if !got.InScope {
		t.Error("in_scope verdict should be fixed at first insert, not overwritten")
	}
}

func TestListAssetsFiltersByType(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	s.CreateProject(ctx, "p1", "P1", "{}")

	s.UpsertAsset(ctx, &model.Asset{ProjectID: "p1", Type: model.AssetHostname, Key: "a.example.com"})
	s.UpsertAsset(ctx, &model.Asset{ProjectID: "p1", Type: model.AssetIP, Key: "1.2.3.4"})
	s.UpsertAsset(ctx, &model.Asset{ProjectID: "p1", Type: model.AssetHostname, Key: "b.example.com"})

	hosts, err := s.ListAssets(ctx, "p1", model.AssetHostname)
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 {
		t.Errorf("len(hosts) = %d, want 2", len(hosts))
	}

	all, err := s.ListAssets(ctx, "p1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Errorf("len(all) = %d, want 3", len(all))
	}
}

func TestUpsertEdgeMergesConfidence(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	s.CreateProject(ctx, "p1", "P1", "{}")

	e := &model.Edge{ProjectID: "p1", SrcID: "src", DstID: "dst", Rel: model.RelResolvesTo, Confidence: 0.4}
	if err := s.UpsertEdge(ctx, e); err != nil {
		t.Fatal(err)
	}
	id := e.ID
	e2 := &model.Edge{ProjectID: "p1", SrcID: "src", DstID: "dst", Rel: model.RelResolvesTo, Confidence: 0.8}
	if err := s.UpsertEdge(ctx, e2); err != nil {
		t.Fatal(err)
	}
	if e2.ID != id {
		t.Fatalf("edge ID changed: %q vs %q", e2.ID, id)
	}

	edges, err := s.ListEdgesFrom(ctx, "p1", "src", model.RelResolvesTo)
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 1 {
		t.Fatalf("len(edges) = %d, want 1 (upsert must not duplicate)", len(edges))
	}
	if edges[0].Confidence != 0.8 {
		t.Errorf("confidence = %v, want max(0.4,0.8)=0.8", edges[0].Confidence)
	}
}

func TestListEdgesToDirection(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	s.CreateProject(ctx, "p1", "P1", "{}")

	s.UpsertEdge(ctx, &model.Edge{ProjectID: "p1", SrcID: "a", DstID: "b", Rel: model.RelResolvesTo})
	s.UpsertEdge(ctx, &model.Edge{ProjectID: "p1", SrcID: "c", DstID: "b", Rel: model.RelResolvesTo})
	s.UpsertEdge(ctx, &model.Edge{ProjectID: "p1", SrcID: "a", DstID: "z", Rel: model.RelResolvesTo})

	incoming, err := s.ListEdgesTo(ctx, "p1", "b", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(incoming) != 2 {
		t.Errorf("len(incoming to b) = %d, want 2", len(incoming))
	}
}

func TestProvenanceAccumulatesPerSubject(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	s.CreateProject(ctx, "p1", "P1", "{}")

	subject := "asset123"
	for _, mod := range []string{"ct", "dns"} {
		if err := s.AddProvenance(ctx, &model.Provenance{
			ProjectID: "p1", SubjectID: subject, SubjectKind: "asset",
			Module: mod, Source: mod + "-source", Confidence: 1.0,
		}); err != nil {
			t.Fatalf("AddProvenance(%s): %v", mod, err)
		}
	}
	rows, err := s.ListProvenance(ctx, "p1", subject)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(provenance) = %d, want 2 (one per source, never merged)", len(rows))
	}
}

func TestInsertEventDedupes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	s.CreateProject(ctx, "p1", "P1", "{}")

	e := &model.Event{ProjectID: "p1", Type: "NEW_SUBDOMAIN", AssetID: "asset1", Source: "ct"}
	ok, err := s.InsertEvent(ctx, e)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("first InsertEvent should report inserted=true")
	}

	dup := &model.Event{ProjectID: "p1", Type: "NEW_SUBDOMAIN", AssetID: "asset1", Source: "dns"}
	ok, err = s.InsertEvent(ctx, dup)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("duplicate event (same type+asset) should report inserted=false")
	}

	pending, err := s.PendingEvents(ctx, "p1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("len(pending) = %d, want 1 after dedup", len(pending))
	}

	if err := s.MarkEventStatus(ctx, "p1", pending[0].ID, "processed"); err != nil {
		t.Fatal(err)
	}
	pending, err = s.PendingEvents(ctx, "p1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("len(pending) = %d, want 0 after marking processed", len(pending))
	}
}

func TestModuleRunSupportsResume(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	s.CreateProject(ctx, "p1", "P1", "{}")

	none, err := s.GetModuleRun(ctx, "p1", "dns", "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if none != nil {
		t.Fatal("GetModuleRun should be nil before any run recorded")
	}

	run := &model.ModuleRun{ProjectID: "p1", Module: "dns", Target: "example.com", Status: "running", Cursor: map[string]any{"offset": float64(5)}}
	if err := s.UpsertModuleRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	run.Status = "done"
	run.Cursor = map[string]any{"offset": float64(42)}
	if err := s.UpsertModuleRun(ctx, run); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetModuleRun(ctx, "p1", "dns", "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Status != "done" {
		t.Fatalf("GetModuleRun = %+v, want status=done", got)
	}
	if got.Cursor["offset"] != float64(42) {
		t.Errorf("cursor = %+v, want offset=42", got.Cursor)
	}
}
