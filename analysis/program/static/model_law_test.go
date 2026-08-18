package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestCommitInputCanonicalStreamsAreConsumedAtPublicationBoundary(t *testing.T) {
	draft, err := Build(staticFixture(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	wantID := draft.state.component.ContentID()
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	input := validCommitInputForFixture()
	component, err := finalizer.Commit(input)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	input.TypeOf[0] = 0
	input.Annotations[0] = 0
	input.Publications[0] = 0
	if component == nil || component.ContentID() != wantID {
		t.Fatal("CommitInput mutation changed the published authored identity")
	}
	if got := component.View().Publications().Count(); got != 1 {
		t.Fatalf("published view lost publication relation after caller mutation: count=%d", got)
	}
}

func TestContractsModelZeroViewsFailClosed(t *testing.T) {
	var contracts Contracts
	if contracts.Functions().Count() != 0 || contracts.Calls().Count() != 0 {
		t.Fatal("zero Contracts model exposed rows")
	}
	var functions Functions
	if functions.Count() != 0 {
		t.Fatal("zero Functions model exposed a term")
	}
	if got, ok := functions.At(0); ok || got != 0 {
		t.Fatalf("zero Functions At(0) = %v/%v", got, ok)
	}
	var calls Calls
	if calls.Count() != 0 {
		t.Fatal("zero Calls model exposed a term")
	}
	if got, ok := calls.At(0); ok || got != 0 {
		t.Fatalf("zero Calls At(0) = %v/%v", got, ok)
	}
}

func TestStaticModelPrimitiveVocabularyAndRuntimeBoundary(t *testing.T) {
	names := map[string]PrimitiveKind{
		"nil": PrimitiveNil, "boolean": PrimitiveBoolean, "number": PrimitiveNumber,
		"integer": PrimitiveInteger, "string": PrimitiveString, "function": PrimitiveFunction,
		"any": PrimitiveAny, "unknown": PrimitiveUnknown, "never": PrimitiveNever, "self": PrimitiveSelf,
	}
	for name, want := range names {
		got, ok := PrimitiveKindForName(name)
		if !ok || got != want || !got.valid() {
			t.Fatalf("PrimitiveKindForName(%q) = %v/%v, want %v/true", name, got, ok, want)
		}
	}
	if _, ok := PrimitiveKindForName("user-defined"); ok {
		t.Fatal("open primitive spelling entered the closed model vocabulary")
	}
	if PrimitiveFunction.RuntimeLoadable() || PrimitiveSelf.RuntimeLoadable() {
		t.Fatal("static-only primitive entered the runtime-loadable subset")
	}
	if !PrimitiveAny.RuntimeLoadable() || !PrimitiveInteger.RuntimeLoadable() {
		t.Fatal("runtime-loadable primitive was excluded from the model subset")
	}
}

func TestOperandsModelRowsRetainExactCrossOwnerHandles(t *testing.T) {
	input := operandsFixture(t)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.Operands.Claim[0].Claim = 0
	input.Operands.TypeValue[0].Target = 0
	input.Operands.Annotation[0].Scope = 0
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	view := component.View().Operands()
	if claim, ok := view.Claims().At(0); !ok || claim != keyspace.MakeTerm(keyspace.FamilyValueClaim, 1) {
		t.Fatalf("ClaimTarget model row = %v/%v", claim, ok)
	}
	if target, ok := view.TypeValues().Target(keyspace.MakeTerm(keyspace.FamilyTypeValue, 1)); !ok ||
		target != keyspace.MakeTerm(keyspace.FamilyTypeRef, 1) {
		t.Fatalf("TypeValueTarget model row = %v/%v", target, ok)
	}
	if row, ok := view.Annotations().Get(keyspace.MakeTerm(keyspace.FamilyAnnotation, 1)); !ok ||
		row.Scope != keyspace.MakeTerm(keyspace.FamilyValueClaim, 1) {
		t.Fatalf("Annotation model row = %+v/%v", row, ok)
	}
}

func TestOperatorsModelRowsRetainTypedOperatorShape(t *testing.T) {
	input := operatorFixture()
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.Operators.TypeOf[0].Scope = 0
	input.Operators.KeyOf[0].Inner = 0
	input.Operators.IndexAccess[0].Object = 0
	input.Operators.Conditional[0].Else = 0
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	view := component.View().Operators()
	if scope, operand, ok := view.TypeOfs().Get(keyspace.MakeTerm(keyspace.FamilyTypeOf, 1)); !ok ||
		scope != keyspace.MakeTerm(keyspace.FamilyCell, 1) || operand != keyspace.MakeTerm(keyspace.FamilyRead, 1) {
		t.Fatalf("TypeOf model row = %v/%v/%v", scope, operand, ok)
	}
	if inner, ok := view.KeyOfs().Get(keyspace.MakeTerm(keyspace.FamilyTypeKeyOf, 1)); !ok ||
		inner != keyspace.MakeTerm(keyspace.FamilyTypeOf, 1) {
		t.Fatalf("KeyOf model row = %v/%v", inner, ok)
	}
	if object, index, ok := view.IndexAccesses().Get(keyspace.MakeTerm(keyspace.FamilyTypeIndexAccess, 1)); !ok ||
		object != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) || index != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2) {
		t.Fatalf("IndexAccess model row = %v/%v/%v", object, index, ok)
	}
	if check, extends, thenTerm, elseTerm, ok := view.Conditionals().Get(keyspace.MakeTerm(keyspace.FamilyTypeConditional, 1)); !ok ||
		check != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3) || extends != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 4) ||
		thenTerm != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 5) || elseTerm != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 6) {
		t.Fatalf("Conditional model row = %v/%v/%v/%v/%v", check, extends, thenTerm, elseTerm, ok)
	}
}

// TestPoolAccessorsFailClosedOnMalformedRanges proves the pool accessors bound
// every read themselves. A range that does not lie inside its pool yields an
// empty window, so no query boundary has to restate the bound to stay safe.
func TestPoolAccessorsFailClosedOnMalformedRanges(t *testing.T) {
	pool := []keyspace.Term{11, 22, 33}
	for _, testCase := range []struct {
		name string
		span poolRange
		want []keyspace.Term
		size int
	}{
		{name: "whole pool", span: poolRange{Start: 0, End: 3}, want: pool, size: 3},
		{name: "interior", span: poolRange{Start: 1, End: 2}, want: pool[1:2], size: 1},
		{name: "empty", span: poolRange{Start: 2, End: 2}, size: 0},
		{name: "past end", span: poolRange{Start: 1, End: 4}, size: 3},
		{name: "inverted", span: poolRange{Start: 2, End: 1}, size: 0},
		{name: "wholly outside", span: poolRange{Start: 9, End: 12}, size: 3},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			window := poolSlice(pool, testCase.span)
			if len(window) != len(testCase.want) {
				t.Fatalf("poolSlice window = %v, want %v", window, testCase.want)
			}
			for index, value := range testCase.want {
				if window[index] != value {
					t.Fatalf("poolSlice[%d] = %d, want %d", index, window[index], value)
				}
			}
			if got := testCase.span.len(); got != testCase.size {
				t.Fatalf("poolRange.len() = %d, want %d", got, testCase.size)
			}
			for index := -1; index <= len(pool)+1; index++ {
				value, ok := poolAt(pool, testCase.span, index)
				inside := index >= 0 && index < len(testCase.want)
				if ok != inside {
					t.Fatalf("poolAt(%d) ok = %v, want %v", index, ok, inside)
				}
				if inside && value != testCase.want[index] {
					t.Fatalf("poolAt(%d) = %d, want %d", index, value, testCase.want[index])
				}
			}
		})
	}
}

func TestPublicationModelRowsRetainPairIdentityAcrossCallerMutation(t *testing.T) {
	input := publicationFixture(t)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.Publications.Type[0].Assign = 0
	input.Publications.Type[0].Pair = 99
	input.Publications.Type[0].Target = 0
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	assign, pair, target, ok := component.View().Publications().Get(keyspace.MakeTerm(keyspace.FamilyTypePublication, 1))
	if !ok || assign != keyspace.MakeTerm(keyspace.FamilyAssign, 1) || pair != 0 ||
		target != keyspace.MakeTerm(keyspace.FamilyTypeRef, 1) {
		t.Fatalf("Publication model row = %v/%d/%v/%v", assign, pair, target, ok)
	}
}

func TestSignaturesModelParameterRowsRetainCoordinatesAndTypes(t *testing.T) {
	input := signatureFixture(t)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.Signatures.TypeFunction[0].Parameters[0].Name = 0
	input.Signatures.TypeFunction[0].Parameters[0].Type = 0
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	row, ok := component.View().Signatures().TypeFunctions().ParameterAt(keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1), 0)
	if !ok || row.Name != 9 || row.Type != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) ||
		row.NameCoordinate == (source.Coordinate{}) {
		t.Fatalf("Parameter model row = %+v/%v", row, ok)
	}
}
