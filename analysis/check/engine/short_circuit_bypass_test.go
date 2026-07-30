package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// bypassOperation builds the branch a value-position short-circuit guards: the
// front states which cell the result occupies, which operand it carries there,
// and which edge carries it.
func bypassOperation(t *testing.T, operand string, edge string) equation.BoundEquation {
	t.Helper()
	return equation.BoundEquation{
		Target: equation.Coordinate{Body: equation.BodyID{1}, Name: "op-00000002"},
		Operands: []equation.BoundOperand{
			{Role: equation.MustOperandRole("short-circuit-result"), Value: []byte("temp/0")},
			{Role: equation.MustOperandRole("short-circuit-operand"), Value: []byte(operand)},
			{Role: equation.MustOperandRole("short-circuit-bypass"), Value: []byte("scalar/bool/" + edge)},
		},
	}
}

func bypassProjection(t *testing.T, facts []equation.Fact, edge string) typ.Type {
	t.Helper()
	if len(facts) != 1 {
		t.Fatalf("bypass publications = %d, want 1", len(facts))
	}
	fact := facts[0]
	if fact.Key != "value/temp/0/op-00000002" {
		t.Fatalf("bypass key = %q, want the result cell's row at this decision", fact.Key)
	}
	if len(fact.Guards) != 1 || string(fact.Guards[0].Encoding) != "front/branch/op-00000002/"+edge {
		t.Fatalf("bypass guards = %v, want only the %s edge of this decision", fact.Guards, edge)
	}
	projection, decoded := shapefact.DecodeTarget(fact.Value)
	if !decoded {
		t.Fatalf("bypass value %q is not a type witness", fact.Value)
	}
	return projection
}

func bypassFacts(t *testing.T, operandType typ.Type, edge string) []equation.Fact {
	t.Helper()
	encoded, ok := shapefact.EncodeTarget(operandType)
	if !ok {
		t.Fatalf("encode %v", operandType)
	}
	partition := joinTestPartition(t, nil, equation.Fact{Key: "value/path/a/op-00000001", Value: encoded})
	facts, err := shortCircuitBypassFacts(bypassOperation(t, "path/a", edge), partition)
	if err != nil {
		t.Fatalf("shortCircuitBypassFacts: %v", err)
	}
	return facts
}

// TestAndBypassCarriesOnlyTheFalsyProjection is the headline case: the record
// side of an optional record is what makes the guard take the other edge, so
// `a and b` can never yield it.
func TestAndBypassCarriesOnlyTheFalsyProjection(t *testing.T) {
	record := &typ.Record{Fields: []typ.Field{{Name: "suite", Type: normalize.Optional(typ.String)}}}
	projection := bypassProjection(t, bypassFacts(t, normalize.Optional(record), "false"), "false")
	if !typ.TypeEquals(projection, typ.Nil) {
		t.Fatalf("and bypass projection = %v, want nil", projection)
	}
}

// TestOrBypassCarriesOnlyTheTruthyProjection is the mirror: `x or default`
// reaches its bypass edge exactly when x is truthy, so the nil is gone.
func TestOrBypassCarriesOnlyTheTruthyProjection(t *testing.T) {
	projection := bypassProjection(t, bypassFacts(t, normalize.Optional(typ.String), "true"), "true")
	if !typ.TypeEquals(projection, typ.String) {
		t.Fatalf("or bypass projection = %v, want string", projection)
	}
}

// TestBooleanBypassKeepsFalse pins the edge of the partition that is easiest to
// lose: false is falsy, so falsy(boolean) is false and not the empty set.
func TestBooleanBypassKeepsFalse(t *testing.T) {
	projection := bypassProjection(t, bypassFacts(t, typ.Boolean, "false"), "false")
	if !typ.TypeEquals(projection, typ.False) {
		t.Fatalf("and bypass projection of boolean = %v, want false", projection)
	}
	truthy := bypassProjection(t, bypassFacts(t, typ.Boolean, "true"), "true")
	if !typ.TypeEquals(truthy, typ.True) {
		t.Fatalf("or bypass projection of boolean = %v, want true", truthy)
	}
}

// TestOptionalBooleanBypassKeepsBothFalsyMembers keeps a `false | nil` operand
// whole: both members reach the bypass edge of an `and`.
func TestOptionalBooleanBypassKeepsBothFalsyMembers(t *testing.T) {
	projection := bypassProjection(t, bypassFacts(t, normalize.Optional(typ.Boolean), "false"), "false")
	want := normalize.UnionForEvidence(typ.Nil, typ.False)
	if !typ.TypeEquals(projection, want) {
		t.Fatalf("and bypass projection of boolean? = %v, want %v", projection, want)
	}
}

// TestBypassWithoutATypeWitnessPublishesNothing keeps the fail-closed rule: an
// operand the value lane cannot type leaves the pre-guard row standing, which is
// the widest answer rather than a wrong one.
func TestBypassWithoutATypeWitnessPublishesNothing(t *testing.T) {
	partition := joinTestPartition(t, nil, equation.Fact{Key: "value/path/a/op-00000001", Value: []byte("scalar/top")})
	facts, err := shortCircuitBypassFacts(bypassOperation(t, "path/a", "false"), partition)
	if err != nil {
		t.Fatalf("shortCircuitBypassFacts: %v", err)
	}
	if len(facts) != 0 {
		t.Fatalf("bypass publications for an untyped operand = %v, want none", facts)
	}
}

// TestBypassOfADecidedOperandPublishesNothing covers the empty projection: a
// value whose falsy side is empty decides the guard, and the arm rule owns that.
func TestBypassOfADecidedOperandPublishesNothing(t *testing.T) {
	facts := bypassFacts(t, typ.String, "false")
	if len(facts) != 0 {
		t.Fatalf("bypass publications for a wholly truthy operand = %v, want none", facts)
	}
}

// TestEqualityOnAnAbstractWitnessIsUndecided states what a published type is: a
// value set, not a value. Two runs reaching the same witness can carry different
// values, so the comparison decides nothing unless the sets are disjoint.
func TestEqualityOnAnAbstractWitnessIsUndecided(t *testing.T) {
	union, ok := shapefact.EncodeTarget(normalize.UnionForEvidence(typ.LiteralString("group"), typ.LiteralString("text")))
	if !ok {
		t.Fatal(`encode "group" | "text"`)
	}
	value, err := basicBinary(wir.BinEq, union, []byte("scalar/string/\"text\""))
	if err != nil {
		t.Fatalf("basicBinary: %v", err)
	}
	if string(value) != "scalar/top" {
		t.Fatalf(`("group" | "text") == "text" = %q, want an undecided result`, value)
	}
	inequality, err := basicBinary(wir.BinNe, union, []byte("scalar/string/\"text\""))
	if err != nil {
		t.Fatalf("basicBinary: %v", err)
	}
	if string(inequality) != "scalar/top" {
		t.Fatalf(`("group" | "text") ~= "text" = %q, want an undecided result`, inequality)
	}
}

// TestEqualityOnDisjointWitnessesIsRefuted keeps the decision the type domain
// does prove: value sets with no common inhabitant can never be equal.
func TestEqualityOnDisjointWitnessesIsRefuted(t *testing.T) {
	group, ok := shapefact.EncodeTarget(typ.LiteralString("group"))
	if !ok {
		t.Fatal(`encode "group"`)
	}
	value, err := basicBinary(wir.BinEq, group, []byte("scalar/string/\"text\""))
	if err != nil {
		t.Fatalf("basicBinary: %v", err)
	}
	if string(value) != "scalar/bool/false" {
		t.Fatalf(`"group" == "text" = %q, want false`, value)
	}
	inequality, err := basicBinary(wir.BinNe, group, []byte("scalar/string/\"text\""))
	if err != nil {
		t.Fatalf("basicBinary: %v", err)
	}
	if string(inequality) != "scalar/bool/true" {
		t.Fatalf(`"group" ~= "text" = %q, want true`, inequality)
	}
}
