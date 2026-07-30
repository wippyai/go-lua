package transformer

import (
	"context"
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestFormalCovariantExposureFreezesExactSparseN6Cone(t *testing.T) {
	reg := standard.Registry()
	program := formalRootInputTestProgram(t, reg)
	body := &program.bodies[0]
	const source = symbol.ID(101)
	point := cfg.Point(1)
	builder := visibility.NewBuilder()
	builder.Define(point, source, "source")
	resolver := visibility.NewResolver(builder.Build())
	body.keys = resolver.KeySpace()
	body.pathSemantics = factapply.NewPathSemanticAuthority(resolver, nil, typevalue.NewCache())
	transaction := factapply.PlanCovariantExposureTransaction(factflow.NewFacts(factflow.FactsInput{
		CovariantExposures: map[cfg.Point][]factflow.CovariantExposure{
			point: {factflow.NewCovariantExposure(pathdom.NewPath(source, "source"), product.Top(), factflow.CovariantExposureRecord)},
		},
	}), point)
	body.relation.code.nodes = []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepCovariantExposure, covariant: transaction}}, next: 2},
		{kind: relationNodeOutcome, outcome: 1},
	}
	body.relation.code.root = 1
	fibers, err := freezeFormalFiberInventory(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalFibers = fibers
	program.formalSlots = fibers.slots
	operator := formalRelationOperatorRef{
		kind: formalRelationCellStep, code: body.relation.code, root: 1, step: 1,
	}
	plan, err := freezeFormalCovariantExposureStep(program, 1, operator)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.valid(operator) || len(plan.bindings) != 1 ||
		plan.bindings[0].Kind != factapply.CovariantFactorBindingStructural {
		t.Fatalf("formal N6 plan = %#v", plan)
	}
	span, ok := fibers.span(1)
	if !ok {
		t.Fatal("formal N6 span")
	}
	sparseWidth := len(plan.currentOrdinals) + len(plan.entryOrdinals)
	fullWidth := 2 * span.count
	if sparseWidth >= fullWidth {
		t.Fatalf("CovariantExposure sparse correlation width = %d, full width = %d", sparseWidth, fullWidth)
	}
	if len(plan.writeOrdinals) >= len(plan.currentOrdinals) ||
		len(plan.currentOrdinals) != len(plan.entryOrdinals) {
		t.Fatalf("formal N6 read/write widths = current %d entry %d writes %d",
			len(plan.currentOrdinals), len(plan.entryOrdinals), len(plan.writeOrdinals))
	}
	for index := range plan.currentOrdinals {
		if plan.currentOrdinals[index] != plan.entryOrdinals[index] {
			t.Fatalf("formal N6 point-entry cone differs at ordinal %d", index)
		}
	}
	t.Logf("CovariantExposure correlation width: %d sparse roots, %d full-product roots; %d published roots",
		sparseWidth, fullWidth, len(plan.writeOrdinals))
}

func TestFormalCovariantExposureExecutesCanonicalN6AndPreservesResidualProduct(t *testing.T) {
	reg := standard.Registry()
	base := formalRootInputTestProgram(t, reg)
	body := &base.bodies[0]
	point := body.graph.Exit()
	const source = symbol.ID(101)
	narrowType := typetable.NewRecord().Field("x", typ.Number).Build()
	wideType := typetable.NewRecord().Field("x", typ.MaterializeUnion([]typ.Type{typ.Number, typ.String})).Build()
	narrowValue := typevalue.WithWitness(reg, typevalue.FromType(reg, narrowType), narrowType)
	wideValue := typevalue.WithWitness(reg, typevalue.FromType(reg, wideType), wideType)
	builder := visibility.NewBuilder()
	builder.Define(point, source, "source")
	resolver := visibility.NewResolver(builder.Build())
	body.keys = resolver.KeySpace()
	body.pathSemantics = factapply.NewPathSemanticAuthority(resolver, nil, typevalue.NewCache())
	sourcePath := pathdom.NewPath(source, "source")
	field, ok := resolver.StateKeyAt(point, sourcePath.Field("x"))
	if !ok {
		t.Fatal("formal N6 field path")
	}
	input := state.Reachable(state.State{}).
		WriteValue(reg, statekey.SymbolValue(source), narrowValue).
		WritePathKey(reg, resolver.KeySpace(), field.PathKey(), typevalue.LiteralNumber(reg, 1))
	body.entrySeedPlan = state.NewEntrySeedPlan([]state.ValueSeed{{Slot: statekey.SymbolValue(source), Value: narrowValue}})
	body.initialStatePlan = testInitialStatePlan(t, body.body, body.graph,
		state.NewInitialStateSeed(state.InitialCoordinate(body.graph.Entry()), input))
	transaction := factapply.PlanCovariantExposureTransaction(factflow.NewFacts(factflow.FactsInput{
		CovariantExposures: map[cfg.Point][]factflow.CovariantExposure{
			point: {factflow.NewCovariantExposure(sourcePath, wideValue, factflow.CovariantExposureRecord)},
		},
	}), point)
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepCovariantExposure, covariant: transaction}}, next: 2},
		{kind: relationNodeOutcome, outcome: 1},
	})
	execution, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	cell := formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
	equation, ok := program.formalTemplate.equation(cell)
	if !ok || equation.Operator.stepCapability != formalRelationStepCapabilityCovariantExposure || equation.Operator.covariantExposure == nil {
		t.Fatal("formal CovariantExposure capability was not admitted")
	}
	regions, err := execution.algebra.tupleLeafRegions(execution.values[cell])
	if err != nil || len(regions) != 1 {
		t.Fatalf("formal N6 regions = %d/%v", len(regions), err)
	}
	values, err := regions[0].evaluator.valuesFactor()
	if err != nil {
		t.Fatal(err)
	}
	slot := equation.Operator.covariantExposure.valueSlots[0]
	gotType, ok := typevalue.TypeOf(reg, values.Values[slot])
	if !ok || !typ.TypeEquals(gotType, wideType) {
		t.Fatalf("formal N6 widened type = %v/%t, want %v", gotType, ok, wideType)
	}
	operands, err := partitionFormalRelationStepOperands(equation)
	if err != nil {
		t.Fatal(err)
	}
	beforeRegions, err := execution.algebra.tupleLeafRegions(execution.values[operands.Flow.Source.cell])
	if err != nil || len(beforeRegions) != 1 {
		t.Fatalf("formal N6 input regions = %d/%v", len(beforeRegions), err)
	}
	formalField, _, _, ok := execution.algebra.span(1)
	if !ok {
		t.Fatal("formal N6 span")
	}
	visibleField, ok := body.pathSemantics.VisibleLocalPathKey(point, sourcePath.Field("x"))
	if !ok {
		t.Fatal("formal N6 visible field")
	}
	rekeyedField, err := body.productDomain.RekeyStructuralKeyFormal(formalField.rekey, visibleField)
	if err != nil {
		t.Fatal(err)
	}
	boundField, ok := formalField.keys.AppendSegment(equation.Operator.covariantExposure.bindings[0].Root, sourcePath.Field("x").Segments[0])
	if !ok || boundField != rekeyedField {
		t.Fatalf("formal N6 target mismatch: bound=%s stored=%s", formalField.keys.Format(boundField), formalField.keys.Format(rekeyedField))
	}
	mutated := make(map[state.LaneOrdinal]bool)
	for _, group := range equation.Operator.covariantExposure.lanes {
		mutated[group.lane.Ordinal()] = true
	}
	for _, group := range regions[0].evaluator.layout.nonValues {
		if mutated[group.lane.Ordinal()] {
			continue
		}
		before, beforeErr := beforeRegions[0].evaluator.laneFactor(group)
		after, afterErr := regions[0].evaluator.laneFactor(group)
		equal, equalErr := regions[0].evaluator.authority.product.LaneEqual(before, after)
		if beforeErr != nil || afterErr != nil || equalErr != nil || !equal {
			t.Fatalf("formal N6 changed unrelated lane %q: %v/%v/%v", group.lane.ID(), beforeErr, afterErr, equalErr)
		}
	}

	canceled, session := cancellation.Attach(context.Background())
	session.Token().Cancel(context.Canceled)
	if result, cancelErr := executeFormalRelation(canceled, program); cancelErr == nil || result != nil {
		t.Fatalf("canceled formal N6 published a result: %#v/%v", result, cancelErr)
	}
}
