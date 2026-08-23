package acceptance_test

import (
	"strings"
	"sync"
	"testing"

	programlower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	"github.com/wippyai/go-lua/analysis/program/target/typeindex"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// hostTypeIndex is the sealed qualified type directory of the standard target.
// It is the same value the compile path hands a lowering, so a law here states
// the namespace the pipeline actually opens rather than a private stand-in.
var hostTypeIndex = sync.OnceValues(func() (typeindex.Table, error) {
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		return typeindex.Table{}, err
	}
	return target.Types(), nil
})

func lowerAgainstHostTypes(t *testing.T, source string) *program.Program {
	t.Helper()
	types, err := hostTypeIndex()
	if err != nil {
		t.Fatalf("seal the standard target: %v", err)
	}
	lowered, err := programlower.Lower(programlower.Source{Name: "fixture.lua", Text: []byte(source), Types: types})
	if err != nil {
		t.Fatal(err)
	}
	return lowered
}

// qualifiedReference is one authored type reference under its exact source
// spelling, with the canonical path its binder disposition carries.
type qualifiedReference struct {
	resolution staticrefs.Resolution
	canonical  string
}

func qualifiedReferences(t *testing.T, p *program.Program) map[string]qualifiedReference {
	t.Helper()
	view := p.Static()
	types := view.StaticTypes()
	references := view.References()
	out := make(map[string]qualifiedReference, types.Count())
	for index := 0; index < types.Count(); index++ {
		ref, ok := types.At(index)
		if !ok {
			t.Fatalf("static type %d is absent", index)
		}
		term := ref.Term()
		resolution, _, _, held := references.Get(term)
		if !held {
			continue
		}
		spelling := referencePath(t, p, term, references.SourceCount, references.SourceAt)
		canonical := referencePath(t, p, term, references.CanonicalCount, references.CanonicalAt)
		out[spelling] = qualifiedReference{resolution: resolution, canonical: canonical}
	}
	return out
}

func referencePath(
	t *testing.T,
	p *program.Program,
	term keyspace.Term,
	count func(keyspace.Term) (int, bool),
	at func(keyspace.Term, int) (keyspace.Key, bool),
) string {
	t.Helper()
	length, ok := count(term)
	if !ok {
		return ""
	}
	parts := make([]string, 0, length)
	for position := 0; position < length; position++ {
		key, keyOK := at(term, position)
		if !keyOK {
			t.Fatalf("type reference %v component %d is absent", term, position)
		}
		literal, literalOK := p.Source().Keys().Exact(key)
		if !literalOK || literal.Kind != keyspace.LiteralString {
			t.Fatalf("type reference %v component %d is not a literal name", term, position)
		}
		parts = append(parts, literal.String)
	}
	return strings.Join(parts, ".")
}

// TestQualifiedHostTypeNameResolvesThroughTheSealedTypeIndex is the qualified
// host namespace law. The Target publishes stream.Stream in its sealed
// qualified type index, so an annotation naming it resolves to that index row
// and carries its exact canonical path; nothing downstream sees an unresolved
// reference for a name the target declares.
func TestQualifiedHostTypeNameResolvesThroughTheSealedTypeIndex(t *testing.T) {
	p := lowerAgainstHostTypes(t, `local stream = require("stream")

local function name(handle: stream.Stream): string
	return handle.id
end

return name
`)
	reference, held := qualifiedReferences(t, p)["stream.Stream"]
	if !held {
		t.Fatal("type reference \"stream.Stream\" is absent from the lowered Program")
	}
	if reference.resolution != staticrefs.CanonicalPath {
		t.Fatalf("stream.Stream resolved %v, want %v", reference.resolution, staticrefs.CanonicalPath)
	}
	if reference.canonical != "stream.Stream" {
		t.Fatalf("stream.Stream canonical path = %q, want %q", reference.canonical, "stream.Stream")
	}
}

// TestQualifiedHostTypeNamePublishesThroughAStaticTypePublication is the
// cross-module law. A chunk that republishes a host type under its own module
// namespace publishes the same canonical path, so a sibling module naming the
// republished member reaches exactly one declaration rather than a second
// spelling of it.
func TestQualifiedHostTypeNamePublishesThroughAStaticTypePublication(t *testing.T) {
	p := lowerAgainstHostTypes(t, `local stream = require("stream")
local M = {}

M.Handle = stream.Stream

local function name(handle: M.Handle): string
	return handle.id
end

return name, M
`)
	references := qualifiedReferences(t, p)
	for _, spelling := range []string{"stream.Stream", "M.Handle"} {
		reference, held := references[spelling]
		if !held {
			t.Fatalf("type reference %q is absent from the lowered Program", spelling)
		}
		if reference.resolution != staticrefs.CanonicalPath {
			t.Fatalf("%s resolved %v, want %v", spelling, reference.resolution, staticrefs.CanonicalPath)
		}
		if reference.canonical != "stream.Stream" {
			t.Fatalf("%s canonical path = %q, want %q", spelling, reference.canonical, "stream.Stream")
		}
	}
}

// TestUnknownQualifiedTypeNameRefusesByName is the refusal law. A qualified
// name the sealed index does not publish stays unresolved and keeps its exact
// authored spelling, so the refusal names the type that was not found instead
// of reporting an anonymous failure.
func TestUnknownQualifiedTypeNameRefusesByName(t *testing.T) {
	p := lowerAgainstHostTypes(t, `local stream = require("stream")

local function name(handle: stream.Missing): string
	return handle.id
end

return name
`)
	reference, held := qualifiedReferences(t, p)["stream.Missing"]
	if !held {
		t.Fatal("type reference \"stream.Missing\" is absent from the lowered Program")
	}
	if reference.resolution != staticrefs.Unresolved {
		t.Fatalf("stream.Missing resolved %v, want %v", reference.resolution, staticrefs.Unresolved)
	}
	if reference.canonical != "" {
		t.Fatalf("stream.Missing carries canonical path %q; an unresolved reference names none", reference.canonical)
	}
}
