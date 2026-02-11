package io

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

type manifestQuerierStub struct {
	manifests map[string]*Manifest
	imports   map[string]*Manifest
}

func (s *manifestQuerierStub) Manifest(path string) *Manifest {
	if s == nil || s.manifests == nil {
		return nil
	}
	return s.manifests[path]
}

func (s *manifestQuerierStub) Imports() map[string]*Manifest {
	if s == nil {
		return nil
	}
	return s.imports
}

func TestLookupManifest_DirectHit(t *testing.T) {
	m := NewManifest("a")
	q := &manifestQuerierStub{
		manifests: map[string]*Manifest{"a": m},
	}
	if got := LookupManifest(q, "a"); got != m {
		t.Fatalf("LookupManifest direct = %v, want %v", got, m)
	}
}

func TestLookupManifest_ImportsFallback(t *testing.T) {
	m := NewManifest("a")
	q := &manifestQuerierStub{
		manifests: map[string]*Manifest{},
		imports:   map[string]*Manifest{"a": m},
	}
	if got := LookupManifest(q, "a"); got != m {
		t.Fatalf("LookupManifest fallback = %v, want %v", got, m)
	}
}

func TestLookupManifest_NilAndEmpty(t *testing.T) {
	if got := LookupManifest(nil, "a"); got != nil {
		t.Fatalf("LookupManifest nil querier = %v, want nil", got)
	}
	if got := LookupManifest(&manifestQuerierStub{}, ""); got != nil {
		t.Fatalf("LookupManifest empty path = %v, want nil", got)
	}
}

func TestLookupEnrichedExport(t *testing.T) {
	m := NewManifest("a")
	m.SetExport(typ.String)
	q := &manifestQuerierStub{
		manifests: map[string]*Manifest{"a": m},
	}
	got := LookupEnrichedExport(q, "a")
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("LookupEnrichedExport = %v, want string", got)
	}
}
