package manifest

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/type/annotation"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestManifestDefineTypeAndSetExport(t *testing.T) {
	m := New("example/module")
	user := typ.NewRecord().
		Field("id", typ.Integer).
		OptField("name", typ.String).
		StaticStringIndex("role", typ.LiteralString("admin")).
		MapComponent(typ.String, typ.Number).
		Build()
	export := typ.Func().
		Param("user", user).
		Returns(typ.NewArray(user)).
		Build()

	m.DefineType("User", user)
	m.SetExport(export)

	if m.Path != "example/module" {
		t.Fatalf("path = %q", m.Path)
	}
	if got := m.Types["User"]; !typ.TypeEquals(got, user) {
		t.Fatalf("User type = %v, want %v", got, user)
	}
	if !typ.TypeEquals(m.Export, export) {
		t.Fatalf("export = %v, want %v", m.Export, export)
	}
}

func TestManifestRoundTrip(t *testing.T) {
	m := New("example/module")
	m.Version = "v1"
	m.DefineType("Status", typ.NewUnion(
		typ.LiteralString("ready"),
		typ.LiteralString("pending"),
	))
	m.DefineType("User", typ.NewAnnotated(
		typ.NewRecord().
			ReadonlyField("id", typ.Integer).
			OptField("name", typ.String).
			Build(),
		[]annotation.Annotation{{Name: "sealed", Arg: true}},
	))
	m.SetExport(typ.Func().
		TypeParam("T", typ.NewRef("", "User")).
		Param("value", typ.NewTypeParam("T", typ.NewRef("", "User"))).
		Returns(typ.NewReadonlyMap(typ.String, typ.NewRef("", "Status"))).
		Build())

	data, err := Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if got.Path != m.Path || got.Version != m.Version {
		t.Fatalf("metadata = %q/%q, want %q/%q", got.Path, got.Version, m.Path, m.Version)
	}
	if len(got.Types) != len(m.Types) {
		t.Fatalf("types = %d, want %d", len(got.Types), len(m.Types))
	}
	for name, want := range m.Types {
		if !typ.TypeEquals(got.Types[name], want) {
			t.Fatalf("type %s = %v, want %v", name, got.Types[name], want)
		}
	}
	if !typ.TypeEquals(got.Export, m.Export) {
		t.Fatalf("export = %v, want %v", got.Export, m.Export)
	}
}

func TestManifestNilHandling(t *testing.T) {
	if _, err := Encode(nil); err == nil {
		t.Fatalf("Encode(nil) succeeded")
	}
	if _, err := Decode(nil); err == nil {
		t.Fatalf("Decode(nil) succeeded")
	}
	if _, err := Decode([]byte("   \n\t")); err == nil {
		t.Fatalf("Decode(blank) succeeded")
	}
}

func TestManifestEncodeOrdersNamedTypesDeterministically(t *testing.T) {
	left := New("example/module")
	left.DefineType("Zed", typ.String)
	left.DefineType("Alpha", typ.Number)
	left.DefineType("Middle", typ.Boolean)

	right := New("example/module")
	right.DefineType("Middle", typ.Boolean)
	right.DefineType("Alpha", typ.Number)
	right.DefineType("Zed", typ.String)

	leftData, err := Encode(left)
	if err != nil {
		t.Fatalf("Encode(left): %v", err)
	}
	rightData, err := Encode(right)
	if err != nil {
		t.Fatalf("Encode(right): %v", err)
	}
	if !bytes.Equal(leftData, rightData) {
		t.Fatalf("encoding is not stable:\nleft:\n%s\nright:\n%s", leftData, rightData)
	}

	text := string(leftData)
	alpha := strings.Index(text, `"name": "Alpha"`)
	middle := strings.Index(text, `"name": "Middle"`)
	zed := strings.Index(text, `"name": "Zed"`)
	if alpha < 0 || middle < 0 || zed < 0 || !(alpha < middle && middle < zed) {
		t.Fatalf("named types are not sorted:\n%s", text)
	}
}
