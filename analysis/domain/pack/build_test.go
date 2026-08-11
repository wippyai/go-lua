package pack

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/static"
)

func TestBuilderConstructsOnlyCompleteRootFencedRelations(t *testing.T) {
	fixture := newCarrierFixture(t, realClasses(t))
	builder, ok := fixture.schema.Builder(fixture.firstRoot)
	if !ok {
		t.Fatal("first root builder")
	}
	scalar, ok := builder.Endpoint(fixture.firstEnd)
	if !ok {
		t.Fatal("endpoint scalar")
	}
	term, ok := builder.Closed(scalar)
	if !ok {
		t.Fatal("closed term")
	}
	scalarEquation, ok := builder.Scalar(fixture.firstEnd, scalar)
	if !ok {
		t.Fatal("scalar equation")
	}
	packEquation, ok := builder.Pack(fixture.firstPort, term)
	if !ok {
		t.Fatal("pack equation")
	}
	caseValue, ok := builder.Case(scalarEquation, packEquation)
	if !ok {
		t.Fatal("complete case")
	}
	value, ok := builder.Value(caseValue)
	if !ok || !fixture.schema.Admit(fixture.firstRoot, value) {
		t.Fatal("complete relation was not admitted at its root")
	}
	if _, ok := builder.Case(scalarEquation); ok {
		t.Fatal("partial scalar observation became a Pack relation")
	}
}

func TestBuilderRejectsForeignRootAndTarget(t *testing.T) {
	fixture := newCarrierFixture(t, realClasses(t))
	foreign := newCarrierFixture(t, realClasses(t))
	first, ok := fixture.schema.Builder(fixture.firstRoot)
	if !ok {
		t.Fatal("first builder")
	}
	if _, ok := fixture.schema.Builder(Root{}); ok {
		t.Fatal("invalid root acquired builder")
	}
	if _, ok := first.Endpoint(fixture.secondEnd); !ok {
		t.Fatal("same-schema source endpoint should remain reusable")
	}
	if _, ok := first.Endpoint(foreign.firstEnd); ok {
		t.Fatal("foreign-schema source endpoint crossed relation fence")
	}
	if _, ok := first.FreeTail(foreign.firstPort); ok {
		t.Fatal("foreign-schema tail crossed relation fence")
	}
	scalar, ok := first.Endpoint(fixture.firstEnd)
	if !ok {
		t.Fatal("first scalar")
	}
	if _, ok := first.Scalar(fixture.secondEnd, scalar); ok {
		t.Fatal("foreign relation target entered first root")
	}
}

func TestBuilderOpenTailPreservesSchemaFence(t *testing.T) {
	fixture := newCarrierFixture(t, realClasses(t))
	builder, ok := fixture.schema.Builder(fixture.firstRoot)
	if !ok {
		t.Fatal("first builder")
	}
	tail, ok := builder.FreeTail(fixture.secondPort)
	if !ok {
		t.Fatal("free tail")
	}
	offset, ok := builder.Zero()
	if !ok {
		t.Fatal("zero offset")
	}
	head, ok := builder.Head(tail, offset)
	if !ok {
		t.Fatal("head")
	}
	rest, ok := builder.Tail(tail, offset)
	if !ok {
		t.Fatal("tail")
	}
	term, ok := builder.Open([]Scalar{head}, rest, nil)
	if !ok || term.Kind() != TermOpen {
		t.Fatal("open term")
	}
	if _, ok := builder.AnyScalar(static.Class{}); ok {
		t.Fatal("invalid class admitted as unknown scalar")
	}
}

func TestOpenSelectionAndDropRetainShortMiddleSuffixAlternatives(t *testing.T) {
	fixture, builder, term, tail, nilScalar, anyScalar, nilAny := openSuffixSelectionTerm(t)
	wantCounts := []int{2, 3, 4}
	for index := 0; index < 3; index++ {
		table, tableOK := fixture.schema.TableIndex(int64(index))
		alternatives, alternativesOK := builder.ScalarAlternatives(term, table)
		if !tableOK || !alternativesOK || len(alternatives) != wantCounts[index] {
			t.Fatalf("open suffix selection index %d = %d/%v table=%v", index, len(alternatives), alternativesOK, tableOK)
		}
		head, headOK := builder.Head(tail, table.offset)
		if !headOK || alternatives[0].Kind() != ScalarHead || !equalScalar(alternatives[0], head) || !equalTail(alternatives[0].tail, tail) || compareOffset(alternatives[0].offset, table.offset) != 0 {
			t.Fatalf("open suffix selection index %d lost exact head provenance", index)
		}
		expected := []Scalar{head}
		if index >= 0 {
			expected = append(expected, nilScalar)
		}
		if index >= 1 {
			expected = append(expected, anyScalar)
		}
		if index >= 2 {
			expected = append(expected, nilAny)
		}
		for alternativeIndex, want := range expected {
			if !equalScalar(alternatives[alternativeIndex], want) {
				t.Fatalf("open suffix selection index %d alternative %d lost exact scalar provenance", index, alternativeIndex)
			}
		}
		if index >= 2 && alternatives[len(alternatives)-1].Kind() != ScalarAny {
			t.Fatalf("open suffix selection index %d nil-fill must remain AnyScalar", index)
		}
		if _, scalarOK := builder.ScalarAt(term, table); scalarOK {
			t.Fatalf("ambiguous open suffix ScalarAt index %d = %v", index, scalarOK)
		}
	}
	for count, want := range map[int]int{0: 1, 1: 2, 2: 3} {
		alternatives, alternativesOK := builder.DropAlternatives(term, count)
		if !alternativesOK || len(alternatives) != want {
			t.Fatalf("open suffix drop count %d = %d/%v, want %d", count, len(alternatives), alternativesOK, want)
		}
	}
}

func openSuffixSelectionTerm(t testing.TB) (*carrierFixture, Builder, Term, TailRef, Scalar, Scalar, Scalar) {
	t.Helper()
	fixture := newCarrierFixture(t, realClasses(t))
	builder, ok := fixture.schema.Builder(fixture.firstRoot)
	if !ok {
		t.Fatal("first builder")
	}
	nilPort, ok := newPort(fixture.owner, 3, fixture.owner.classes.Nil(), true)
	if !ok {
		t.Fatal("nil tail port")
	}
	tail, ok := builder.FreeTail(nilPort)
	if !ok {
		t.Fatal("nil tail")
	}
	offset, ok := builder.Zero()
	if !ok {
		t.Fatal("zero offset")
	}
	rest, ok := builder.Tail(tail, offset)
	if !ok {
		t.Fatal("nil rest")
	}
	nilEndpoint, ok := newEndpoint(fixture.owner, 3, fixture.owner.classes.Nil())
	if !ok {
		t.Fatal("nil suffix endpoint")
	}
	nilScalar, ok := builder.Endpoint(nilEndpoint)
	if !ok {
		t.Fatal("nil suffix scalar")
	}
	anyScalar, ok := builder.Endpoint(fixture.secondEnd)
	if !ok {
		t.Fatal("any suffix scalar")
	}
	nilAny, ok := builder.AnyScalar(fixture.owner.classes.Nil())
	if !ok {
		t.Fatal("nil AnyScalar")
	}
	term, ok := builder.Open(nil, rest, []Scalar{nilScalar, anyScalar})
	if !ok {
		t.Fatal("open suffix term")
	}
	return fixture, builder, term, tail, nilScalar, anyScalar, nilAny
}

func TestOpenEmptyEndedRestAnyRetainsConstrainedClass(t *testing.T) {
	fixture := newCarrierFixture(t, realClasses(t))
	builder, ok := fixture.schema.Builder(fixture.firstRoot)
	if !ok {
		t.Fatal("first builder")
	}
	anyRest, ok := builder.AnyTail(fixture.owner.classes.AnyValue())
	if !ok {
		t.Fatal("Any rest")
	}
	anyTerm, ok := builder.Open(nil, anyRest, nil)
	if !ok || anyTerm.Kind() != TermAny {
		t.Fatal("unconstrained empty-ended RestAny must canonicalize to AnyPack")
	}
	nilRest, ok := builder.AnyTail(fixture.owner.classes.Nil())
	if !ok {
		t.Fatal("Nil rest")
	}
	nilTerm, ok := builder.Open(nil, nilRest, nil)
	if !ok || nilTerm.Kind() != TermOpen {
		t.Fatal("constrained empty-ended RestAny widened to AnyPack")
	}
	rest, suffix, restOK := nilTerm.Tail()
	if !restOK || rest.Kind() != RestAny || len(suffix) != 0 {
		t.Fatal("constrained RestAny lost its open representation")
	}
	class, classOK := rest.Class()
	if !classOK || !fixture.owner.equalClass(class, fixture.owner.classes.Nil()) {
		t.Fatal("constrained RestAny lost its Nil class")
	}
	if !termCovers(anyTerm, nilTerm) || termCovers(nilTerm, anyTerm) {
		t.Fatal("Any/constraint term order widened or reversed")
	}
}

func TestOffsetLookupAndAdditionStayAllocationFreeOnLargeShape(t *testing.T) {
	classes := realClasses(t)
	offsets := make([]nat, 16384)
	for index := range offsets {
		offsets[index] = natFromUint64(uint64(index))
	}
	owner, ok := newAlgebraWithOffsets(classes, nil, offsets)
	if !ok {
		t.Fatal("large offset algebra")
	}
	left, ok := offsetForUint64(owner, 7000)
	if !ok {
		t.Fatal("left offset")
	}
	right, ok := offsetForUint64(owner, 8000)
	if !ok {
		t.Fatal("right offset")
	}
	want, ok := offsetForUint64(owner, 15000)
	if !ok {
		t.Fatal("expected sum offset")
	}
	allocations := testing.AllocsPerRun(1000, func() {
		selected, selectedOK := offsetForUint64(owner, 16383)
		if !selectedOK || selected.index != 16383 {
			t.Fatal("large direct offset lookup")
		}
		sum, sumOK := addOffsets(left, right)
		if !sumOK || !sameOffset(sum, want) {
			t.Fatal("large direct offset addition")
		}
	})
	if allocations != 0 {
		t.Fatalf("large offset lookup/addition allocated %v times per run", allocations)
	}
}

func TestOpenSelectionComposesWithDropAlternatives(t *testing.T) {
	fixture, builder, term, _, _, _, _ := openSuffixSelectionTerm(t)
	for count := 0; count <= 3; count++ {
		table, ok := fixture.schema.TableIndex(int64(count))
		if !ok {
			t.Fatalf("table index %d", count)
		}
		direct, ok := builder.ScalarAlternatives(term, table)
		if !ok {
			t.Fatalf("direct scalar alternatives %d", count)
		}
		residuals, ok := builder.DropAlternatives(term, count)
		if !ok {
			t.Fatalf("drop alternatives %d", count)
		}
		union := make([]Scalar, 0, len(direct))
		appendUnique := func(value Scalar) {
			for _, existing := range union {
				if equalScalar(existing, value) {
					return
				}
			}
			union = append(union, value)
		}
		zero, ok := fixture.schema.TableIndex(0)
		if !ok {
			t.Fatal("zero table index")
		}
		for residualIndex, residual := range residuals {
			values, valuesOK := builder.ScalarAlternatives(residual, zero)
			if !valuesOK {
				t.Fatalf("residual %d scalar alternatives %d", residualIndex, count)
			}
			for _, value := range values {
				appendUnique(value)
			}
		}
		if len(union) != len(direct) {
			t.Fatalf("selection/drop composition %d = %d, direct %d", count, len(union), len(direct))
		}
		for index := range direct {
			if !equalScalar(union[index], direct[index]) {
				t.Fatalf("selection/drop composition %d alternative %d reordered or changed provenance", count, index)
			}
		}
	}
}

func TestBindAlternativesRetainCorrelatedShortTailPairs(t *testing.T) {
	fixture, builder, term, tail, nilScalar, anyScalar, nilAny := openSuffixSelectionTerm(t)
	alternatives, ok := builder.BindAlternatives(term, 3)
	if !ok || len(alternatives) != 4 {
		t.Fatalf("bind alternatives = %d/%v, want 4", len(alternatives), ok)
	}
	offset0, ok := fixture.schema.TableIndex(0)
	if !ok {
		t.Fatal("offset zero")
	}
	offset1, ok := fixture.schema.TableIndex(1)
	if !ok {
		t.Fatal("offset one")
	}
	offset2, ok := fixture.schema.TableIndex(2)
	if !ok {
		t.Fatal("offset two")
	}
	offset3, ok := fixture.schema.TableIndex(3)
	if !ok {
		t.Fatal("offset three")
	}
	head0, ok := builder.Head(tail, offset0.offset)
	if !ok {
		t.Fatal("head zero")
	}
	head1, ok := builder.Head(tail, offset1.offset)
	if !ok {
		t.Fatal("head one")
	}
	head2, ok := builder.Head(tail, offset2.offset)
	if !ok {
		t.Fatal("head two")
	}
	longRest, ok := builder.Tail(tail, offset3.offset)
	if !ok {
		t.Fatal("long residual")
	}
	longTerm, ok := builder.Open(nil, longRest, []Scalar{nilScalar, anyScalar})
	if !ok {
		t.Fatal("long residual term")
	}
	residualAny, ok := builder.Closed(anyScalar)
	if !ok {
		t.Fatal("one-suffix residual")
	}
	empty, ok := builder.Closed()
	if !ok {
		t.Fatal("empty residual")
	}
	wantFixed := [][]Scalar{
		{head0, head1, head2},
		{head0, head1, nilScalar},
		{head0, nilScalar, anyScalar},
		{nilScalar, anyScalar, nilAny},
	}
	wantResiduals := []Term{longTerm, residualAny, empty, empty}
	for alternativeIndex, alternative := range alternatives {
		if alternative.FixedCount() != 3 {
			t.Fatalf("bind alternative %d fixed count = %d", alternativeIndex, alternative.FixedCount())
		}
		residual, residualOK := alternative.Residual()
		if !residualOK || !residual.Equal(wantResiduals[alternativeIndex]) {
			t.Fatalf("bind alternative %d lost exact residual correlation", alternativeIndex)
		}
		for fixedIndex, want := range wantFixed[alternativeIndex] {
			got, gotOK := alternative.FixedAt(fixedIndex)
			if !gotOK || !equalScalar(got, want) {
				t.Fatalf("bind alternative %d fixed %d lost exact provenance", alternativeIndex, fixedIndex)
			}
		}
	}
	firstResidual, _ := alternatives[2].Residual()
	secondResidual, _ := alternatives[3].Residual()
	if !firstResidual.Equal(secondResidual) {
		t.Fatal("short tail alternatives should share the empty residual")
	}
	for index := 0; index < 3; index++ {
		left, _ := alternatives[2].FixedAt(index)
		right, _ := alternatives[3].FixedAt(index)
		if equalScalar(left, right) {
			t.Fatalf("short tail alternatives %d fixed slot %d unexpectedly deduplicated", index, index)
		}
	}
}
