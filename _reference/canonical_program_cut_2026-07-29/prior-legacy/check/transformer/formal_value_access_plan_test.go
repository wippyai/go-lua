package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

func formalValueAccessTestFixture(t *testing.T) (*formalTupleAlgebra, formalValuesFiberGroup, []FormalSlot) {
	t.Helper()
	base := formalRootInputTestProgram(t, standard.Registry())
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{{}, {kind: relationNodeOutcome, outcome: 1}})
	algebra := formalTupleTestAlgebra(t, program)
	span, _, _, ok := algebra.span(1)
	if !ok {
		t.Fatal("formal Values span")
	}
	values, ok := span.valuesGroup()
	if !ok || len(values.descriptor.valueSlots) < 4 {
		t.Fatal("formal Values group")
	}
	slots := make([]FormalSlot, len(values.descriptor.valueSlots))
	for index, value := range values.descriptor.valueSlots {
		slots[index] = value.slot
	}
	return algebra, values, slots
}

func formalValueAccessTestView(t *testing.T, algebra *formalTupleAlgebra, values formalValuesFiberGroup, factor state.ValueFactor[FormalSlot], ordinals []formalFiberOrdinal) formalSparseLeafView {
	t.Helper()
	tuple, err := algebra.writeValuesFactor(formalTupleTestLive(t, algebra, 1), values, factor)
	if err != nil {
		t.Fatal(err)
	}
	regions, err := algebra.partitionSparseLeafViewsUnderCare([]formalSparseTupleProjection{{tuple: tuple, ordinals: ordinals}}, nil)
	if err != nil || len(regions) != 1 || len(regions[0].views) != 1 {
		t.Fatalf("sparse Values regions=%d err=%v", len(regions), err)
	}
	return regions[0].views[0]
}

func TestFormalValueAccessPlanSealsExactDeterministicCapabilities(t *testing.T) {
	_, values, slots := formalValueAccessTestFixture(t)
	plan, err := sealFormalValueAccessPlan(values, []FormalSlot{slots[2], slots[0], slots[2]}, []FormalSlot{slots[3], slots[1], slots[3]})
	if err != nil || !plan.valid() {
		t.Fatalf("seal sparse Values plan: %#v, %v", plan, err)
	}
	if len(plan.reads) != 2 || len(plan.writes) != 2 || len(plan.readOrdinals) != 3 || len(plan.writeOrdinals) != 2 {
		t.Fatalf("sparse Values widths reads=%d writes=%d readOrdinals=%d writeOrdinals=%d", len(plan.reads), len(plan.writes), len(plan.readOrdinals), len(plan.writeOrdinals))
	}
	for index := 1; index < len(plan.readOrdinals); index++ {
		if plan.readOrdinals[index-1] >= plan.readOrdinals[index] {
			t.Fatalf("read ordinals are not canonical: %v", plan.readOrdinals)
		}
	}
	for index := 1; index < len(plan.writeOrdinals); index++ {
		if plan.writeOrdinals[index-1] >= plan.writeOrdinals[index] {
			t.Fatalf("write ordinals are not canonical: %v", plan.writeOrdinals)
		}
	}
	reversed, err := sealFormalValueAccessPlan(values, []FormalSlot{slots[0], slots[2]}, []FormalSlot{slots[1], slots[3]})
	if err != nil || len(reversed.readOrdinals) != len(plan.readOrdinals) || len(reversed.writeOrdinals) != len(plan.writeOrdinals) {
		t.Fatalf("reversed sparse Values plan = %#v, %v", reversed, err)
	}
	for index := range plan.readOrdinals {
		if reversed.readOrdinals[index] != plan.readOrdinals[index] {
			t.Fatalf("read order depends on input order: %v != %v", reversed.readOrdinals, plan.readOrdinals)
		}
	}
	for index := range plan.writeOrdinals {
		if reversed.writeOrdinals[index] != plan.writeOrdinals[index] {
			t.Fatalf("write order depends on input order: %v != %v", reversed.writeOrdinals, plan.writeOrdinals)
		}
	}
	empty, err := sealFormalValueAccessPlan(values, nil, nil)
	if err != nil || !empty.valid() || len(empty.readOrdinals) != 0 || len(empty.writeOrdinals) != 0 {
		t.Fatalf("empty sparse Values plan = %#v, %v", empty, err)
	}
}

func TestFormalValueAccessPlanMaterializesOnlyDeclaredReadsAndPublishesOnlyWrites(t *testing.T) {
	algebra, values, slots := formalValueAccessTestFixture(t)
	plan, err := sealFormalValueAccessPlan(values, []FormalSlot{slots[0]}, []FormalSlot{slots[1]})
	if err != nil {
		t.Fatal(err)
	}
	read := typevalue.String(algebra.program.registry)
	unrelated := typevalue.LiteralNumber(algebra.program.registry, 7)
	view := formalValueAccessTestView(t, algebra, values, state.ValueFactor[FormalSlot]{Values: map[FormalSlot]product.Value{
		slots[0]: read, slots[2]: unrelated,
	}}, plan.readOrdinals)
	input, err := plan.materialize(view)
	if err != nil || len(input.Values) != 1 || !product.Equal(algebra.program.registry, input.Values[slots[0]], read) {
		t.Fatalf("sparse Values input=%#v err=%v", input, err)
	}
	result := state.ValueFactor[FormalSlot]{Values: map[FormalSlot]product.Value{
		slots[0]: read, slots[1]: unrelated,
	}}
	writes, err := plan.factorPublication(view, result)
	if err != nil || len(writes) != 1 || writes[0].slot != slots[1] || writes[0].ordinal != plan.writeOrdinals[0] || writes[0].leaf == 0 {
		t.Fatalf("sparse Values writes=%#v err=%v", writes, err)
	}
	result.Values[slots[2]] = unrelated
	if _, err := plan.factorPublication(view, result); err == nil {
		t.Fatal("undeclared Values write was accepted")
	}
	delete(result.Values, slots[2])
	result.Values[slots[0]] = unrelated
	if _, err := plan.factorPublication(view, result); err == nil {
		t.Fatal("read-only Values mutation was accepted")
	}
}

func TestFormalValueAccessPlanTopCarriesDormantWriteRoots(t *testing.T) {
	algebra, values, slots := formalValueAccessTestFixture(t)
	plan, err := sealFormalValueAccessPlan(values, []FormalSlot{slots[0]}, []FormalSlot{slots[1]})
	if err != nil {
		t.Fatal(err)
	}
	view := formalValueAccessTestView(t, algebra, values, state.ValueFactor[FormalSlot]{Top: true}, plan.readOrdinals)
	input, err := plan.materialize(view)
	if err != nil || !input.Top || len(input.Values) != 0 {
		t.Fatalf("Top sparse Values input=%#v err=%v", input, err)
	}
	writes, err := plan.factorPublication(view, state.ValueFactor[FormalSlot]{Top: true})
	if err != nil || len(writes) != 0 {
		t.Fatalf("Top sparse Values publication=%#v err=%v", writes, err)
	}
	if _, err := plan.factorPublication(view, state.ValueFactor[FormalSlot]{}); err == nil {
		t.Fatal("Values Top change was accepted")
	}
}
