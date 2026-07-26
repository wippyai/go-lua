package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// eventRecordType is the declaration a returned local is checked against: a
// record whose optional member the constructor literal leaves absent.
func eventRecordType() *typ.Record {
	return typetable.NewRecord().
		Field("kind", typ.String).
		Field("payload", typ.MaterializeOptional(typ.String)).
		Build()
}

// metricLiteralValue is the sealed shape one constructor produced for that
// declaration: a singleton kind and a member proven absent.
func metricLiteralValue(t *testing.T) []byte {
	t.Helper()
	encoded, ok := shapefact.EncodeTable(shapefact.Table{
		Closed: true,
		Members: []shapefact.Member{
			{Suffix: ".kind", Present: true, Value: `scalar/string/"metric"`},
			{Suffix: ".payload"},
		},
	})
	if !ok {
		t.Fatal("encode sealed constructor literal")
	}
	return encoded
}

func returnedTermPartition(t *testing.T, facts ...equation.Fact) equation.Partition {
	t.Helper()
	partition, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: facts})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	return partition
}

func declarationFact(t *testing.T, term, operation string, declared typ.Type) equation.Fact {
	t.Helper()
	encoded, ok := shapefact.EncodeTarget(declared)
	if !ok {
		t.Fatal("encode declaration")
	}
	return equation.Fact{Key: "type/" + term + "/" + operation, Value: encoded}
}

// TestDeclaredContainerPublicationOutranksTheConstructorLiteral states the rule
// a returned container carries out of its body: every write to the cell was
// checked against the declaration, so the declaration is the honest summary and
// the singleton members one constructor happened to produce are not.
func TestDeclaredContainerPublicationOutranksTheConstructorLiteral(t *testing.T) {
	declared := eventRecordType()
	partition := returnedTermPartition(t, declarationFact(t, "path/sym4", "op-00000005", declared))

	published, wins := declaredContainerPublication([]byte("path/sym4"), metricLiteralValue(t), partition)
	if !wins {
		t.Fatal("a checked container declaration did not reach the return publication")
	}
	surface, decoded := shapefact.DecodeTarget(published)
	if !decoded || !typ.TypeEquals(surface, declared) {
		t.Fatalf("published surface = %v, want the declared record", surface)
	}
}

// TestUnannotatedReturnedTermKeepsItsLiteralShape is the fail-closed half:
// nothing checked the cell, so nothing else describes it.
func TestUnannotatedReturnedTermKeepsItsLiteralShape(t *testing.T) {
	partition := returnedTermPartition(t)
	if published, wins := declaredContainerPublication([]byte("path/sym4"), metricLiteralValue(t), partition); wins {
		t.Fatalf("an undeclared term published %q instead of its literal shape", published)
	}
}

// TestEntryDeclarationDoesNotDecideAReturnedFormal keeps a formal's boundary
// contract out of this lane: the caller's own argument is the more precise
// authority for the cell a formal binds.
func TestEntryDeclarationDoesNotDecideAReturnedFormal(t *testing.T) {
	partition := returnedTermPartition(t, declarationFact(t, "path/sym4", "entry", eventRecordType()))
	if published, wins := declaredContainerPublication([]byte("path/sym4"), metricLiteralValue(t), partition); wins {
		t.Fatalf("an entry boundary declaration widened a returned formal to %q", published)
	}
}

// TestOptionalDeclarationDoesNotRetractAProvenPresence keeps the value lane
// where the declaration is strictly weaker about nilability than the sealed
// value the body proved.
func TestOptionalDeclarationDoesNotRetractAProvenPresence(t *testing.T) {
	optional := typ.MaterializeOptional(eventRecordType())
	partition := returnedTermPartition(t, declarationFact(t, "path/sym4", "op-00000005", optional))
	if published, wins := declaredContainerPublication([]byte("path/sym4"), metricLiteralValue(t), partition); wins {
		t.Fatalf("an optional declaration retracted a proven presence, publishing %q", published)
	}
}

// TestScalarDeclarationDoesNotDecideAReturnedValue keeps branch narrowing on a
// scalar cell: a declaration cannot express the arm a guard proved.
func TestScalarDeclarationDoesNotDecideAReturnedValue(t *testing.T) {
	partition := returnedTermPartition(t, declarationFact(t, "path/sym4", "op-00000005", typ.String))
	if published, wins := declaredContainerPublication([]byte("path/sym4"), []byte(`scalar/string/"metric"`), partition); wins {
		t.Fatalf("a scalar declaration displaced a narrowed value with %q", published)
	}
}

// TestNilValuedTermIsNeverRepublishedAsItsDeclaration is the soundness floor: a
// container declaration must not assert a table where the cell holds nil.
func TestNilValuedTermIsNeverRepublishedAsItsDeclaration(t *testing.T) {
	partition := returnedTermPartition(t, declarationFact(t, "path/sym4", "op-00000005", eventRecordType()))
	if published, wins := declaredContainerPublication([]byte("path/sym4"), []byte("scalar/nil"), partition); wins {
		t.Fatalf("a nil cell was republished as its declaration: %q", published)
	}
}

func callableContractPartition(t *testing.T, handleOperation, contractOperation string) equation.Partition {
	t.Helper()
	encoded, err := typ.EncodeCanonical(context.Background(), eventRecordType())
	if err != nil {
		t.Fatalf("encode derived result contract: %v", err)
	}
	return returnedTermPartition(t,
		equation.Fact{Key: "closure/path/sym2/" + handleOperation, Value: []byte(`{"prototype":"chunk.fn0#1"}`)},
		equation.Fact{Key: inferredCallableReturnPrefix + "path/sym2/" + contractOperation, Value: encoded},
	)
}

// TestInferredCallableReturnReadsItsOwnAllocation pins the pairing: the
// allocation states the capability and the result it derived together.
func TestInferredCallableReturnReadsItsOwnAllocation(t *testing.T) {
	result, stated := inferredCallableReturn([]byte("path/sym2"), callableContractPartition(t, "op-00000002", "op-00000002"))
	if !stated || !typ.TypeEquals(result, eventRecordType()) {
		t.Fatalf("derived result = %v (stated %v), want the allocation's own contract", result, stated)
	}
}

// TestSupersededCallableContractStopsAnsweringForItsReplacement is the
// falsifiable half: a write that rebinds the term republishes the capability
// without a contract, and the previous closure's result must not survive it.
func TestSupersededCallableContractStopsAnsweringForItsReplacement(t *testing.T) {
	result, stated := inferredCallableReturn([]byte("path/sym2"), callableContractPartition(t, "op-00000005", "op-00000002"))
	if stated {
		t.Fatalf("a superseded closure's contract answered for its replacement: %v", result)
	}
}
