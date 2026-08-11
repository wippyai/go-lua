package flow

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	flowbinding "github.com/wippyai/go-lua/program/flow/internal/binding"
	"github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/flow/internal/directbinding"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
)

func TestDirectBindingsPublicCursorTraversesDeepPathLinearlyWithoutAllocation(t *testing.T) {
	const depth = 2048
	bodyTerm := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyCell] = 1
	counts[keyspace.FamilyKey] = depth
	counts[keyspace.FamilyLensExact] = depth
	counts[keyspace.FamilyRead] = depth + 1
	counts[keyspace.FamilyValues] = 1

	keys := make([]source.KeyInput, depth)
	atoms := make([]keyspace.LiteralValue, depth+1)
	atoms[0] = keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "g"}
	for index := 0; index < depth; index++ {
		name := fmt.Sprintf("segment-%05d", index)
		keys[index] = source.NameKey(bodyTerm, name)
		atoms[index+1] = keyspace.LiteralValue{Kind: keyspace.LiteralString, String: name}
	}
	sourceDraft, err := source.Build(source.Input{
		Name:       "public-binding-cursor.lua",
		Families:   assemblyFamilySpans("public-binding-cursor.lua", counts),
		ExactAtoms: atoms,
		Keys:       keys,
		Bodies:     []source.BodySource{{Body: bodyTerm}},
	})
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalizer, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	defer sourceFinalizer.Abort()

	exact := make([]ExactLens, depth)
	reads := make([]Read, depth+1)
	reads[0] = Read{Owner: bodyTerm, Source: cell}
	for index := 0; index < depth; index++ {
		exact[index] = ExactLens{
			Owner:  bodyTerm,
			Base:   keyspace.MakeTerm(keyspace.FamilyRead, uint32(index+1)),
			Source: keyspace.MakeTerm(keyspace.FamilyKey, uint32(index+1)),
			Kind:   kind.FieldName,
		}
		reads[index+1] = Read{
			Owner:  bodyTerm,
			Source: keyspace.MakeTerm(keyspace.FamilyLensExact, uint32(index+1)),
		}
	}
	flowDraft, err := Build(Input{
		Counts: counts,
		Values: ValuesInput{Rows: []Value{{Owner: bodyTerm}}},
		Access: AccessInput{Exact: exact},
		Storage: StorageInput{
			Cells: []Cell{{Kind: authored.CellGlobal, Key: 1}},
			Reads: reads,
		},
	})
	if err != nil {
		t.Fatalf("flow.Build: %v", err)
	}
	flowFinalizer, err := flowDraft.claim()
	if err != nil {
		t.Fatalf("flow.claim: %v", err)
	}
	defer flowFinalizer.Abort()

	staticDraft, err := static.Build(static.Input{})
	if err != nil {
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalizer, err := staticDraft.Finalizer()
	if err != nil {
		t.Fatalf("static.Finalizer: %v", err)
	}
	defer staticFinalizer.Abort()
	moduleDraft, err := module.Build(module.Input{})
	if err != nil {
		t.Fatalf("module.Build: %v", err)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		t.Fatalf("module.Finalizer: %v", err)
	}
	defer moduleFinalizer.Abort()

	preimage, flowView := sourceFinalizer.Preimage(), flowFinalizer.View()
	bodies, err := body.Seal(preimage, flowView, staticFinalizer.View(), bodyTerm)
	if err != nil {
		t.Fatalf("body.Seal: %v", err)
	}
	bindings, err := flowbinding.Seal(preimage, flowView, bodies, bodyTerm)
	if err != nil {
		t.Fatalf("binding.Seal: %v", err)
	}
	result, err := directbinding.Seal(preimage, flowView, bodies, bindings, staticFinalizer.View(), moduleFinalizer.View())
	if err != nil {
		t.Fatalf("directbinding.Seal: %v", err)
	}
	lastRead := keyspace.MakeTerm(keyspace.FamilyRead, depth+1)
	public := DirectBindings{result: result}
	root, gotDepth, ok := public.Selection(lastRead)
	if !ok || root != cell || gotDepth != depth {
		t.Fatalf("Selection = %v/%d/%v; want %v/%d/true", root, gotDepth, ok, cell, depth)
	}
	start, ok := public.SelectionPath(lastRead)
	if !ok {
		t.Fatal("SelectionPath rejected the sealed deep path")
	}
	assertBindingPathLength(t, start, depth)

	allocations := testing.AllocsPerRun(50, func() {
		cursor := start
		for index := 0; index < depth; index++ {
			_, next, segmentOK := cursor.Segment()
			if !segmentOK {
				t.Fatalf("cursor stopped at segment %d", index)
			}
			cursor = next
		}
		if _, _, segmentOK := cursor.Segment(); segmentOK {
			t.Fatal("cursor traversed past its exact depth")
		}
	})
	if allocations != 0 {
		t.Fatalf("public deep cursor allocated %v objects", allocations)
	}
}

func assertBindingPathLength(t *testing.T, cursor BindingPath, depth int) {
	t.Helper()
	for index := 0; index < depth; index++ {
		_, next, ok := cursor.Segment()
		if !ok {
			t.Fatalf("cursor stopped after %d of %d segments", index, depth)
		}
		cursor = next
	}
	if _, _, ok := cursor.Segment(); ok {
		t.Fatalf("cursor exposed more than %d segments", depth)
	}
}
