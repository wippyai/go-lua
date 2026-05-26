package value

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestCovers_RecursiveProductAdmitsCoveredObservation(t *testing.T) {
	suite := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("full_path", typ.String).
			Build()
	})
	observation := typ.NewRecord().
		Field("name", typ.String).
		Field("children", typ.NewArray(suite)).
		Field("full_path", typ.String).
		Build()

	if !Covers(suite, observation) {
		t.Fatalf("recursive product should cover its unfolded observation")
	}
}

func TestCovers_RecursiveUnionAdmitsCoveredObservation(t *testing.T) {
	suite := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	observation := typ.NewRecord().
		Field("name", typ.String).
		Field("children", typ.NewArray(suite)).
		Build()
	upper := typ.NewUnion(suite, typ.Boolean)

	if !Covers(upper, observation) {
		t.Fatalf("recursive union should cover observation through its recursive member")
	}
}

func TestCovers_AcyclicUsesSubtype(t *testing.T) {
	upper := typ.Number
	observation := typ.Integer
	if !Covers(upper, observation) {
		t.Fatalf("number should cover integer through acyclic subtype")
	}
	if Covers(observation, upper) {
		t.Fatalf("integer must not cover number")
	}
}
