package directbinding

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	flowbody "github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
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
		Name:       "directbinding-deep.lua",
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
	defer sourceFinalizer.Abort()

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

	staticDraft, err := static.Build(static.Input{})
	if err != nil {
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalizer, err := staticDraft.Finalizer()
	if err != nil {
		t.Fatalf("static.Finalizer: %v", err)
	}
	defer staticFinalizer.Abort()

	bindings := directBindingProof(t, sourceFinalizer.Preimage(), flowFinalizer.View(), staticFinalizer.View(), body)
	bodyResult, err := flowbody.Seal(sourceFinalizer.Preimage(), flowFinalizer.View(), staticFinalizer.View(), body)
	if err != nil {
		t.Fatalf("body.Seal: %v", err)
	}
	result, err := Seal(sourceFinalizer.Preimage(), flowFinalizer.View(), bodyResult, bindings, staticFinalizer.View(), moduleFinalizer.View())
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	lastRead := keyspace.MakeTerm(keyspace.FamilyRead, depth+1)
	selections := result.BindingSelections()
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
