package refinement

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestCanBeFalseUsesTypeWitness(t *testing.T) {
	reg := standard.Registry()
	record := typetable.NewRecord().Field("kind", typ.String).Build()
	if CanBeFalse(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, record), record)) {
		t.Fatal("record value can be false")
	}
	if !CanBeFalse(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Boolean), typ.Boolean)) {
		t.Fatal("boolean value cannot be false")
	}
	if !CanBeFalse(reg, product.Top()) {
		t.Fatal("unknown value cannot be false")
	}
}

func TestLiteralTypeRequiresLiteralWitness(t *testing.T) {
	reg := standard.Registry()
	lit := typ.LiteralString("ready")
	got, ok := LiteralType(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, lit), lit))
	if !ok || !typ.TypeEquals(got, lit) {
		t.Fatalf("LiteralType = %v/%v, want %v", got, ok, lit)
	}
	if got, ok := LiteralType(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)); ok {
		t.Fatalf("LiteralType(string) = %v, want !ok", got)
	}
}

func TestMeetConstraintRecoversCompatibleWitnessRefinement(t *testing.T) {
	reg := standard.Registry()
	value := typevalue.FromType(reg, typeexpr.Union(typ.String, typ.Number))
	constraint := typevalue.FromType(reg, typ.String)

	got := MeetConstraint(reg, value, constraint)
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("refined type = %v/%v, want string", gotType, ok)
	}
}
