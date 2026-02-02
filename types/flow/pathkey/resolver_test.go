package pathkey

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

type mockGraph struct {
	versions map[cfg.Point]map[cfg.SymbolID]cfg.Version
}

func (m *mockGraph) VisibleVersion(p cfg.Point, sym cfg.SymbolID) cfg.Version {
	if m.versions == nil {
		return cfg.Version{}
	}
	if pointVersions, ok := m.versions[p]; ok {
		return pointVersions[sym]
	}
	return cfg.Version{}
}

func TestResolver_KeyAt_WithVersion(t *testing.T) {
	g := &mockGraph{
		versions: map[cfg.Point]map[cfg.SymbolID]cfg.Version{
			1: {100: {Root: "x", Symbol: 100, ID: 1}},
		},
	}
	r := NewResolver(g)

	path := constraint.Path{Root: "x", Symbol: 100}
	key := r.KeyAt(1, path)

	if key == "" {
		t.Fatal("expected non-empty key")
	}
	// Verify format: sym<SymbolID>@<VersionID>
	expected := "sym100@1"
	if string(key) != expected {
		t.Fatalf("expected %q, got %q", expected, key)
	}
}

func TestResolver_KeyAt_NoVersion_ReturnsEmpty(t *testing.T) {
	g := &mockGraph{versions: nil}
	r := NewResolver(g)

	path := constraint.Path{Root: "x", Symbol: 100}
	key := r.KeyAt(1, path)

	// No version = empty key (cannot create canonical key without version)
	if key != "" {
		t.Fatalf("expected empty key when no version, got %q", key)
	}
}

func TestResolver_KeyAt_EmptyPath(t *testing.T) {
	r := NewResolver(nil)

	path := constraint.Path{}
	key := r.KeyAt(1, path)

	if key != "" {
		t.Fatalf("expected empty key for empty path, got %q", key)
	}
}

func TestResolver_KeyAt_Placeholder(t *testing.T) {
	r := NewResolver(nil)

	path := constraint.Path{Root: "$0", Symbol: 0}
	key := r.KeyAt(1, path)

	// Placeholder uses Root directly
	if string(key) != "$0" {
		t.Fatalf("expected $0, got %q", key)
	}
}
