package factapply

import (
	"context"
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestPathEqualityFactorProgramMatchesConcreteRootEquality(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(1)
	leftSymbol, rightSymbol := symbol.ID(904), symbol.ID(905)
	builder := visibility.NewBuilder()
	builder.Define(point, leftSymbol, "path-equality-left")
	builder.Define(point, rightSymbol, "path-equality-right")
	resolver := visibility.NewResolver(builder.Build())
	leftPath, rightPath := pathdom.Path{Symbol: leftSymbol}, pathdom.Path{Symbol: rightSymbol}
	left, leftOK := factKeyspaceKeyAt(resolver, point, leftPath)
	right, rightOK := factKeyspaceKeyAt(resolver, point, rightPath)
	if !leftOK || !rightOK {
		t.Fatal("fixture roots are unresolved")
	}
	input := state.Reachable(state.State{}).
		WriteValue(reg, statekey.SymbolValue(leftSymbol), product.Top()).
		WriteValue(reg, statekey.SymbolValue(rightSymbol), presentValue(reg))
	program, frame := prepareConcretePathEqualityFactorTest(t, domain, resolver, left, right, input)
	gotFrame, err := program.Apply(context.Background(), nil, frame)
	if err != nil {
		t.Fatal(err)
	}
	got := composeFactorFrameTest(t, domain, input, program.Lanes(), gotFrame)
	want := presentValue(reg)
	if leftValue := got.ReadValue(reg, statekey.SymbolValue(leftSymbol)); !product.Equal(reg, leftValue, want) {
		t.Fatalf("left equality value = %s, want %s", formatValue(reg, leftValue), formatValue(reg, want))
	}
	if rightValue := got.ReadValue(reg, statekey.SymbolValue(rightSymbol)); !product.Equal(reg, rightValue, want) {
		t.Fatalf("right equality value = %s, want %s", formatValue(reg, rightValue), formatValue(reg, want))
	}
}

func prepareConcretePathEqualityFactorTest(
	t *testing.T,
	domain state.ProductDomain,
	resolver *visibility.Resolver,
	left, right keyspace.Key,
	input state.State,
) (PathEqualityFactorProgram[statekey.Value], ValueRefinementFactorFrame[statekey.Value]) {
	t.Helper()
	residual, values := state.DecomposeValueLane(domain.Lattice(), input)
	family, present := domain.PathEvidenceCoordinateFamily()
	if !present {
		t.Fatal("path evidence coordinate family is absent")
	}
	pathFactors, err := domain.DecomposeLanes(residual, []state.ProductLane{family.Lane()})
	if err != nil {
		t.Fatal(err)
	}
	skeleton, scalars, err := domain.DecomposeCoordinateFamily(pathFactors[0], family, resolver.KeySpace())
	if err != nil {
		t.Fatal(err)
	}
	union := make([]state.CoordinateSlot, len(scalars))
	for index := range scalars {
		union[index] = scalars[index].Slot()
	}
	plan, err := domain.SealPathEqualityFactorPlan(resolver.KeySpace(), left, right, union)
	if err != nil {
		t.Fatal(err)
	}
	program, err := PreparePathEqualityFactorProgram(
		domain, plan,
		func(dependency statekey.ValueDependency) (statekey.Value, bool) { return dependency.Concrete() },
		typevalue.NewCache(),
	)
	if err != nil {
		t.Fatal(err)
	}
	factors, err := domain.DecomposeLanes(residual, program.Lanes())
	if err != nil {
		t.Fatal(err)
	}
	mutation, err := domain.DecomposePathDescendantMutationFactors(residual, resolver.KeySpace())
	if err != nil {
		t.Fatal(err)
	}
	carrier, err := domain.OpenCoordinatePathEvidenceCarrier(
		skeleton, scalars, state.ValueLaneFactor{}, true, program.PathEvidenceAuthority(), mutation,
	)
	if err != nil {
		t.Fatal(err)
	}
	return program, ValueRefinementFactorFrame[statekey.Value]{
		Values: values, Factors: factors, Carrier: carrier, Reachable: true,
	}
}
