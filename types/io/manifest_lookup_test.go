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

func TestLookupTypeQualifiesManifestLocalAlias(t *testing.T) {
	event := typ.NewAlias("Event", typ.NewRecord().
		Field("kind", typ.String).
		Build())
	m := NewManifest("protocol")
	m.DefineType("Event", event)

	got, ok := m.LookupType("Event")
	if !ok {
		t.Fatal("LookupType(Event) should resolve")
	}
	alias, ok := got.(*typ.Alias)
	if !ok || alias.Name != "protocol.Event" {
		t.Fatalf("LookupType(Event) = %v, want protocol.Event alias", got)
	}
}

func TestLookupEnrichedExportQualifiesNestedManifestLocalAlias(t *testing.T) {
	event := typ.NewAlias("Event", typ.NewRecord().
		Field("kind", typ.String).
		Build())
	m := NewManifest("protocol")
	m.DefineType("Event", event)
	m.SetExport(typ.NewRecord().
		Field("make", typ.Func().
			Returns(event).
			Build()).
		Build())
	q := &manifestQuerierStub{
		manifests: map[string]*Manifest{"protocol": m},
	}

	got := LookupEnrichedExport(q, "protocol")
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("LookupEnrichedExport = %v, want record", got)
	}
	field := rec.GetField("make")
	if field == nil {
		t.Fatalf("LookupEnrichedExport missing make field: %v", got)
	}
	fn, ok := field.Type.(*typ.Function)
	if !ok || len(fn.Returns) != 1 {
		t.Fatalf("make field = %v, want one-return function", field.Type)
	}
	alias, ok := fn.Returns[0].(*typ.Alias)
	if !ok || alias.Name != "protocol.Event" {
		t.Fatalf("make return = %v, want protocol.Event alias", fn.Returns[0])
	}
}

func TestManifestLookupValue_RecordField(t *testing.T) {
	m := NewManifest("a")
	m.SetExport(typ.NewRecord().Field("name", typ.String).Build())
	got, ok := m.LookupValue("name")
	if !ok {
		t.Fatal("LookupValue(name) should resolve record field")
	}
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("LookupValue(name) = %v, want string", got)
	}
}

func TestManifestLookupValue_InterfaceMethod(t *testing.T) {
	fn := typ.Func().Param("x", typ.Integer).Returns(typ.String).Build()
	m := NewManifest("a")
	m.SetExport(typ.NewInterface("", []typ.Method{{Name: "run", Type: fn}}))
	got, ok := m.LookupValue("run")
	if !ok {
		t.Fatal("LookupValue(run) should resolve interface method")
	}
	if got != fn {
		t.Fatalf("LookupValue(run) = %v, want %v", got, fn)
	}
}

func TestManifestLookupValue_Missing(t *testing.T) {
	m := NewManifest("a")
	m.SetExport(typ.NewRecord().Field("name", typ.String).Build())
	if got, ok := m.LookupValue("missing"); ok || got != nil {
		t.Fatalf("LookupValue(missing) = (%v,%v), want (nil,false)", got, ok)
	}
}

func TestManifestLookupValue_CacheInvalidatesOnSetExport(t *testing.T) {
	m := NewManifest("a")
	m.SetExport(typ.NewRecord().Field("name", typ.String).Build())

	got, ok := m.LookupValue("name")
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("LookupValue(name) first = (%v,%v), want (string,true)", got, ok)
	}

	m.SetExport(typ.NewRecord().Field("name", typ.Integer).Build())
	got, ok = m.LookupValue("name")
	if !ok || !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("LookupValue(name) after SetExport = (%v,%v), want (integer,true)", got, ok)
	}
}

func TestManifestLookupValue_CacheInvalidatesOnDefineType(t *testing.T) {
	m := NewManifest("a")
	m.DefineType("NameType", typ.String)
	m.SetExport(typ.NewRecord().Field("name", typ.NewRef("", "NameType")).Build())

	got, ok := m.LookupValue("name")
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("LookupValue(name) first = (%v,%v), want (string,true)", got, ok)
	}

	m.DefineType("NameType", typ.Integer)
	got, ok = m.LookupValue("name")
	if !ok || !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("LookupValue(name) after DefineType = (%v,%v), want (integer,true)", got, ok)
	}
}
