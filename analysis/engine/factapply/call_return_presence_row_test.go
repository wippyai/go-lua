package factapply

import (
	"context"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestCallReturnPresenceRowsJoinToCrossResultCorrelation(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(31)
	valueSymbol, errorSymbol := symbol.ID(41), symbol.ID(42)
	builder := visibility.NewBuilder()
	builder.Define(point, valueSymbol, "value")
	builder.Define(point, errorSymbol, "err")
	resolver := visibility.NewResolver(builder.Build())
	authority := NewPathSemanticAuthority(resolver, nil, nil)
	valueKey := resolver.KeySpace().FromPath(pathdom.NewPath(valueSymbol, "value"))
	errorKey := resolver.KeySpace().FromPath(pathdom.NewPath(errorSymbol, "err"))
	returnValueKey := resolver.KeySpace().FromPath(pathdom.Path{Root: "ret[0]"})
	returnErrorKey := resolver.KeySpace().FromPath(pathdom.Path{Root: "ret[1]"})
	present, absent := product.NewWithPresence(reg, product.ShapeTop, presence.Present()), product.Absent(reg)
	row := func(valuePresence, errorPresence product.Value) state.State {
		input := state.State{}.
			WriteValue(reg, key.SymbolValue(valueSymbol), valuePresence).
			WriteValue(reg, key.SymbolValue(errorSymbol), errorPresence).
			WriteValue(reg, key.ReturnSlot(0), valuePresence).
			WriteValue(reg, key.ReturnSlot(1), errorPresence)
		out, err := authority.PublishCallReturnPresenceRow(context.Background(), reg, point, []CallReturnPresenceRowTarget{
			{Index: 0, Path: returnValueKey, Value: valuePresence},
			{Index: 0, Path: valueKey, Value: valuePresence},
			{Index: 1, Path: returnErrorKey, Value: errorPresence},
			{Index: 1, Path: errorKey, Value: errorPresence},
		}, input)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	joined := state.Domain(reg).Join(row(present, absent), row(absent, present))
	want := pathevidence.NewPathPresenceImplication(errorKey, presence.Absent(), valueKey, presence.Present())
	if !joined.HasPathPresenceImplication(want) {
		t.Fatal("joined feasible rows lost err-absent => value-present correlation")
	}
}

func TestCallReturnPresencePlanCoordinatesEqualConcretePathPublication(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(51)
	valueSymbol, errorSymbol := symbol.ID(61), symbol.ID(62)
	builder := visibility.NewBuilder()
	builder.Define(point, valueSymbol, "value")
	builder.Define(point, errorSymbol, "err")
	resolver := visibility.NewResolver(builder.Build())
	authority := NewPathSemanticAuthority(resolver, nil, nil)
	valueKey := resolver.KeySpace().FromPath(pathdom.NewPath(valueSymbol, "value"))
	errorKey := resolver.KeySpace().FromPath(pathdom.NewPath(errorSymbol, "err"))
	present, absent := product.NewWithPresence(reg, product.ShapeTop, presence.Present()), product.Absent(reg)
	targets := []CallReturnPresenceRowTarget{
		{Index: 0, Path: valueKey, Value: present},
		{Index: 1, Path: errorKey, Value: absent},
	}
	plan, err := authority.PrepareCallReturnPresenceRow(reg, point, targets)
	if err != nil {
		t.Fatal(err)
	}
	input := state.State{}.
		WriteValue(reg, key.SymbolValue(valueSymbol), present).
		WriteValue(reg, key.SymbolValue(errorSymbol), absent)
	concrete, err := plan.ApplyConcrete(context.Background(), input, input)
	if err != nil {
		t.Fatal(err)
	}

	domain := state.RegisteredProductDomain(reg)
	family, ok := domain.PathEvidenceCoordinateFamily()
	if !ok {
		t.Fatal("registered product has no presence-implication family")
	}
	inputFactors, err := domain.DecomposeLanes(input, []state.ProductLane{family.Lane()})
	if err != nil || len(inputFactors) != 1 {
		t.Fatalf("input family decomposition = %d/%v", len(inputFactors), err)
	}
	skeleton, scalars, err := domain.DecomposeCoordinateFamily(inputFactors[0], family, resolver.KeySpace())
	if err != nil {
		t.Fatal(err)
	}
	skeleton, scalars, err = plan.ApplyCoordinates(domain, skeleton, scalars)
	if err != nil {
		t.Fatal(err)
	}
	factored, err := domain.ComposeCoordinateFamilies(family.Lane(), resolver.KeySpace(), []state.CoordinateFamilySkeleton{skeleton}, [][]state.CoordinateScalarFactor{scalars})
	if err != nil {
		t.Fatal(err)
	}
	concreteFactors, err := domain.DecomposeLanes(concrete, []state.ProductLane{family.Lane()})
	if err != nil || len(concreteFactors) != 1 {
		t.Fatalf("concrete family decomposition = %d/%v", len(concreteFactors), err)
	}
	factoredState, err := domain.ComposeSparse([]state.LaneFactor{factored})
	if err != nil {
		t.Fatal(err)
	}
	concreteState, err := domain.ComposeSparse(concreteFactors)
	if err != nil {
		t.Fatal(err)
	}
	if !domain.Lattice().Equal(factoredState, concreteState) {
		t.Fatal("factorwise call-return presence publication differs from concrete canonical publication")
	}
}

func TestCallReturnPresenceCoordinateInventoryCoversEveryFeasibleRow(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(71)
	valueSymbol, errorSymbol := symbol.ID(81), symbol.ID(82)
	builder := visibility.NewBuilder()
	builder.Define(point, valueSymbol, "value")
	builder.Define(point, errorSymbol, "err")
	resolver := visibility.NewResolver(builder.Build())
	authority := NewPathSemanticAuthority(resolver, nil, nil)
	domain := state.RegisteredProductDomain(reg)
	valuePath, errorPath := pathdom.NewPath(valueSymbol, "value"), pathdom.NewPath(errorSymbol, "err")
	rootValueKey := resolver.KeySpace().FromPath(valuePath)
	rootErrorKey := resolver.KeySpace().FromPath(errorPath)
	valueKey, valueVisible := resolver.VisibleKeyspaceKeyAt(point, valuePath)
	errorKey, errorVisible := resolver.VisibleKeyspaceKeyAt(point, errorPath)
	if !valueVisible || !errorVisible || valueKey == rootValueKey || errorKey == rootErrorKey {
		t.Fatal("test requires distinct versioned call-result paths")
	}
	universe, err := authority.CallReturnPresenceCoordinateInventory(domain, []CallReturnPresenceTarget{
		{Index: 0, Path: valueKey},
		{Index: 1, Path: errorKey},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := universe.Len(); got != 8 {
		t.Fatalf("call-return presence coordinate universe = %d, want 8", got)
	}
	present, absent := product.NewWithPresence(reg, product.ShapeTop, presence.Present()), product.Absent(reg)
	for _, row := range [][2]product.Value{{present, absent}, {absent, present}} {
		plan, err := authority.PrepareCallReturnPresenceRow(reg, point, []CallReturnPresenceRowTarget{
			{Index: 0, Path: valueKey, Value: row[0]},
			{Index: 1, Path: errorKey, Value: row[1]},
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, publication := range plan.publications {
			if publication.Trigger != rootValueKey && publication.Trigger != rootErrorKey ||
				publication.Target != rootValueKey && publication.Target != rootErrorKey {
				t.Fatalf("call-return row retained noncanonical root endpoint: %#v", publication)
			}
		}
		produced, err := plan.CoordinateFactorInventory(domain)
		if err != nil {
			t.Fatal(err)
		}
		for _, slot := range produced.Slots() {
			contained, err := universe.Contains(domain, slot)
			if err != nil || !contained {
				t.Fatalf("feasible row coordinate outside frozen universe: contained=%v err=%v", contained, err)
			}
		}
	}
}
