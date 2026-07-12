package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestDirectCallCatalogOwnsPointIdentityAndDependencies(t *testing.T) {
	first := CellRef{Function: 8}
	second := CellRef{Function: 3, Slot: 1}
	input := map[cfg.Point]DirectCallTarget{
		4: {Cell: first, Shape: Shape{Params: 2}},
		1: {Cell: second, Shape: Shape{Params: 1}},
		2: {Cell: first, Shape: Shape{Params: 2}},
	}
	catalog, err := NewDirectCallCatalog(5, input)
	if err != nil {
		t.Fatal(err)
	}
	delete(input, 4)
	if got, ok := catalog.Lookup(4); !ok || got.Cell != first || got.Shape.Params != 2 {
		t.Fatalf("owned point target = %#v/%v", got, ok)
	}
	dependencies := catalog.Cells()
	if len(dependencies) != 2 || dependencies[0] != second || dependencies[1] != first {
		t.Fatalf("canonical dependencies = %#v", dependencies)
	}
	if _, err := NewDirectCallCatalog(1, map[cfg.Point]DirectCallTarget{1: {Cell: first}}); err == nil {
		t.Fatal("out-of-range catalog point accepted")
	}
}

func TestComposeDirectCallRowsPreservesCorrelationAndConsumesCalleeResultMetadata(t *testing.T) {
	reg := standard.Registry()
	calleeShape := Shape{Params: 1}
	calleeArena := NewArena(reg)
	param := calleeArena.Root(Root{Kind: RootParam, Index: 0})
	leftProduct := typevalue.LiteralString(reg, "left")
	rightProduct := typevalue.LiteralString(reg, "right")
	nilProduct := typevalue.Nil(reg)
	left := calleeArena.Constant(leftProduct)
	right := calleeArena.Constant(rightProduct)
	nilValue := calleeArena.Constant(nilProduct)
	proofPath := pathdom.NewPlaceholder(0).Field("ready")
	rowOutput := summary.Summary{
		NormalReturnParams: []product.Value{product.Top()},
		NormalReturnFacts: callboundary.NormalReturnFacts{BranchProofs: []callboundary.BranchProof{{
			Kind: pathevidence.BranchProofPathPresence, Path: proofPath, Presence: presence.Present(),
		}}},
		// These are deliberately malformed payload entries: composition consumes
		// the metadata by row identity and must never copy either slice outward.
		ReturnConditionSlotRefinements: []summary.ReturnConditionSlotRefinement{{}},
		ReturnPresenceRelations:        []summary.ReturnPresenceRelation{{}},
	}
	callee := Relation{
		shape: calleeShape, arena: calleeArena, effects: NewEffectArena(calleeArena), descriptors: DefaultDescriptorRegistry(),
		rows: []Row{
			{Guard: calleeArena.Truthy(param), Output: rowOutput, Ops: []Operation{
				{Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 0, Value: left},
				{Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 1, Value: nilValue},
			}},
			{Guard: calleeArena.Falsy(param), Output: rowOutput, Ops: []Operation{
				{Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 0, Value: nilValue},
				{Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 1, Value: right},
			}},
		},
	}
	callerShape := Shape{Params: 1}
	plan := operationplan.New(cfg.New(), factflow.FactsInput{})
	callerBuilder := NewBuilder(reg, callerShape, DefaultOutputCapabilityRegistry(), plan)
	callerParam := callerBuilder.Arena().Root(Root{Kind: RootParam, Index: 0})
	targetValue, targetErr := symbol.ID(41), symbol.ID(42)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Final: true, Expanded: true,
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, targetValue, pathdom.NewPath(targetValue, "value")),
			factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 1, 1, targetErr, pathdom.NewPath(targetErr, "err")),
		},
	}).View()
	rows, err := ComposeDirectCallRows(callerBuilder, callerShape, SymbolicCFGRow{
		Guard: callerBuilder.Arena().True(), Values: map[symbol.ID]ValueTerm{1: callerParam},
	}, callee, DirectCallBindings{
		Values: []ValueTerm{callerParam}, Paths: []PathTerm{callerBuilder.Arena().Path(Root{Kind: RootParam, Index: 0})},
	}, site, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("composed rows = %d, want two correlated alternatives", len(rows))
	}
	pairs := make(map[string]bool)
	for _, row := range rows {
		if len(row.Output.ReturnConditionSlotRefinements) != 0 || len(row.Output.ReturnPresenceRelations) != 0 {
			t.Fatalf("callee result correlation leaked into caller output: %#v", row.Output)
		}
		if len(row.Proofs) != 1 || row.Proofs[0].Key != 0 || !row.Proofs[0].valid(callerBuilder.Arena(), callerShape) {
			t.Fatalf("rebased static proof = %#v", row.Proofs)
		}
		cursor, _ := NewBindingCursor(callerShape, []product.Value{typevalue.LiteralBool(reg, true)}, nil)
		value, valueOK := callerBuilder.Arena().evalValue(row.Values[targetValue], cursor, SpecializationContext{})
		errValue, errOK := callerBuilder.Arena().evalValue(row.Values[targetErr], cursor, SpecializationContext{})
		if !valueOK || !errOK {
			t.Fatal("rebased result terms did not specialize")
		}
		switch {
		case product.Equal(reg, value, leftProduct) && product.Equal(reg, errValue, nilProduct):
			pairs["left/nil"] = true
		case product.Equal(reg, value, nilProduct) && product.Equal(reg, errValue, rightProduct):
			pairs["nil/right"] = true
		default:
			t.Fatalf("unexpected correlated return pair: %#v / %#v", value, errValue)
		}
	}
	if len(pairs) != 2 {
		t.Fatalf("return pairs lost row correlation: %#v", pairs)
	}
}

func TestComposeDirectCallRowsFailsClosedWithoutCallerBoundaryProofPath(t *testing.T) {
	reg := standard.Registry()
	calleeArena := NewArena(reg)
	value := calleeArena.Constant(typevalue.Nil(reg))
	callee := Relation{shape: Shape{Params: 1}, arena: calleeArena, effects: NewEffectArena(calleeArena), descriptors: DefaultDescriptorRegistry(), rows: []Row{{
		Guard: calleeArena.True(),
		Output: summary.Summary{NormalReturnParams: []product.Value{product.Top()}, NormalReturnFacts: callboundary.NormalReturnFacts{BranchProofs: []callboundary.BranchProof{{
			Kind: pathevidence.BranchProofPathPresence, Path: pathdom.NewPlaceholder(0), Presence: presence.Present(),
		}}}},
		Ops: []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 0, Value: value}},
	}}}
	plan := operationplan.New(cfg.New(), factflow.FactsInput{})
	caller := NewBuilder(reg, Shape{Params: 1, Globals: 1}, DefaultOutputCapabilityRegistry(), plan)
	param := caller.Arena().Root(Root{Kind: RootParam, Index: 0})
	globalPath := caller.Arena().Path(Root{Kind: RootGlobal, Index: 0})
	target := symbol.ID(50)
	site := factflow.NewCallSite(factflow.CallSiteConfig{Final: true, Expanded: true, ResultTargets: []factflow.CallResultTarget{
		factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, target, pathdom.NewPath(target, "target")),
	}}).View()
	if rows, err := ComposeDirectCallRows(caller, Shape{Params: 1, Globals: 1}, SymbolicCFGRow{Guard: caller.Arena().True(), Values: map[symbol.ID]ValueTerm{}}, callee, DirectCallBindings{Values: []ValueTerm{param}, Paths: []PathTerm{globalPath}}, site, 4); err == nil || len(rows) != 0 {
		t.Fatalf("non-boundary proof path composed: rows=%#v err=%v", rows, err)
	}
}
