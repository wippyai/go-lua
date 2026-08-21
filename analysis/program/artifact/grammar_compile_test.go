package artifact

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
)

func compileKeyProgram(t *testing.T, name string) *program.Program {
	t.Helper()
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyBody] = 1
	spans := make([]source.FamilySpans, 0, int(keyspace.FamilyCount)-1)
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		rows := make([]source.Span, counts[family])
		for index := range rows {
			line := uint32(index + 1)
			rows[index] = source.Span{File: name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		spans = append(spans, source.FamilySpans{Family: family, Spans: rows})
	}
	sourceDraft, err := source.Build(source.Input{Name: name, Families: spans, Bodies: []source.BodySource{{Body: entry}}})
	if err != nil {
		t.Fatal(err)
	}
	sourceFinalizer, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	staticComponent, staticView, err := programstatic.Build(programstatic.Input{Counts: counts})
	if err != nil {
		_ = sourceFinalizer.Abort()
		t.Fatal(err)
	}
	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		_ = sourceFinalizer.Abort()
		t.Fatal(err)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		_ = sourceFinalizer.Abort()
		t.Fatal(err)
	}
	flowDraft, err := flow.Build(flow.Input{Counts: counts})
	if err != nil {
		_ = moduleFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		t.Fatal(err)
	}
	assembly, err := flow.Assemble(sourceFinalizer, staticComponent, staticView, moduleFinalizer, flowDraft, entry)
	if err != nil {
		t.Fatal(err)
	}
	published, err := program.Publish(assembly)
	if err != nil || published == nil || !published.Available() {
		t.Fatal(err)
	}
	return published
}

func TestCompileKeyIsProgramAndGrammarOnly(t *testing.T) {
	grammar, grammarOK := NewGrammarIdentity(identity.ContentID{1}, GrammarABIVersion)
	if !grammarOK {
		t.Fatal("grammar identity unavailable")
	}
	left := compileKeyProgram(t, "compile-key-left.lua")
	right := compileKeyProgram(t, "compile-key-left.lua")
	changed := compileKeyProgram(t, "compile-key-changed.lua")
	leftKey, leftOK := NewCompileKey(left, grammar)
	rightKey, rightOK := NewCompileKey(right, grammar)
	changedKey, changedOK := NewCompileKey(changed, grammar)
	if !leftOK || !rightOK || !changedOK || !leftKey.Available() || !rightKey.Available() || !changedKey.Available() {
		t.Fatal("compile key unavailable")
	}
	if left.ContentID() != right.ContentID() || leftKey.ID() != rightKey.ID() || leftKey.ProgramID() != rightKey.ProgramID() {
		t.Fatal("equal Programs under one grammar issued distinct compile keys")
	}
	if changed.ContentID() == left.ContentID() || changedKey.ID() == leftKey.ID() {
		t.Fatal("changed Program reused a compile key")
	}
	foreignGrammar, foreignOK := NewGrammarIdentity(identity.ContentID{2}, GrammarABIVersion)
	foreignKey, foreignKeyed := NewCompileKey(left, foreignGrammar)
	if !foreignOK || !foreignKeyed || foreignKey.ID() == leftKey.ID() {
		t.Fatal("grammar digest did not enter the compile key")
	}
	keyType := reflect.TypeOf(CompileKey{})
	for index := 0; index < keyType.NumField(); index++ {
		name := keyType.Field(index).Name
		switch name {
		case "program", "grammar", "format", "compilerLaw", "operatorLaw", "substituteLaw", "summaryLaw",
			"wtoLaw", "routeLaw", "valuesLaw", "bodyOutcomeLaw", "functionBoundaryLaw", "occurrenceLaw",
			"diagnosticLaw", "callRowsLaw", "id":
		default:
			t.Fatalf("compile key carries non-program identity field %s", name)
		}
	}
}

func TestGrammarIdentityRejectsWrongABI(t *testing.T) {
	if grammar, ok := NewGrammarIdentity(identity.ContentID{1}, GrammarABIVersion-1); ok || grammar.Available() {
		t.Fatal("wrong grammar ABI was admitted")
	}
	grammar, ok := NewGrammarIdentity(identity.ContentID{1}, GrammarABIVersion)
	if !ok || !grammar.Available() || grammar.SchemaDigest() != (identity.ContentID{1}) {
		t.Fatal("valid grammar identity unavailable")
	}
}
