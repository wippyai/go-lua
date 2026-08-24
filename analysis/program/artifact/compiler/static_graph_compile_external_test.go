package compiler_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
	staticnode "github.com/wippyai/go-lua/analysis/schema/program/staticnode"
	"github.com/wippyai/go-lua/domain/composite"
)

func staticNodeTestView(t testing.TB, program programschema.Program) staticnode.View {
	t.Helper()
	catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
	state, stateOK := programstate.New(program.Frozen, catalog)
	view, viewOK := staticnode.NewView(state)
	if !catalogOK || !stateOK || !viewOK {
		t.Fatal("static node view")
	}
	return view
}

func compileStaticReferenceLeafArtifact(t *testing.T, name, text string) *programartifact.Artifact {
	t.Helper()
	published, err := lower.Lower(lower.Source{Name: name, Text: []byte(text)})
	if err != nil {
		t.Fatalf("Lower(%s): %v", name, err)
	}
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("program schema unavailable")
	}
	artifact, failure := compileArtifactForTest(t, published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("CompileDetailed(%s): %s", name, failure.Error())
	}
	return artifact
}

func canonicalPathStaticReferenceProgram(t *testing.T) *program.Program {
	t.Helper()
	const name = "static-reference-canonical.lua"
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	alias := keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
	reference := keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyTypeAlias] = 1
	counts[keyspace.FamilyTypeRef] = 1
	spans := make([]source.FamilySpans, 0, int(keyspace.FamilyCount)-1)
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		rows := make([]source.Span, counts[family])
		for index := range rows {
			rows[index] = source.Span{
				File: name, StartLine: uint32(index + 1), StartCol: 1,
				EndLine: uint32(index + 1), EndCol: 2,
			}
		}
		spans = append(spans, source.FamilySpans{Family: family, Spans: rows})
	}
	sourceDraft, err := source.Build(source.Input{
		Name: name, Families: spans,
		Bodies:     []source.BodySource{{Body: body, Terms: []keyspace.Term{alias}}},
		ExactAtoms: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "Remote"}},
	})
	if err != nil {
		t.Fatalf("source.Build canonical reference fixture: %v", err)
	}
	sourceFinalizer, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer canonical reference fixture: %v", err)
	}
	coordinate, coordinateOK := source.CoordinateFromParts(1, 1, 1, 2)
	if !coordinateOK {
		_ = sourceFinalizer.Abort()
		t.Fatal("canonical reference fixture coordinate unavailable")
	}
	staticComponent, staticView, err := programstatic.Build(programstatic.Input{
		Counts: counts,
		References: staticrefs.Input{TypeRef: []staticrefs.TypeRef{{
			Resolution: staticrefs.CanonicalPath,
			Source:     []keyspace.Key{1},
			Canonical:  []keyspace.Key{1},
		}}},
		Declarations: staticdecl.Input{Alias: []staticdecl.TypeAlias{{
			Owner: body, Target: reference, Name: 1, NameCoordinate: coordinate,
		}}},
	})
	if err != nil {
		_ = sourceFinalizer.Abort()
		t.Fatalf("static.Build canonical reference fixture: %v", err)
	}
	flowDraft, err := authored.Build(authored.Input{Counts: counts})
	if err != nil {
		_ = sourceFinalizer.Abort()
		t.Fatalf("flow.Build canonical reference fixture: %v", err)
	}
	assembly, err := flow.Assemble(sourceFinalizer, staticComponent, staticView, flowDraft, body)
	if err != nil {
		t.Fatalf("flow.Assemble canonical reference fixture: %v", err)
	}
	published, err := program.Publish(assembly)
	if err != nil {
		t.Fatalf("program.Publish canonical reference fixture: %v", err)
	}
	resolution, target, root, referenceOK := published.Static().References().Get(reference)
	canonicalCount, canonicalOK := published.Static().References().CanonicalCount(reference)
	if !referenceOK || resolution != staticrefs.CanonicalPath || target != 0 || root != 0 ||
		!canonicalOK || canonicalCount != 1 {
		t.Fatalf("canonical reference = resolution %v target %v root %v canonical %d/%v", resolution, target, root, canonicalCount, canonicalOK)
	}
	return published
}

func TestStaticReferenceLeafAdmissionDistinguishesUnresolvedFromResolved(t *testing.T) {
	unresolved := compileStaticReferenceLeafArtifact(t, "static-reference-unresolved.lua", `
type Declared = { x: number }
local value: Missing = { x = 1 }
return value
`)
	seenUnresolved := false
	program := unresolved.Program()
	view := staticNodeTestView(t, program)
	count, countOK := view.StaticTypeNodeCount()
	if !countOK {
		t.Fatal("unresolved Program static graph unavailable")
	}
	for index := 0; index < count; index++ {
		row, rowOK := view.StaticTypeNodeAt(index)
		if !rowOK {
			t.Fatalf("unresolved StaticTypeNodeAt(%d)", index)
		}
		if row.Kind() != staticnode.StaticNodeReference || row.Resolution() != uint8(staticrefs.Unresolved) {
			continue
		}
		if _, targetOK := row.ReferenceTarget(); targetOK {
			t.Fatal("unresolved reference retained a target child")
		}
		seenUnresolved = true
	}
	if !seenUnresolved {
		t.Fatal("fixture did not produce a targetless unresolved static reference")
	}

	resolved := compileStaticReferenceLeafArtifact(t, "static-reference-resolved.lua", `
type Declared = { x: number }
local value: Declared = { x = 1 }
return value
`)
	seenResolved := false
	program = resolved.Program()
	view = staticNodeTestView(t, program)
	count, countOK = view.StaticTypeNodeCount()
	if !countOK {
		t.Fatal("resolved Program static graph unavailable")
	}
	for index := 0; index < count; index++ {
		row, rowOK := view.StaticTypeNodeAt(index)
		if !rowOK {
			t.Fatalf("resolved StaticTypeNodeAt(%d)", index)
		}
		if row.Kind() != staticnode.StaticNodeReference || row.Resolution() == uint8(staticrefs.Unresolved) {
			continue
		}
		if _, targetOK := row.ReferenceTarget(); !targetOK {
			t.Fatal("sealed artifact admitted a targetless resolved static reference")
		}
		seenResolved = true
	}
	if !seenResolved {
		t.Fatal("fixture did not produce a resolved static reference")
	}

	canonical := canonicalPathStaticReferenceProgram(t)
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("program schema unavailable")
	}
	artifact, failure := compileArtifactForTest(t, canonical, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("CompileDetailed refused canonical-path reference leaf: artifact=%v failure=%s", artifact != nil, failure.Error())
	}
	seenCanonical := false
	program = artifact.Program()
	view = staticNodeTestView(t, program)
	count, countOK = view.StaticTypeNodeCount()
	if !countOK {
		t.Fatal("canonical Program static graph unavailable")
	}
	for index := 0; index < count; index++ {
		row, rowOK := view.StaticTypeNodeAt(index)
		if !rowOK {
			t.Fatalf("canonical StaticTypeNodeAt(%d)", index)
		}
		if row.Kind() != staticnode.StaticNodeReference || row.Resolution() != uint8(staticrefs.CanonicalPath) {
			continue
		}
		if _, targetOK := row.ReferenceTarget(); targetOK {
			t.Fatal("canonical-path reference acquired a local declaration target")
		}
		seenCanonical = true
	}
	if !seenCanonical {
		t.Fatal("sealed artifact lost the canonical-path reference leaf")
	}
}
