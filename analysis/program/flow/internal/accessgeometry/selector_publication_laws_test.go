package accessgeometry

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	flowbody "github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
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
		Name:       "accessgeometry-bracket-publication.lua",
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

	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		t.Fatalf("imports.Finalizer: %v", err)
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

	bindings := selectorBindingProof(t, sourceFinalizer.Preimage(), flowFinalizer.View(), staticFinalizer.View(), body)
	bodyResult, bodyErr := flowbody.Seal(sourceFinalizer.Preimage(), flowFinalizer.View(), staticFinalizer.View(), body)
	if bodyErr != nil {
		t.Fatalf("body.Seal: %v", bodyErr)
	}
	_, err = sealSelectors(sourceFinalizer.Preimage(), flowFinalizer.View(), bodyResult, bindings, staticFinalizer.View(), moduleFinalizer.View())
	if err == nil || !strings.Contains(err.Error(), "publication target is not a same-owner name lens") {
		t.Fatalf("Seal error = %v, want bracket-string publication rejection", err)
	}
}

// TestSealSmoke uses the real four owner views and exercises one global
// root, a two-segment exact chain, both direct-call forms, and one valid
// Static publication. The deep chain is covered separately below.
func TestSealSmoke(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	keyA := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	keyB := keyspace.MakeTerm(keyspace.FamilyKey, 2)
	lensA := keyspace.MakeTerm(keyspace.FamilyLensExact, 1)
	lensB := keyspace.MakeTerm(keyspace.FamilyLensExact, 2)
	read1 := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	read2 := keyspace.MakeTerm(keyspace.FamilyRead, 2)
	read3 := keyspace.MakeTerm(keyspace.FamilyRead, 3)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	methodCall := keyspace.MakeTerm(keyspace.FamilyCall, 2)
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	write := keyspace.MakeTerm(keyspace.FamilyWrite, 1)
	typeRef := keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)
	publication := keyspace.MakeTerm(keyspace.FamilyTypePublication, 1)
	var counts [keyspace.FamilyCount]uint32
	for _, term := range []keyspace.Term{body, cell, keyA, keyB, lensA, lensB, read1, read2, read3, values, call, methodCall, assign, write, typeRef, publication} {
		counts[keyspace.TermFamily(term)]++
	}

	input := source.Input{
		Name: "accessgeometry-smoke.lua",
		ExactAtoms: []keyspace.LiteralValue{
			{Kind: keyspace.LiteralString, String: "g"},
			{Kind: keyspace.LiteralString, String: "a"},
			{Kind: keyspace.LiteralString, String: "b"},
		},
		Keys:   []source.KeyInput{source.NameKey(body, "a"), source.NameKey(body, "b")},
		Bodies: []source.BodySource{{Body: body, Terms: []keyspace.Term{assign, call, methodCall}}},
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

	flowInput := authored.Input{
		Counts: counts,
		Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}}},
		Access: authored.AccessInput{Exact: []authored.ExactLens{
			{Owner: body, Base: read1, Source: keyA, Kind: kind.FieldName},
			{Owner: body, Base: read2, Source: keyB, Kind: kind.FieldName},
		}},
		Storage: authored.StorageInput{
			Cells:   []authored.Cell{{Kind: authored.CellGlobal, Key: 3}},
			Reads:   []authored.Read{{Owner: body, Source: cell}, {Owner: body, Source: lensA}, {Owner: body, Source: lensB}},
			Assigns: []authored.Assign{{Owner: body, Values: values}},
			Writes:  []authored.Write{{Assign: assign, Target: lensB}},
		},
		Calls: []authored.Call{
			{Owner: body, Callee: read3, Actuals: values},
			{Owner: body, Callee: read3, Receiver: read2, Actuals: values},
		},
	}
	flowDraft, err := authored.Build(flowInput)
	if err != nil {
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalizer, err := flowDraft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer: %v", err)
	}
	defer flowFinalizer.Abort()

	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		t.Fatalf("imports.Finalizer: %v", err)
	}
	defer moduleFinalizer.Abort()

	staticInput := static.Input{}
	staticInput.Counts[keyspace.FamilyBody] = 1
	staticInput.Counts[keyspace.FamilyCall] = 2
	staticInput.Counts[keyspace.FamilyAssign] = 1
	staticInput.Counts[keyspace.FamilyTypeRef] = 1
	staticInput.Counts[keyspace.FamilyTypePublication] = 1
	staticInput.Contracts.Call = []static.CallContract{{}, {}}
	staticInput.References.TypeRef = []static.TypeRef{{
		Resolution: static.TypeRefCanonicalPath,
		Source:     []keyspace.Key{1}, Canonical: []keyspace.Key{1},
	}}
	staticInput.Publications.Type = []static.Publication{{Assign: assign, Pair: 0, Target: typeRef}}
	staticDraft, err := static.Build(staticInput)
	if err != nil {
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalizer, err := staticDraft.Finalizer()
	if err != nil {
		t.Fatalf("static.Finalizer: %v", err)
	}
	defer staticFinalizer.Abort()

	bindings := selectorBindingProof(t, sourceFinalizer.Preimage(), flowFinalizer.View(), staticFinalizer.View(), body)
	bodyResult, err := flowbody.Seal(sourceFinalizer.Preimage(), flowFinalizer.View(), staticFinalizer.View(), body)
	if err != nil {
		t.Fatalf("body.Seal: %v", err)
	}
	result, err := sealSelectors(sourceFinalizer.Preimage(), flowFinalizer.View(), bodyResult, bindings, staticFinalizer.View(), moduleFinalizer.View())
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	selections := result.ExactReads()
	root, depth, ok := selections.Get(read3)
	if !ok || root != cell || depth != 2 {
		t.Fatalf("selection = %v/%d/%v, want %v/2/true", root, depth, ok, cell)
	}
	rootCursor, ok := selections.PathCursor(read1)
	if !ok {
		t.Fatal("PathCursor rejected exact root with empty suffix")
	}
	if _, _, ok := rootCursor.Segment(); ok {
		t.Fatal("root PathCursor exposed a segment")
	}
	read, form, ok := result.DirectCalls().Get(call)
	if !ok || read != read3 || form != selectorCallPlain {
		t.Fatalf("direct call = %v/%v/%v", read, form, ok)
	}
	methodRead, methodForm, ok := result.DirectCalls().Get(methodCall)
	if !ok || methodRead != read3 || methodForm != selectorCallMethod {
		t.Fatalf("method call = %v/%v/%v", methodRead, methodForm, ok)
	}
	publicationPaths := result.TypePublications()
	publicationRoot, publicationOwner, publicationDepth, ok := publicationPaths.Get(publication)
	if !ok || publicationRoot != cell || publicationOwner != body || publicationDepth != 2 {
		t.Fatalf("publication = %v/%v/%d/%v, want %v/%v/2/true", publicationRoot, publicationOwner, publicationDepth, ok, cell, body)
	}
	publicationCursor, ok := publicationPaths.PathCursor(publication)
	if !ok {
		t.Fatal("publication PathCursor rejected valid path")
	}
	for index, want := range []keyspace.Key{2, 1} {
		got, next, segmentOK := publicationCursor.Segment()
		if !segmentOK || got != want {
			t.Fatalf("publication cursor segment %d = %v/%v, want %v/true", index, got, segmentOK, want)
		}
		publicationCursor = next
	}
	if _, _, ok := publicationCursor.Segment(); ok {
		t.Fatal("publication PathCursor exposed a segment past exact depth")
	}
	allocations := testing.AllocsPerRun(100, func() {
		_, _, _ = selections.Get(read3)
		cursor, _ := selections.PathCursor(read3)
		_, cursor, _ = cursor.Segment()
		_, _, _, _ = publicationPaths.Get(publication)
		publicationCursor, _ := publicationPaths.PathCursor(publication)
		_, publicationCursor, _ = publicationCursor.Segment()
		_, _, _ = result.DirectCalls().Get(call)
	})
	if allocations != 0 {
		t.Fatalf("typed accessgeometry queries allocated %v objects", allocations)
	}

	sourceID := sourceFinalizer.Preimage().Identity().ContentID()
	flowID := flowFinalizer.View().Cold().ContentID()
	staticID := staticFinalizer.View().ContentID()
	moduleID := moduleFinalizer.View().ContentID()
	if !Matches(result, sourceID, flowID, staticID, moduleID) {
		t.Fatal("Matches rejected the four owner identities used by Seal")
	}
	foreign := []struct {
		name string
		id   *identity.ContentID
	}{
		{name: "Source", id: &sourceID},
		{name: "Flow", id: &flowID},
		{name: "Static", id: &staticID},
		{name: "Module", id: &moduleID},
	}
	for _, test := range foreign {
		bad := *test.id
		bad[0] ^= 1
		ids := [4]identity.ContentID{sourceID, flowID, staticID, moduleID}
		switch test.name {
		case "Source":
			ids[0] = bad
		case "Flow":
			ids[1] = bad
		case "Static":
			ids[2] = bad
		case "Module":
			ids[3] = bad
		}
		if Matches(result, ids[0], ids[1], ids[2], ids[3]) {
			t.Fatalf("Matches accepted foreign %s identity", test.name)
		}
	}
}
