package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestConcreteGuardRefinementUsesEvolvingOutputAndContradictionIsBottom(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(910)
	target := symbol.ID(911)
	stringValue := typevalue.FromType(reg, typ.String)
	numberValue := typevalue.FromType(reg, typ.Number)

	// Input and Output intentionally disagree. A refinement transaction reads
	// the evolving Output; consulting immutable Input here would keep the edge
	// reachable and silently lose the preceding point-local write.
	input := state.State{}.WriteValue(reg, key.SymbolValue(target), stringValue)
	output := state.State{}.WriteValue(reg, key.SymbolValue(target), numberValue)
	got, reachable := ApplyConcreteGuardRefinement(ConcreteGuardRefinementRequest{
		Registry:   reg,
		Point:      point,
		Input:      input,
		Output:     output,
		TargetPath: path.NewPath(target, "target"),
		Refinement: factflow.NewValueConstraint(stringValue),
	})
	if reachable {
		t.Fatal("contradictory guard reported a reachable edge")
	}
	if !state.IsBottom(reg, got) {
		t.Fatal("contradictory guard did not produce Bottom from evolving output")
	}
}

func TestConcreteValueRefinementLeavesImplicationActivationToPostStep(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(920)
	trigger := symbol.ID(921)
	target := symbol.ID(922)
	builder := visibility.NewBuilder()
	builder.Define(point, trigger, "trigger")
	builder.Define(point, target, "target")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()
	triggerPath := path.NewPath(trigger, "trigger")
	targetPath := path.NewPath(target, "target")
	triggerKey := ks.FromPath(triggerPath)
	targetKey := ks.FromPath(targetPath)
	falseValue := typevalue.FromType(reg, typ.False)
	stringValue := typevalue.FromType(reg, typ.String)

	out := state.State{}.
		WriteValue(reg, key.SymbolValue(trigger), product.Top()).
		WriteValue(reg, key.SymbolValue(target), product.Top()).
		AddPathPresenceImplication(pathevidence.NewPathValueRefinementImplication(triggerKey, falseValue, targetKey, stringValue))

	request := ConcreteValueRefinementRequest{
		Registry:   reg,
		Resolver:   resolver,
		Point:      point,
		Input:      out,
		Output:     out,
		TargetPath: triggerPath,
		Refinement: factflow.NewValueConstraint(falseValue),
	}
	narrowed := ApplyConcreteValueRefinement(request)
	if got := narrowed.ReadValue(reg, key.SymbolValue(target)); !product.Equal(reg, got, product.Top()) {
		t.Fatal("refinement kernel activated implications before its transfer barrier")
	}

	activated := activatePathPresenceImplicationsForPath(reg, resolver, point, narrowed, triggerPath)
	if got := activated.ReadValue(reg, key.SymbolValue(target)); !product.Equal(reg, got, stringValue) {
		t.Fatalf("explicit implication post-step target = %v, want string constraint", got)
	}
}

func TestConcreteValueRefinementSupportsValueOnlyStateDomain(t *testing.T) {
	reg := standard.Registry()
	target := symbol.ID(931)
	domain := state.DomainWithLanes(reg, []state.LaneID{state.LaneValues})
	out := domain.Bottom().WriteValue(reg, key.SymbolValue(target), product.Top())
	want := typevalue.FromType(reg, typ.Number)

	got := ApplyConcreteValueRefinement(ConcreteValueRefinementRequest{
		Registry:   reg,
		Point:      cfg.Point(930),
		Input:      out,
		Output:     out,
		TargetPath: path.NewPath(target, "target"),
		Refinement: factflow.NewValueConstraint(want),
	})
	if value := got.ReadValue(reg, key.SymbolValue(target)); !product.Equal(reg, value, want) {
		t.Fatalf("value-only refinement = %v, want number constraint", value)
	}
	if !domain.Equal(got, domain.Join(domain.Bottom(), got)) {
		t.Fatal("refinement escaped the selected value-only state domain")
	}
}
