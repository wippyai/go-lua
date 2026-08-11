package directbinding

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	flowbody "github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
)

// TestSealRejectsBracketStringPublication keeps selector admission and
// publication-path admission separate: a normalized FieldExact string may
// select a Read, but Static publication paths require authored FieldName
// segments.
func TestSealRejectsBracketStringPublication(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	stringTerm := keyspace.MakeTerm(keyspace.FamilyString, 1)
	lens := keyspace.MakeTerm(keyspace.FamilyLensExact, 1)
	readRoot := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	readField := keyspace.MakeTerm(keyspace.FamilyRead, 2)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	write := keyspace.MakeTerm(keyspace.FamilyWrite, 1)
	ref := keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)

	var counts [keyspace.FamilyCount]uint32
	for _, term := range []keyspace.Term{body, cell, stringTerm, lens, readRoot, readField, values, assign, write} {
		counts[keyspace.TermFamily(term)]++
	}
	counts[keyspace.FamilyTypeRef] = 1
	counts[keyspace.FamilyTypePublication] = 1

	input := source.Input{
		Name:       "directbinding-bracket-publication.lua",
		ExactAtoms: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "x"}},
		String:     []source.StringLiteral{{Owner: body, Value: "x"}},
		Bodies:     []source.BodySource{{Body: body, Terms: []keyspace.Term{assign}}},
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, counts[family])
		for index := range spans {
			line := uint32(index + 1)
			spans[index] = source.Span{File: input.Name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		input.Families = append(input.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	sourceDraft, err := source.Build(input)
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalizer, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	defer sourceFinalizer.Abort()

	flowDraft, err := authored.Build(authored.Input{
		Counts: counts,
		Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}}},
		Access: authored.AccessInput{Exact: []authored.ExactLens{{
			Owner: body, Base: readRoot, Source: stringTerm, Kind: kind.FieldExact,
		}}},
		Storage: authored.StorageInput{
			Cells:   []authored.Cell{{Kind: authored.CellGlobal, Key: 1}},
			Reads:   []authored.Read{{Owner: body, Source: cell}, {Owner: body, Source: lens}},
			Assigns: []authored.Assign{{Owner: body, Values: values}},
			Writes:  []authored.Write{{Assign: assign, Target: lens}},
		},
	})
	if err != nil {
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalizer, err := flowDraft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer: %v", err)
	}
	defer flowFinalizer.Abort()

	moduleDraft, err := module.Build(module.Input{})
	if err != nil {
		t.Fatalf("module.Build: %v", err)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		t.Fatalf("module.Finalizer: %v", err)
	}
	defer moduleFinalizer.Abort()

	staticDraft, err := static.Build(static.Input{
		Counts: counts,
		References: static.ReferencesInput{TypeRef: []static.TypeRef{{
			Resolution: static.TypeRefCanonicalPath,
			Source:     []keyspace.Key{1},
			Canonical:  []keyspace.Key{1},
		}}},
		Publications: static.PublicationsInput{Type: []static.Publication{{
			Assign: assign, Pair: 0, Target: ref,
		}}},
	})
	if err != nil {
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalizer, err := staticDraft.Finalizer()
	if err != nil {
		t.Fatalf("static.Finalizer: %v", err)
	}
	defer staticFinalizer.Abort()

	bindings := directBindingProof(t, sourceFinalizer.Preimage(), flowFinalizer.View(), staticFinalizer.View(), body)
	bodyResult, bodyErr := flowbody.Seal(sourceFinalizer.Preimage(), flowFinalizer.View(), staticFinalizer.View(), body)
	if bodyErr != nil {
		t.Fatalf("body.Seal: %v", bodyErr)
	}
	_, err = Seal(sourceFinalizer.Preimage(), flowFinalizer.View(), bodyResult, bindings, staticFinalizer.View(), moduleFinalizer.View())
	if err == nil || !strings.Contains(err.Error(), "publication target is not a same-owner name lens") {
		t.Fatalf("Seal error = %v, want bracket-string publication rejection", err)
	}
}
