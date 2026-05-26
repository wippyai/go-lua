package narrow

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestRefineLengthIndex_RemovesNilForPositiveExactLengthIndex(t *testing.T) {
	indexResult := typ.NewOptional(typ.String)
	got := RefineLengthIndex(typ.NewMap(typ.Integer, typ.String), indexResult, 1, 0)
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("RefineLengthIndex exact #t = %v, want string", got)
	}
}

func TestRefineLengthIndex_RequiresSequenceForOffsetIndex(t *testing.T) {
	indexResult := typ.NewOptional(typ.String)
	if got := RefineLengthIndex(typ.NewMap(typ.Integer, typ.String), indexResult, 2, -1); got != nil {
		t.Fatalf("RefineLengthIndex on sparse map offset = %v, want nil", got)
	}
	got := RefineLengthIndex(typ.NewArray(typ.String), indexResult, 2, -1)
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("RefineLengthIndex on array offset = %v, want string", got)
	}
}

func TestRefineSequenceIndex_UsesTupleLength(t *testing.T) {
	indexResult := typ.NewOptional(typ.Integer)
	got := RefineSequenceIndex(typ.NewTuple(typ.String, typ.Integer), indexResult, 2)
	if !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("RefineSequenceIndex tuple = %v, want integer", got)
	}
	if got := RefineSequenceIndex(typ.NewTuple(typ.String), indexResult, 2); got != nil {
		t.Fatalf("RefineSequenceIndex past tuple end = %v, want nil", got)
	}
}

func TestRefineByLengthLowerBound_FiltersFreshEmptyBranch(t *testing.T) {
	item := typ.NewRecord().Field("text", typ.String).Build()
	arr := typ.NewArray(item)
	base := typ.NewUnion(typ.NewRecord().Build(), arr)

	got := RefineByLengthLowerBound(base, 1)
	if !typ.TypeEquals(got, arr) {
		t.Fatalf("RefineByLengthLowerBound({}|array, 1) = %v, want %v", got, arr)
	}
}

func TestRefineByLengthLowerBound_RejectsClosedEmptyShape(t *testing.T) {
	got := RefineByLengthLowerBound(typ.NewRecord().Build(), 1)
	if !typ.IsNever(got) {
		t.Fatalf("RefineByLengthLowerBound({}, 1) = %v, want never", got)
	}
}
