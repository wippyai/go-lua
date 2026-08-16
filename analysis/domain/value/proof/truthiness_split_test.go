package proof

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/normalize"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestTruthinessSplitKeepsBothEdgesOfABoolean(t *testing.T) {
	truthy, falsy, ok := TruthinessSplit(typ.Boolean)
	if !ok {
		t.Fatal("TruthinessSplit(boolean) reported no split; boolean is exactly true | false")
	}
	if typ.IsNever(truthy) || typ.IsNever(falsy) {
		t.Fatalf("TruthinessSplit(boolean) = %v / %v, want both edges inhabited", truthy, falsy)
	}
	if !typ.TypeEquals(truthy, typ.LiteralBool(true)) || !typ.TypeEquals(falsy, typ.LiteralBool(false)) {
		t.Fatalf("TruthinessSplit(boolean) = %v / %v, want true / false", truthy, falsy)
	}
}

func TestTruthinessSplitProvesEachEdgeOfAClosedType(t *testing.T) {
	for _, testcase := range []struct {
		name         string
		value        typ.Type
		truthyNever  bool
		falsyNever   bool
		wantSplitted bool
	}{
		{name: "string", value: typ.String, falsyNever: true, wantSplitted: true},
		{name: "integer", value: typ.Integer, falsyNever: true, wantSplitted: true},
		{name: "record", value: typetable.NewRecord().Field("id", typ.String).Build(), falsyNever: true, wantSplitted: true},
		{name: "nil", value: typ.Nil, truthyNever: true, wantSplitted: true},
		{name: "false", value: typ.LiteralBool(false), truthyNever: true, wantSplitted: true},
		{name: "true", value: typ.LiteralBool(true), falsyNever: true, wantSplitted: true},
		{name: "any", value: typ.Any},
		{name: "unknown", value: typ.Unknown},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			truthy, falsy, ok := TruthinessSplit(testcase.value)
			if ok != testcase.wantSplitted {
				t.Fatalf("TruthinessSplit(%s) split = %v, want %v", testcase.name, ok, testcase.wantSplitted)
			}
			if !ok {
				return
			}
			if typ.IsNever(truthy) != testcase.truthyNever {
				t.Fatalf("TruthinessSplit(%s) truthy = %v, want never = %v", testcase.name, truthy, testcase.truthyNever)
			}
			if typ.IsNever(falsy) != testcase.falsyNever {
				t.Fatalf("TruthinessSplit(%s) falsy = %v, want never = %v", testcase.name, falsy, testcase.falsyNever)
			}
		})
	}
}

func TestTruthinessSplitSeparatesNilFromAnOptionalValueArm(t *testing.T) {
	truthy, falsy, ok := TruthinessSplit(normalize.Optional(typ.String))
	if !ok {
		t.Fatal("TruthinessSplit(string?) reported no split")
	}
	if !typ.TypeEquals(truthy, typ.String) {
		t.Fatalf("TruthinessSplit(string?) truthy = %v, want string", truthy)
	}
	if !typ.TypeEquals(falsy, typ.Nil) {
		t.Fatalf("TruthinessSplit(string?) falsy = %v, want nil", falsy)
	}
}

// An optional boolean is the adversarial case: its truthy edge proves nothing
// about nil-versus-false, so the falsy edge must retain both.
func TestTruthinessSplitRetainsFalseBesideNilForAnOptionalBoolean(t *testing.T) {
	truthy, falsy, ok := TruthinessSplit(normalize.Optional(typ.Boolean))
	if !ok {
		t.Fatal("TruthinessSplit(boolean?) reported no split")
	}
	if !typ.TypeEquals(truthy, typ.LiteralBool(true)) {
		t.Fatalf("TruthinessSplit(boolean?) truthy = %v, want true", truthy)
	}
	if typ.IsNever(falsy) || !typ.AdmitsFalse(falsy) {
		t.Fatalf("TruthinessSplit(boolean?) falsy = %v, want a type admitting false", falsy)
	}
	if !subtypeOfFalsy(falsy) {
		t.Fatalf("TruthinessSplit(boolean?) falsy = %v, want only nil and false", falsy)
	}
}

// subtypeOfFalsy accepts exactly Lua's falsy value set in any of its canonical
// spellings: nil, false, their union, and the optional form the union
// normalizer produces for it.
func subtypeOfFalsy(t typ.Type) bool {
	switch value := t.(type) {
	case *typ.Optional:
		return value != nil && subtypeOfFalsy(value.Inner)
	case *typ.Union:
		for _, member := range value.Members {
			if !subtypeOfFalsy(member) {
				return false
			}
		}
		return len(value.Members) != 0
	default:
		return typ.TypeEquals(t, typ.Nil) || typ.TypeEquals(t, typ.LiteralBool(false))
	}
}

func TestTruthinessSplitLiftsOverAUnionPointwise(t *testing.T) {
	value := normalize.UnionForEvidence(typ.String, typ.LiteralBool(false), typ.Nil)
	truthy, falsy, ok := TruthinessSplit(value)
	if !ok {
		t.Fatalf("TruthinessSplit(%v) reported no split", value)
	}
	if !typ.TypeEquals(truthy, typ.String) {
		t.Fatalf("TruthinessSplit(string | false | nil) truthy = %v, want string", truthy)
	}
	if typ.IsNever(falsy) || !subtypeOfFalsy(falsy) {
		t.Fatalf("TruthinessSplit(string | false | nil) falsy = %v, want nil | false", falsy)
	}
}
