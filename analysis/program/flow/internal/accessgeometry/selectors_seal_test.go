package accessgeometry

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	flowbinding "github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	flowbody "github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

// TestSealDeepExactChain keeps the fixture deliberately mechanical: every
// Read after the global root is one same-Body FieldName lens. It is large
// enough to make a recursive selector fail in practice while keeping the
// assertion surface narrow and the query buffer caller-owned.
func TestSealDeepExactChain(t *testing.T) {
	const depth = 4096
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)

	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyCell] = 1
	counts[keyspace.FamilyKey] = depth
	counts[keyspace.FamilyLensExact] = depth
	counts[keyspace.FamilyRead] = depth + 1
	counts[keyspace.FamilyValues] = 1

	keys := make([]source.KeyInput, depth)
	exactAtoms := make([]keyspace.LiteralValue, depth+1)
	exactAtoms[0] = keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "g"}
	for index := 0; index < depth; index++ {
		keys[index] = source.NameKey(body, fmt.Sprintf("z%05d", index))
		exactAtoms[index+1] = keyspace.LiteralValue{Kind: keyspace.LiteralString, String: fmt.Sprintf("z%05d", index)}
	}

	input := source.Input{
		Name:       "accessgeometry-deep.lua",
		ExactAtoms: exactAtoms,
		Keys:       keys,
		Bodies:     []source.BodySource{{Body: body}},
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, counts[family])
		for index := range spans {
			line := uint32(index + 1)
			spans[index] = source.Span{
				File: input.Name, StartLine: line, StartCol: 1,
				EndLine: line, EndCol: 1,
			}
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
	defer func() { _ = sourceFinalizer.Abort() }()

	exact := make([]authored.ExactLens, depth)
	reads := make([]authored.Read, depth+1)
	reads[0] = authored.Read{Owner: body, Source: cell}
	for index := 0; index < depth; index++ {
		lens := keyspace.MakeTerm(keyspace.FamilyLensExact, uint32(index+1))
		base := keyspace.MakeTerm(keyspace.FamilyRead, uint32(index+1))
		exact[index] = authored.ExactLens{
			Owner: body, Base: base,
			Source: keyspace.MakeTerm(keyspace.FamilyKey, uint32(index+1)),
			Kind:   kind.FieldName,
		}
		reads[index+1] = authored.Read{Owner: body, Source: lens}
	}
	flowDraft, err := authored.Build(authored.Input{
		Counts: counts,
		Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}}},
		Access: authored.AccessInput{Exact: exact},
		Storage: authored.StorageInput{
			Cells: []authored.Cell{{Kind: authored.CellGlobal, Key: 1}},
			Reads: reads,
		},
	})
	if err != nil {
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalizer, err := flowDraft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer: %v", err)
	}
	defer func() { _ = flowFinalizer.Abort() }()

	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		t.Fatalf("imports.Finalizer: %v", err)
	}
	defer func() { _ = moduleFinalizer.Abort() }()

	staticDraft, err := static.Build(static.Input{})
	if err != nil {
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalizer, err := staticDraft.Finalizer()
	if err != nil {
		t.Fatalf("static.Finalizer: %v", err)
	}
	defer func() { _ = staticFinalizer.Abort() }()

	bindings := selectorBindingProof(t, sourceFinalizer.Preimage(), flowFinalizer.View(), staticFinalizer.View(), body)
	bodyResult, err := flowbody.Seal(sourceFinalizer.Preimage(), flowFinalizer.View(), staticFinalizer.View(), body)
	if err != nil {
		t.Fatalf("body.Seal: %v", err)
	}
	result, err := sealSelectors(sourceFinalizer.Preimage(), flowFinalizer.View(), bodyResult, bindings, staticFinalizer.View(), moduleFinalizer.View())
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	lastRead := keyspace.MakeTerm(keyspace.FamilyRead, depth+1)
	selections := result.ExactReads()
	root, gotDepth, ok := selections.Get(lastRead)
	if !ok || root != cell || gotDepth != depth {
		t.Fatalf("deep selection = %v/%d/%v, want %v/%d/true", root, gotDepth, ok, cell, depth)
	}

	want := make([]keyspace.Key, depth)
	for index := range want {
		keyTerm := keyspace.MakeTerm(keyspace.FamilyKey, uint32(index+1))
		_, _, key, ok := sourceFinalizer.Preimage().Keys().Name(keyTerm)
		if !ok {
			t.Fatalf("source key %v unavailable", keyTerm)
		}
		want[index] = key
	}
	// PathCursor walks the same sealed parent chain from leaf to root one edge
	// at a time. The extra Segment after the exact depth proves termination
	// rather than merely trusting the depth returned by Get.
	cursor, ok := selections.PathCursor(lastRead)
	if !ok {
		t.Fatal("PathCursor rejected deep selection")
	}
	startCursor := cursor
	for index := 0; index < depth; index++ {
		got, next, segmentOK := cursor.Segment()
		wantKey := want[depth-index-1]
		if !segmentOK || got != wantKey {
			t.Fatalf("cursor segment %d = %v/%v, want %v/true", index, got, segmentOK, wantKey)
		}
		cursor = next
	}
	if _, _, ok := cursor.Segment(); ok {
		t.Fatal("PathCursor exposed a segment past exact depth")
	}

	allocations := testing.AllocsPerRun(100, func() {
		current := startCursor
		for index := 0; index < depth; index++ {
			_, next, segmentOK := current.Segment()
			if !segmentOK {
				t.Fatalf("allocation probe cursor segment %d failed", index)
			}
			current = next
		}
		if _, _, ok := current.Segment(); ok {
			t.Fatal("allocation probe cursor did not terminate")
		}
	})
	if allocations != 0 {
		t.Fatalf("deep PathCursor allocated %v objects", allocations)
	}
}

func TestSealReadLocalCellUsesBodyAncestry(t *testing.T) {
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	child := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	sibling := keyspace.MakeTerm(keyspace.FamilyBody, 3)

	if err := sealReadCase(t, entry, child); err != nil {
		t.Fatalf("descendant Read rejected: %v", err)
	}
	if err := sealReadCase(t, child, sibling); err == nil ||
		!strings.Contains(err.Error(), "local Cell owner disagrees with Read") {
		t.Fatalf("sibling Read error = %v, want local-cell ancestry rejection", err)
	}
}

func sealReadCase(t *testing.T, cellBody, readOwner keyspace.Term) error {
	t.Helper()
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	child := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	sibling := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	loopOne := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	loopTwo := keyspace.MakeTerm(keyspace.FamilyLoop, 2)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	nilOne := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	nilTwo := keyspace.MakeTerm(keyspace.FamilyNil, 2)

	var counts [keyspace.FamilyCount]uint32
	for _, term := range []keyspace.Term{
		entry, child, sibling, cell, bind, values, loopOne, loopTwo, read, nilOne, nilTwo,
	} {
		counts[keyspace.TermFamily(term)]++
	}

	name := "accessgeometry-descendant-read.lua"
	input := source.Input{
		Name:  name,
		Nil:   []source.NilLiteral{{Owner: entry}, {Owner: entry}},
		Binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		Bodies: []source.BodySource{
			{Body: entry, Terms: []keyspace.Term{loopOne, loopTwo}},
			{Body: child},
			{Body: sibling},
		},
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, counts[family])
		for index := range spans {
			line := uint32(index + 1)
			spans[index] = source.Span{File: name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		input.Families = append(input.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	if cellBody != entry {
		input.Bodies[0].Terms = []keyspace.Term{loopOne, loopTwo}
		input.Bodies[1].Terms = []keyspace.Term{bind}
	} else {
		input.Bodies[0].Terms = []keyspace.Term{bind, loopOne, loopTwo}
	}

	sourceDraft, err := source.Build(input)
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalizer, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	defer func() { _ = sourceFinalizer.Abort() }()

	flowDraft, err := authored.Build(authored.Input{
		Counts: counts,
		Values: authored.ValuesInput{Rows: []authored.Value{{Owner: cellBody}}},
		Storage: authored.StorageInput{
			Cells: []authored.Cell{{Kind: authored.CellLocal, Body: cellBody}},
			Reads: []authored.Read{{Owner: readOwner, Source: cell}},
			Binds: []authored.Bind{{Owner: cellBody, Values: values}},
		},
		Control: authored.ControlInput{
			Loops: []authored.Loop{
				{Owner: entry, Body: child, Kind: kind.LoopWhile, Control: nilOne},
				{Owner: entry, Body: sibling, Kind: kind.LoopWhile, Control: nilTwo},
			},
		},
	})
	if err != nil {
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalizer, err := flowDraft.Finalizer()
	if err != nil {
		t.Fatalf("flow.Finalizer: %v", err)
	}
	defer func() { _ = flowFinalizer.Abort() }()

	staticDraft, err := static.Build(static.Input{})
	if err != nil {
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalizer, err := staticDraft.Finalizer()
	if err != nil {
		t.Fatalf("static.Finalizer: %v", err)
	}
	defer func() { _ = staticFinalizer.Abort() }()

	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		t.Fatalf("imports.Finalizer: %v", err)
	}
	defer func() { _ = moduleFinalizer.Abort() }()

	preimage, flowView, staticView := sourceFinalizer.Preimage(), flowFinalizer.View(), staticFinalizer.View()
	bodies, err := flowbody.Seal(preimage, flowView, staticView, entry)
	if err != nil {
		t.Fatalf("body.Seal: %v", err)
	}
	bindings, err := flowbinding.Seal(preimage, flowView, bodies, entry)
	if err != nil {
		t.Fatalf("binding.Seal: %v", err)
	}
	_, err = sealSelectors(preimage, flowView, bodies, bindings, staticView, moduleFinalizer.View())
	return err
}

func TestSealIgnoresExactLensOverScalarBase(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	stringTerm := keyspace.MakeTerm(keyspace.FamilyString, 1)
	key := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	lens := keyspace.MakeTerm(keyspace.FamilyLensExact, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyString: 1, keyspace.FamilyKey: 1,
		keyspace.FamilyLensExact: 1, keyspace.FamilyRead: 1, keyspace.FamilyValues: 1,
	}
	input := source.Input{
		Name:       "accessgeometry-scalar-lens.lua",
		ExactAtoms: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "upper"}},
		String:     []source.StringLiteral{{Owner: body, Value: "hello"}},
		Keys:       []source.KeyInput{source.NameKey(body, "upper")},
		Bodies:     []source.BodySource{{Body: body}},
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
	defer func() { _ = sourceFinalizer.Abort() }()
	flowDraft, err := authored.Build(authored.Input{
		Counts: counts,
		Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}}},
		Access: authored.AccessInput{Exact: []authored.ExactLens{{
			Owner: body, Base: stringTerm, Source: key, Kind: kind.FieldName,
		}}},
		Storage: authored.StorageInput{Reads: []authored.Read{{Owner: body, Source: lens}}},
	})
	if err != nil {
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalizer, err := flowDraft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer: %v", err)
	}
	defer func() { _ = flowFinalizer.Abort() }()
	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		t.Fatalf("imports.Finalizer: %v", err)
	}
	defer func() { _ = moduleFinalizer.Abort() }()
	staticDraft, err := static.Build(static.Input{})
	if err != nil {
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalizer, err := staticDraft.Finalizer()
	if err != nil {
		t.Fatalf("static.Finalizer: %v", err)
	}
	defer func() { _ = staticFinalizer.Abort() }()
	flowView := flowFinalizer.View()
	bindings := selectorBindingProof(t, sourceFinalizer.Preimage(), flowView, staticFinalizer.View(), body)
	bodyResult, err := flowbody.Seal(sourceFinalizer.Preimage(), flowView, staticFinalizer.View(), body)
	if err != nil {
		t.Fatalf("body.Seal: %v", err)
	}
	result, err := sealSelectors(sourceFinalizer.Preimage(), flowView, bodyResult, bindings, staticFinalizer.View(), moduleFinalizer.View())
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if _, _, ok := result.ExactReads().Get(read); ok {
		t.Fatal("scalar-base exact Lens unexpectedly produced an exact-read selector")
	}
}

func selectorBindingProof(t *testing.T, preimage source.Preimage, flow authored.View, staticView static.View, entry keyspace.Term) flowbinding.Result {
	t.Helper()
	bodyResult, err := flowbody.Seal(preimage, flow, staticView, entry)
	if err != nil {
		t.Fatalf("flowbody.Seal: %v", err)
	}
	result, err := flowbinding.Seal(preimage, flow, bodyResult, entry)
	if err != nil {
		t.Fatalf("binding.Seal: %v", err)
	}
	return result
}

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
	defer func() { _ = sourceFinalizer.Abort() }()

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
	defer func() { _ = flowFinalizer.Abort() }()

	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		t.Fatalf("imports.Finalizer: %v", err)
	}
	defer func() { _ = moduleFinalizer.Abort() }()

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
	defer func() { _ = staticFinalizer.Abort() }()

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
	defer func() { _ = sourceFinalizer.Abort() }()

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
	defer func() { _ = flowFinalizer.Abort() }()

	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		t.Fatalf("imports.Finalizer: %v", err)
	}
	defer func() { _ = moduleFinalizer.Abort() }()

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
	defer func() { _ = staticFinalizer.Abort() }()

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
		_, _, _ = cursor.Segment()
		_, _, _, _ = publicationPaths.Get(publication)
		publicationCursor, _ := publicationPaths.PathCursor(publication)
		_, _, _ = publicationCursor.Segment()
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
