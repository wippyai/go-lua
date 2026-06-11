package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

var _ SourceValues = sourceValuesStub{}

type sourceValuesStub struct{}

func (sourceValuesStub) ValueOfSource(cfg.Point, ValueSource, state.State, func(cfg.Point) state.State) (product.Value, bool) {
	return product.Value{}, false
}

func TestDTOConstructorsAndAccessorsCopySlices(t *testing.T) {
	source := ValueSource{
		Kind:        ValueSourceExpression,
		ExprRef:     ExprRef(1),
		HasExpr:     true,
		ExprIndex:   0,
		TargetIndex: 0,
		ResultIndex: NoValueSourceIndex,
	}
	callSource := ValueSource{
		Kind:         ValueSourceCall,
		ExprRef:      ExprRef(2),
		HasExpr:      true,
		ExprIndex:    1,
		TargetIndex:  1,
		ResultIndex:  0,
		CallPoint:    cfg.Point(99),
		HasCallPoint: true,
		Final:        true,
		Adjusted:     true,
	}

	localPath := path.NewPath(symbol.ID(10), "local")
	local := NewLocalAssignment(symbol.ID(10), localPath, source)
	assertPathEqual(t, local.TargetPath(), localPath)
	if got := local.Source(); got != source {
		t.Fatalf("local source = %#v, want %#v", got, source)
	}

	ordinaryPath := path.NewPath(symbol.ID(11), "ordinary")
	ordinary := NewOrdinaryAssignment(symbol.ID(11), ordinaryPath, source)
	assertPathEqual(t, ordinary.TargetPath(), ordinaryPath)
	if got := ordinary.Source(); got != source {
		t.Fatalf("ordinary source = %#v, want %#v", got, source)
	}

	returnSources := []ValueSource{source, callSource}
	ret := NewReturn(returnSources)
	returnSources[0].Kind = ValueSourceNil
	if got := ret.Sources(); got[0].Kind != ValueSourceExpression {
		t.Fatalf("return source copied from input as %v, want %v", got[0].Kind, ValueSourceExpression)
	}
	gotReturnSources := ret.Sources()
	gotReturnSources[0].Kind = ValueSourceNil
	if got := ret.Sources(); got[0].Kind != ValueSourceExpression {
		t.Fatalf("return source exposed mutable slice, got %v", got[0].Kind)
	}

	calleePath := path.NewPath(symbol.ID(12), "callee").Field("method")
	targetPath := path.NewPath(symbol.ID(13), "target")
	target := NewCallResultTarget(CallResultTargetLocalAssignment, 0, symbol.ID(13), targetPath)
	assertPathEqual(t, target.TargetPath(), targetPath)

	targets := []CallResultTarget{target}
	call := NewCallProducer(CallProducerConfig{
		Context:       CallProducerContextAssignment,
		CalleeSymbol:  symbol.ID(12),
		CalleePath:    calleePath,
		ExprRef:       ExprRef(3),
		HasExpr:       true,
		ExprIndex:     2,
		ResultTargets: targets,
		Final:         true,
		Expanded:      true,
		Adjusted:      true,
		OpenTail:      true,
	})
	calleePath.Segments[0].Name = "changed"
	targets[0] = NewCallResultTarget(CallResultTargetReturn, 0, 0, path.Path{})

	assertDirectField(t, call.CalleePath(), "method")
	gotCalleePath := call.CalleePath()
	gotCalleePath.Segments[0].Name = "changed-again"
	assertDirectField(t, call.CalleePath(), "method")
	if call.Context() != CallProducerContextAssignment || call.CalleeSymbol() != symbol.ID(12) {
		t.Fatalf("call context/symbol = %v/%v", call.Context(), call.CalleeSymbol())
	}
	if expr, ok := call.Expr(); !ok || expr != ExprRef(3) {
		t.Fatalf("call expr = %v/%v, want %v/true", expr, ok, ExprRef(3))
	}
	if !call.Final() || !call.Expanded() || !call.Adjusted() || !call.OpenTail() {
		t.Fatalf("call value-list flags were not preserved")
	}
	gotTargets := call.ResultTargets()
	if len(gotTargets) != 1 || gotTargets[0].Kind() != CallResultTargetLocalAssignment {
		t.Fatalf("call targets = %#v, want one local-assignment target", gotTargets)
	}
	gotTargets[0] = NewCallResultTarget(CallResultTargetReturn, 0, 0, path.Path{})
	if got := call.ResultTargets(); got[0].Kind() != CallResultTargetLocalAssignment {
		t.Fatalf("call result targets exposed mutable slice, got %v", got[0].Kind())
	}
	gotCallTargetPath := call.ResultTargets()[0].TargetPath()
	assertPathEqual(t, gotCallTargetPath, targetPath)
	assertPathEqual(t, call.ResultTargets()[0].TargetPath(), targetPath)
}

func TestTransferOwnedEnumsAreIndependentContracts(t *testing.T) {
	kinds := []ValueSourceKind{
		ValueSourceUnknown,
		ValueSourceExpression,
		ValueSourceCall,
		ValueSourceVararg,
		ValueSourceNil,
	}
	if len(kinds) != 5 || kinds[0] != ValueSourceUnknown || kinds[4] != ValueSourceNil {
		t.Fatalf("unexpected value source kinds: %#v", kinds)
	}

	contexts := []CallProducerContext{
		CallProducerContextUnknown,
		CallProducerContextAssignment,
		CallProducerContextReturn,
	}
	if len(contexts) != 3 || contexts[1] != CallProducerContextAssignment {
		t.Fatalf("unexpected call producer contexts: %#v", contexts)
	}

	targets := []CallResultTargetKind{
		CallResultTargetUnknown,
		CallResultTargetLocalAssignment,
		CallResultTargetOrdinaryAssignment,
		CallResultTargetReturn,
	}
	if len(targets) != 4 || targets[2] != CallResultTargetOrdinaryAssignment {
		t.Fatalf("unexpected call result target kinds: %#v", targets)
	}
}

func TestFactsCarrierCopiesAndReturnsFalseForMissingFacts(t *testing.T) {
	point := cfg.Point(20)
	missing := cfg.Point(21)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(1), HasExpr: true}
	callSource := ValueSource{Kind: ValueSourceCall, ExprRef: ExprRef(2), HasExpr: true}

	input := FactsInput{
		LocalAssignments: map[cfg.Point]LocalAssignment{
			point: NewLocalAssignment(symbol.ID(30), path.NewPath(symbol.ID(30), "local"), source),
		},
		OrdinaryAssignments: map[cfg.Point]OrdinaryAssignment{
			point: NewOrdinaryAssignment(symbol.ID(31), path.NewPath(symbol.ID(31), "ordinary"), source),
		},
		Returns: map[cfg.Point]Return{
			point: NewReturn([]ValueSource{source, callSource}),
		},
		Calls: map[cfg.Point]CallProducer{
			point: NewCallProducer(CallProducerConfig{
				Context:      CallProducerContextReturn,
				CalleeSymbol: symbol.ID(32),
				CalleePath:   path.NewPath(symbol.ID(32), "callee").Field("method"),
				ExprRef:      ExprRef(3),
				HasExpr:      true,
				ExprIndex:    0,
				ResultTargets: []CallResultTarget{
					NewCallResultTarget(CallResultTargetReturn, 0, 0, path.Path{}),
				},
			}),
		},
	}

	facts := NewFacts(input)
	input.LocalAssignments[point] = NewLocalAssignment(symbol.ID(40), path.NewPath(symbol.ID(40), "changed"), callSource)
	input.OrdinaryAssignments[point] = NewOrdinaryAssignment(symbol.ID(41), path.NewPath(symbol.ID(41), "changed"), callSource)
	input.Returns[point] = NewReturn([]ValueSource{{Kind: ValueSourceNil}})
	input.Calls[point] = NewCallProducer(CallProducerConfig{Context: CallProducerContextAssignment})

	if _, ok := facts.LocalAssignment(missing); ok {
		t.Fatal("missing local assignment returned ok")
	}
	if _, ok := facts.OrdinaryAssignment(missing); ok {
		t.Fatal("missing ordinary assignment returned ok")
	}
	if _, ok := facts.Return(missing); ok {
		t.Fatal("missing return returned ok")
	}
	if _, ok := facts.Call(missing); ok {
		t.Fatal("missing call returned ok")
	}

	local, ok := facts.LocalAssignment(point)
	if !ok {
		t.Fatal("local assignment missing")
	}
	if local.TargetSymbol() != symbol.ID(30) {
		t.Fatalf("local target symbol = %v, want 30", local.TargetSymbol())
	}
	localAgain, _ := facts.LocalAssignment(point)
	assertPathEqual(t, localAgain.TargetPath(), path.NewPath(symbol.ID(30), "local"))

	ordinary, ok := facts.OrdinaryAssignment(point)
	if !ok {
		t.Fatal("ordinary assignment missing")
	}
	if ordinary.TargetSymbol() != symbol.ID(31) {
		t.Fatalf("ordinary target symbol = %v, want 31", ordinary.TargetSymbol())
	}
	ordinaryAgain, _ := facts.OrdinaryAssignment(point)
	assertPathEqual(t, ordinaryAgain.TargetPath(), path.NewPath(symbol.ID(31), "ordinary"))

	ret, ok := facts.Return(point)
	if !ok {
		t.Fatal("return missing")
	}
	retSources := ret.Sources()
	if len(retSources) != 2 || retSources[0].Kind != ValueSourceExpression {
		t.Fatalf("return sources = %#v", retSources)
	}
	retSources[0].Kind = ValueSourceNil
	retAgain, _ := facts.Return(point)
	if got := retAgain.Sources(); got[0].Kind != ValueSourceExpression {
		t.Fatalf("facts return exposed mutable sources, got %v", got[0].Kind)
	}

	call, ok := facts.Call(point)
	if !ok {
		t.Fatal("call missing")
	}
	if call.Context() != CallProducerContextReturn {
		t.Fatalf("call context = %v, want %v", call.Context(), CallProducerContextReturn)
	}
	callCalleePath := call.CalleePath()
	callCalleePath.Segments[0].Name = "mutated"
	callAgain, _ := facts.Call(point)
	assertDirectField(t, callAgain.CalleePath(), "method")
	callTargets := call.ResultTargets()
	if len(callTargets) != 1 || callTargets[0].Kind() != CallResultTargetReturn {
		t.Fatalf("call targets = %#v", callTargets)
	}
	callTargets[0] = NewCallResultTarget(CallResultTargetLocalAssignment, 0, symbol.ID(33), path.NewPath(symbol.ID(33), "changed"))
	if got := callAgain.ResultTargets(); got[0].Kind() != CallResultTargetReturn {
		t.Fatalf("facts call exposed mutable targets, got %v", got[0].Kind())
	}
}

func assertDirectField(t *testing.T, p path.Path, want string) {
	t.Helper()
	got, ok := p.DirectFieldName()
	if !ok || got != want {
		t.Fatalf("path %q direct field = %q/%v, want %q/true", p.String(), got, ok, want)
	}
}

func assertPathEqual(t *testing.T, got path.Path, want path.Path) {
	t.Helper()
	if !got.Equal(want) {
		t.Fatalf("path = %q, want %q", got.String(), want.String())
	}
}
