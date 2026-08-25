package resultpolicy

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func policyID(seed byte) identity.ContentID {
	var id identity.ContentID
	id[0] = seed
	return id
}

func exactPolicyRow(t testing.TB, occurrence, subject identity.ContentID, role programschema.ExactScalarSummaryRole, value int64) programschema.ExactScalarSummary {
	t.Helper()
	row, ok := programschema.NewExactScalarSummary(occurrence, subject, policyID(90), role, programschema.SummaryLiteral{Kind: uint8(keyspace.LiteralInteger), Integer: value})
	if !ok {
		t.Fatal("exact summary")
	}
	return row
}

func integerLiteral(value int64) keyspace.LiteralValue {
	return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}
}

// TestSealStatesOnePolicyForEveryArithmeticOccurrence is the closure law. The
// operand roles are the proof: Program emits an exact row per retained literal
// per role and emits none for a role whose image it could not close, so both
// operand roles present is a closed Cartesian image and anything less is open.
// An occurrence Program mentions in no exact row at all is still an occurrence
// and still has a policy - an open one - because the seal that consumes this
// directory must find an answer for every arithmetic row it walks.
func TestSealStatesOnePolicyForEveryArithmeticOccurrence(t *testing.T) {
	closedOccurrence, partialOccurrence, silentOccurrence := policyID(1), policyID(2), policyID(3)
	directory, ok := sealRows(
		[]identity.ContentID{closedOccurrence, partialOccurrence, silentOccurrence},
		[]programschema.ExactScalarSummary{
			exactPolicyRow(t, closedOccurrence, policyID(4), programschema.ExactScalarSummaryLeft, 1),
			exactPolicyRow(t, closedOccurrence, policyID(5), programschema.ExactScalarSummaryRight, 1),
			exactPolicyRow(t, closedOccurrence, closedOccurrence, programschema.ExactScalarSummaryResult, 2),
			exactPolicyRow(t, partialOccurrence, policyID(6), programschema.ExactScalarSummaryRight, 1),
		},
	)
	if !ok {
		t.Fatal("seal policies")
	}
	closed, closedOK := directory.For(closedOccurrence)
	if !closedOK || !closed.Closed() || !closed.Admits(integerLiteral(2)) {
		t.Fatalf("closed policy = %#v/%v", closed, closedOK)
	}
	if closed.Admits(integerLiteral(3)) {
		t.Fatal("closed policy admitted a result outside its sealed image")
	}
	for _, occurrence := range []identity.ContentID{partialOccurrence, silentOccurrence} {
		open, openOK := directory.For(occurrence)
		if !openOK || open.Closed() || open.Admits(integerLiteral(2)) {
			t.Fatalf("open policy = %#v/%v", open, openOK)
		}
	}
	if _, foreignOK := directory.For(policyID(7)); foreignOK {
		t.Fatal("directory answered for an occurrence it never sealed")
	}
}

// TestUnsealedPolicyIsNotAnOpenPolicy states that the zero value carries no
// contract. An open occurrence is a sealed answer about a program; a zero
// Policy is the absence of one, and a consumer that treated the two alike
// would run arithmetic on a row the seal never reached.
func TestUnsealedPolicyIsNotAnOpenPolicy(t *testing.T) {
	var unsealed Policy
	if unsealed.Available() || unsealed.Closed() || unsealed.Admits(integerLiteral(0)) {
		t.Fatalf("zero policy = %#v", unsealed)
	}
	if open := OpenImage(); !open.Available() || open.Closed() {
		t.Fatalf("open policy = %#v", open)
	}
}

// TestClosedImageOwnsItsRosterAfterConstruction states that a sealed roster is
// immutable: the caller's slice is not the policy's, whatever order it arrived
// in, and an empty roster is a real closed answer - every pair of the product
// traps - rather than an absent one.
func TestClosedImageOwnsItsRosterAfterConstruction(t *testing.T) {
	roster := []keyspace.LiteralValue{integerLiteral(9), integerLiteral(-4), integerLiteral(0)}
	policy := ClosedImage(roster...)
	roster[0] = integerLiteral(99)
	for _, literal := range []keyspace.LiteralValue{integerLiteral(9), integerLiteral(-4), integerLiteral(0)} {
		if !policy.Admits(literal) {
			t.Fatalf("sealed roster lost %v", literal)
		}
	}
	if policy.Admits(integerLiteral(99)) {
		t.Fatal("caller mutated a sealed roster")
	}
	empty := ClosedImage()
	if !empty.Available() || !empty.Closed() || empty.Admits(integerLiteral(0)) {
		t.Fatalf("empty closed image = %#v", empty)
	}
}

// TestSealRefusesAnExactRowItCannotAttribute states the two malformed shapes.
// A result roster without both complete operands is not a Cartesian image, and
// an exact row naming an occurrence that is not a published binary-arithmetic
// row - or a result whose subject is not its own occurrence - is a Program the
// directory must refuse rather than describe.
func TestSealRefusesAnExactRowItCannotAttribute(t *testing.T) {
	occurrence, foreign := policyID(1), policyID(2)
	left := exactPolicyRow(t, occurrence, policyID(3), programschema.ExactScalarSummaryLeft, 1)
	right := exactPolicyRow(t, occurrence, policyID(4), programschema.ExactScalarSummaryRight, 1)
	cases := []struct {
		name        string
		occurrences []identity.ContentID
		exact       []programschema.ExactScalarSummary
	}{
		{
			name:        "result-without-operands",
			occurrences: []identity.ContentID{occurrence},
			exact:       []programschema.ExactScalarSummary{exactPolicyRow(t, occurrence, occurrence, programschema.ExactScalarSummaryResult, 2)},
		},
		{
			name:        "unpublished-occurrence",
			occurrences: []identity.ContentID{occurrence},
			exact:       []programschema.ExactScalarSummary{left, right, exactPolicyRow(t, foreign, foreign, programschema.ExactScalarSummaryResult, 2)},
		},
		{
			name:        "foreign-result-subject",
			occurrences: []identity.ContentID{occurrence},
			exact:       []programschema.ExactScalarSummary{left, right, exactPolicyRow(t, occurrence, foreign, programschema.ExactScalarSummaryResult, 2)},
		},
		{
			name:        "duplicate-occurrence",
			occurrences: []identity.ContentID{occurrence, occurrence},
			exact:       []programschema.ExactScalarSummary{left, right},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, ok := sealRows(test.occurrences, test.exact); ok {
				t.Fatal("malformed exact column admitted")
			}
		})
	}
}
