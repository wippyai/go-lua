package factapply

import (
	"context"
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestCallResultPostconditionFactorProgramAppliesRefinementEqualityPresence(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(1911)
	const leftID, rightID, targetID = symbol.ID(1911), symbol.ID(1912), symbol.ID(1913)
	left := pathdom.NewPath(leftID, "left")
	right := pathdom.NewPath(rightID, "right")
	target := pathdom.NewPath(targetID, "target")
	builder := visibility.NewBuilder()
	builder.Define(point, leftID, "left")
	builder.Define(point, rightID, "right")
	builder.Define(point, targetID, "target")
	resolver := visibility.NewResolver(builder.Build())
	authority := NewPathSemanticAuthority(resolver, nil, nil)
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	transaction := PlanCallResultTransaction(factflow.NewFacts(factflow.FactsInput{
		PostconditionRefinements: map[cfg.Point]factflow.PostconditionRefinementSet{
			point: factflow.NewPostconditionRefinementSet(
				factflow.NewPostconditionRefinement(left, factflow.NewValueConstraint(present)),
			),
		},
		PostconditionPathRelations: map[cfg.Point][]factflow.PostconditionPathRelation{
			point: {factflow.NewPostconditionPathEquality(left, right)},
		},
	}), point)
	leftKey, leftOK := visibility.AddressAt(resolver, point, left).RootOrVisibleKeyspaceKey()
	targetKey, targetOK := visibility.AddressAt(resolver, point, target).RootOrVisibleKeyspaceKey()
	if !leftOK || !targetOK {
		t.Fatal("presence endpoints")
	}
	row := pathevidence.NewPathPresenceImplication(leftKey, presence.Present(), targetKey, presence.Present())
	input := state.Reachable(domain.Lattice().Bottom()).
		WriteValue(reg, statekey.SymbolValue(leftID), product.Top()).
		WriteValue(reg, statekey.SymbolValue(rightID), present).
		WriteValue(reg, statekey.SymbolValue(targetID), product.Top()).
		AddPathPresenceImplication(row)

	program, frame := prepareCallResultPostconditionFactorTest(t, authority, domain, transaction, input)
	gotFrame, err := program.Apply(context.Background(), nil, frame)
	if err != nil {
		t.Fatal(err)
	}
	got := composeCallResultPostconditionFactorTest(t, domain, input, program, gotFrame)
	if !presence.Equal(product.PresenceOf(got.ReadValue(reg, statekey.SymbolValue(leftID))), presence.Present()) ||
		!presence.Equal(product.PresenceOf(got.ReadValue(reg, statekey.SymbolValue(targetID))), presence.Present()) {
		t.Fatal("N3 did not apply its declared refinement/equality/presence semantics")
	}
	programLanes := make(map[state.LaneID]struct{}, len(program.Lanes()))
	programLanes[state.LaneValues] = struct{}{}
	for _, lane := range program.Lanes() {
		programLanes[lane.ID()] = struct{}{}
	}
	for _, lane := range domain.LaneInventory() {
		if _, owned := programLanes[lane.ID()]; owned {
			continue
		}
		before, beforeErr := domain.DecomposeLanes(input, []state.ProductLane{lane})
		after, afterErr := domain.DecomposeLanes(got, []state.ProductLane{lane})
		if beforeErr != nil || afterErr != nil || len(before) != 1 || len(after) != 1 {
			t.Fatalf("residual lane %s decomposition failed", lane.ID())
		}
		equal, equalErr := domain.LaneEqual(before[0], after[0])
		if equalErr != nil || !equal {
			t.Fatalf("residual lane %s changed: equal=%t err=%v", lane.ID(), equal, equalErr)
		}
	}
}

func TestCallResultPostconditionFactorProgramCancellationIsAtomic(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(1912)
	const targetID = symbol.ID(1921)
	target := pathdom.NewPath(targetID, "target")
	builder := visibility.NewBuilder()
	builder.Define(point, targetID, "target")
	resolver := visibility.NewResolver(builder.Build())
	authority := NewPathSemanticAuthority(resolver, nil, nil)
	transaction := PlanCallResultTransaction(factflow.NewFacts(factflow.FactsInput{
		PostconditionRefinements: map[cfg.Point]factflow.PostconditionRefinementSet{
			point: factflow.NewPostconditionRefinementSet(factflow.NewPostconditionRefinement(
				target, factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Present())),
			)),
		},
	}), point)
	input := state.Reachable(domain.Lattice().Bottom()).WriteValue(reg, statekey.SymbolValue(targetID), product.Top())
	program, frame := prepareCallResultPostconditionFactorTest(t, authority, domain, transaction, input)
	ctx, session := cancellation.Attach(context.Background())
	session.Token().Cancel(context.Canceled)
	got, err := program.Apply(ctx, session.Token(), frame)
	if err == nil {
		t.Fatal("pre-canceled N3 factor program succeeded")
	}
	if !valueFactorEqual(reg, got.Values, frame.Values) || got.Reachable != frame.Reachable || len(got.Factors) != len(frame.Factors) {
		t.Fatal("canceled N3 factor program changed its frame")
	}
	for index := range frame.Factors {
		equal, equalErr := domain.LaneEqual(got.Factors[index], frame.Factors[index])
		if equalErr != nil || !equal {
			t.Fatalf("canceled N3 changed factor %d", index)
		}
	}
}

func prepareCallResultPostconditionFactorTest(
	t *testing.T,
	authority *PathSemanticAuthority,
	domain state.ProductDomain,
	transaction CallResultTransaction,
	input state.State,
) (CallResultPostconditionFactorProgram[statekey.Value], CallResultPostconditionFactorFrame[statekey.Value]) {
	t.Helper()
	seed, err := authority.CoordinateFactorInventoryFromPreparedState(domain, input)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := authority.CloseCoordinateFactorInventory(domain, seed)
	if err != nil {
		t.Fatal(err)
	}
	program, err := PrepareCallResultPostconditionFactorProgram(
		authority, domain, transaction, inventory,
		func(dependency statekey.ValueDependency) (statekey.Value, bool) { return dependency.Concrete() }, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	residual, values := state.DecomposeValueLane(domain.Lattice(), input)
	factors, err := domain.DecomposeLanes(residual, program.Lanes())
	if err != nil {
		t.Fatal(err)
	}
	return program, CallResultPostconditionFactorFrame[statekey.Value]{
		Values: values, Factors: factors, Reachable: !domain.Lattice().Equal(input, domain.Lattice().Bottom()),
	}
}

func composeCallResultPostconditionFactorTest(
	t *testing.T,
	domain state.ProductDomain,
	input state.State,
	program CallResultPostconditionFactorProgram[statekey.Value],
	frame CallResultPostconditionFactorFrame[statekey.Value],
) state.State {
	t.Helper()
	return composeFactorFrameTest(t, domain, input, program.Lanes(), ValueRefinementFactorFrame[statekey.Value]{
		Values: frame.Values, Factors: frame.Factors, Reachable: frame.Reachable,
	})
}

func valueFactorEqual[K comparable](reg *axis.Registry, left, right state.ValueFactor[K]) bool {
	return state.ValueFactorLattice[K](reg).Equal(left, right)
}
