package operationplan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPlanOwnsImmutableTypedGenericForPayload(t *testing.T) {
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeAssign)
	path := pathdom.Path{Symbol: 7, Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "items"}}}
	contracts := []typ.Type{typ.String}
	op, ok := NewGenericForOperation(1, symbol.ID(9), symbol.ID(8), []GenericForSource{{Kind: GenericForSourceExpression, RootPath: path, HasRootPath: true}}, contracts)
	if !ok {
		t.Fatal("valid generic-for operation rejected")
	}
	iterator := iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateKeyed}
	op = op.WithIterator(iterator)
	plan := New(graph, factflow.FactsInput{}).WithExtensions([]ExtensionInput{{Point: point, Kind: BodyGenericFor, GenericFor: op}})
	path.Segments[0].Name = "mutated"
	contracts[0] = typ.Number

	got, ok := plan.GenericForOperation(point)
	if !ok {
		t.Fatal("typed generic-for payload missing")
	}
	if got.Target() != 9 || got.FirstTarget() != 8 || got.VariableIndex() != 1 {
		t.Fatalf("payload identity = %#v", got)
	}
	if source, _ := got.ProtocolSource(0); source.RootPath.Segments[0].Name != "items" {
		t.Fatalf("plan retained caller path: %#v", source)
	}
	if contract, ok := got.SourceContract(0); !ok || !typ.TypeEquals(contract, typ.String) {
		t.Fatalf("contract = %v/%v", contract, ok)
	}
	if gotIterator, ok := got.Iterator(); !ok || gotIterator != iterator {
		t.Fatalf("iterator = %#v/%v", gotIterator, ok)
	}

	returned, _ := got.ProtocolSource(0)
	returned.RootPath.Segments[0].Name = "again"
	gotAgain, _ := plan.GenericForOperation(point)
	if source, _ := gotAgain.ProtocolSource(0); source.RootPath.Segments[0].Name != "items" {
		t.Fatal("payload getter exposed plan storage")
	}
}

func TestGenericForMarkerWithoutTypedPayloadFailsClosed(t *testing.T) {
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeAssign)
	plan := New(graph, factflow.FactsInput{}).WithExtensions([]ExtensionInput{{Point: point, Kind: BodyGenericFor}})
	if !plan.HasExtensions() {
		t.Fatal("marker disappeared")
	}
	if _, ok := plan.GenericForOperation(point); ok {
		t.Fatal("marker invented a zero payload")
	}
}

func TestConflictingGenericForPayloadsFailClosedIndependentOfOrder(t *testing.T) {
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeAssign)
	one, _ := NewGenericForOperation(0, 8, 8, nil, nil)
	two, _ := NewGenericForOperation(1, 9, 8, nil, nil)
	for _, input := range [][]ExtensionInput{
		{{Point: point, Kind: BodyGenericFor, GenericFor: one}, {Point: point, Kind: BodyGenericFor, GenericFor: two}},
		{{Point: point, Kind: BodyGenericFor, GenericFor: two}, {Point: point, Kind: BodyGenericFor, GenericFor: one}},
	} {
		plan := New(graph, factflow.FactsInput{}).WithExtensions(input)
		if _, ok := plan.GenericForOperation(point); ok {
			t.Fatal("conflicting typed payload published")
		}
	}
}
