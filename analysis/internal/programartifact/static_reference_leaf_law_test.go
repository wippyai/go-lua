package programartifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/internal/programartifact"
	"github.com/wippyai/go-lua/analysis/internal/programartifact/schemaadapter"
	"github.com/wippyai/go-lua/analysis/internal/programschema"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/source"
	programstatic "github.com/wippyai/go-lua/program/static"
)

func compileStaticReferenceLeafArtifact(t *testing.T, name, text string) *programartifact.Artifact {
	t.Helper()
	published, err := lower.Lower(lower.Source{Name: name, Text: []byte(text)})
	if err != nil {
		t.Fatalf("Lower(%s): %v", name, err)
	}
	receipt, receiptOK := programschema.Global()
	if !receiptOK {
		t.Fatal("program schema receipt unavailable")
	}
	artifact, failure := schemaadapter.CompileDetailed(published.TransformerInput(), receipt)
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
	staticDraft, err := programstatic.Build(programstatic.Input{
		Counts: counts,
		References: programstatic.ReferencesInput{TypeRef: []programstatic.TypeRef{{
			Resolution: programstatic.TypeRefCanonicalPath,
			Source:     []keyspace.Key{1},
			Canonical:  []keyspace.Key{1},
		}}},
		Declarations: programstatic.DeclarationsInput{Alias: []programstatic.TypeAlias{{
			Owner: body, Target: reference, Name: 1, NameCoordinate: coordinate,
		}}},
	})
	if err != nil {
		_ = sourceFinalizer.Abort()
		t.Fatalf("static.Build canonical reference fixture: %v", err)
	}
	staticFinalizer, err := staticDraft.Finalizer()
	if err != nil {
		_ = sourceFinalizer.Abort()
		t.Fatalf("static.Finalizer canonical reference fixture: %v", err)
	}
	moduleDraft, err := module.Build(module.Input{})
	if err != nil {
		_ = staticFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		t.Fatalf("module.Build canonical reference fixture: %v", err)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		_ = staticFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		t.Fatalf("module.Finalizer canonical reference fixture: %v", err)
	}
	flowDraft, err := flow.Build(flow.Input{Counts: counts})
	if err != nil {
		_ = moduleFinalizer.Abort()
		_ = staticFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		t.Fatalf("flow.Build canonical reference fixture: %v", err)
	}
	assembly, err := flow.Assemble(sourceFinalizer, staticFinalizer, moduleFinalizer, flowDraft, body)
	if err != nil {
		t.Fatalf("flow.Assemble canonical reference fixture: %v", err)
	}
	published, err := program.Publish(assembly)
	if err != nil {
		t.Fatalf("program.Publish canonical reference fixture: %v", err)
	}
	resolution, target, root, referenceOK := published.Static().References().Get(reference)
	canonicalCount, canonicalOK := published.Static().References().CanonicalCount(reference)
	if !referenceOK || resolution != programstatic.TypeRefCanonicalPath || target != 0 || root != 0 ||
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
	for index := 0; index < unresolved.StaticTypeNodeCount(); index++ {
		row, rowOK := unresolved.StaticTypeNodeAt(index)
		if !rowOK {
			t.Fatalf("unresolved StaticTypeNodeAt(%d)", index)
		}
		if row.Kind() != programartifact.StaticNodeReference || row.Resolution() != uint8(programstatic.TypeRefUnresolved) {
			continue
		}
		if row.ChildCount() != 0 {
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
	for index := 0; index < resolved.StaticTypeNodeCount(); index++ {
		row, rowOK := resolved.StaticTypeNodeAt(index)
		if !rowOK {
			t.Fatalf("resolved StaticTypeNodeAt(%d)", index)
		}
		if row.Kind() != programartifact.StaticNodeReference || row.Resolution() == uint8(programstatic.TypeRefUnresolved) {
			continue
		}
		if row.ChildCount() == 0 {
			t.Fatal("sealed artifact admitted a targetless resolved static reference")
		}
		seenResolved = true
	}
	if !seenResolved {
		t.Fatal("fixture did not produce a resolved static reference")
	}

	canonical := canonicalPathStaticReferenceProgram(t)
	receipt, receiptOK := programschema.Global()
	if !receiptOK {
		t.Fatal("program schema receipt unavailable")
	}
	artifact, failure := schemaadapter.CompileDetailed(canonical.TransformerInput(), receipt)
	if artifact != nil || !failure.Available() || failure.Stage() != programartifact.CompileStageSeal ||
		failure.RowKind() != programartifact.CompileRowOccurrence || failure.Reason() != programartifact.CompileReasonOccurrenceUnavailable {
		t.Fatalf("CompileDetailed admitted targetless canonical reference: artifact=%v failure=%s", artifact != nil, failure.Error())
	}
}
