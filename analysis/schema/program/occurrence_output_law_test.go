package programschema

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func occurrenceOutputLawID(t *testing.T, label string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("analysis/schema/program/occurrence-output-law", []byte(label))
	if !ok {
		t.Fatalf("derive %s", label)
	}
	return id
}

// occurrenceOutputLawProgram seals one parent occurrence over a dense operand
// plane and returns the Program that owns both.
func occurrenceOutputLawProgram(t *testing.T, kind OccurrenceKind, id identity.ContentID, operands []identity.ContentID) Program {
	t.Helper()
	inputs := make([]OccurrenceInput, len(operands))
	for index, operand := range operands {
		var ok bool
		inputs[index], ok = NewOccurrenceInput(operand)
		if !ok {
			t.Fatalf("operand %d", index)
		}
	}
	point, pointOK := NewOccurrencePoint(occurrenceOutputLawID(t, "point"))
	if !pointOK {
		t.Fatal("point")
	}
	occurrence, occurrenceOK := NewOccurrence(
		kind, id, occurrenceOutputLawID(t, "body"), 0,
		0, 1, 0, uint32(len(inputs)),
		keyspace.FamilyInvalid, keyspace.LiteralValue{}, false,
	)
	if !occurrenceOK {
		t.Fatalf("occurrence kind %d rejected with %d operands", kind, len(inputs))
	}
	catalog, _ := CatalogID(occurrenceOutputLawID(t, "schema"))
	frozen, sealed := (Publication{
		Occurrences:      []Occurrence{occurrence},
		OccurrencePoints: []OccurrencePoint{point},
		OccurrenceInputs: inputs,
	}).Seal(catalog, identity.StoreID(1))
	program := Program{
		Frozen: frozen, ArtifactID: occurrenceOutputLawID(t, "artifact"),
		ProgramID: occurrenceOutputLawID(t, "program"), SchemaID: occurrenceOutputLawID(t, "schema"),
	}
	if !sealed || !program.Available() {
		t.Fatal("publication")
	}
	return program
}

// A storage write carries the operation under its own identity. The value it
// establishes is named in its operand vector, so reading the occurrence
// identity as the output would attribute a value to a point that never
// produced it.
func TestStorageWriteOutputIsItsOperandNotItsOwnIdentity(t *testing.T) {
	id := occurrenceOutputLawID(t, "write")
	operands := []identity.ContentID{
		occurrenceOutputLawID(t, "assignment"),
		occurrenceOutputLawID(t, "written-value"),
		occurrenceOutputLawID(t, "cell"),
		occurrenceOutputLawID(t, "predecessor"),
		occurrenceOutputLawID(t, "route"),
	}
	program := occurrenceOutputLawProgram(t, OccurrenceStorageWrite, id, operands)
	output, produces := program.OccurrenceOutputSemanticID(0)
	if !produces {
		t.Fatal("a storage write establishes no value")
	}
	if output == id {
		t.Fatal("a storage write's output is its own occurrence identity")
	}
	if output != operands[occurrenceOutputOperand] {
		t.Fatalf("storage write output = %s, want operand %d %s", output, occurrenceOutputOperand, operands[occurrenceOutputOperand])
	}
}

// The same law holds for every family that names its output in the operand
// vector, and a value-producing family still answers with its own identity.
func TestOccurrenceOutputIdentityFollowsItsFamily(t *testing.T) {
	cell := occurrenceOutputLawID(t, "operand-output")
	operands := []identity.ContentID{
		occurrenceOutputLawID(t, "operand-zero"),
		occurrenceOutputLawID(t, "operand-one"),
		cell,
	}
	for _, kind := range []OccurrenceKind{OccurrenceStorageBindTransfer, OccurrenceIndexRead} {
		program := occurrenceOutputLawProgram(t, kind, occurrenceOutputLawID(t, "operand-family"), operands)
		output, produces := program.OccurrenceOutputSemanticID(0)
		if !produces || output != cell {
			t.Fatalf("kind %d output = %s/%v, want %s", kind, output, produces, cell)
		}
	}

	source := occurrenceOutputLawID(t, "value-source")
	program := occurrenceOutputLawProgram(t, OccurrenceValueSource, source, operands[:1])
	output, produces := program.OccurrenceOutputSemanticID(0)
	if !produces || output != source {
		t.Fatalf("value source output = %s/%v, want %s", output, produces, source)
	}
}

// The occurrence vocabulary is closed and most of it establishes no value. A
// non-producing kind must never be readable as a producer: the consumer would
// mint a producer at an execution point the observation cannot anchor to.
func TestNonValueProducingOccurrenceKindsPublishNoOutput(t *testing.T) {
	producing := map[OccurrenceKind]struct{}{
		OccurrenceValueSource: {}, OccurrenceFormalEntry: {}, OccurrenceStorageRead: {},
		OccurrenceBinaryEquality: {}, OccurrenceBinaryArithmetic: {}, OccurrenceBinaryOrder: {},
		OccurrenceStorageBindTransfer: {}, OccurrenceStorageWrite: {}, OccurrenceIndexRead: {},
	}
	operands := []identity.ContentID{
		occurrenceOutputLawID(t, "closed-zero"),
		occurrenceOutputLawID(t, "closed-one"),
		occurrenceOutputLawID(t, "closed-two"),
		occurrenceOutputLawID(t, "closed-three"),
	}
	id := occurrenceOutputLawID(t, "closed-kind")
	for kind := OccurrencePointAttachment; kind <= OccurrenceOperationPredicateRefinement; kind++ {
		if _, expected := producing[kind]; expected {
			continue
		}
		occurrence, ok := NewOccurrence(kind, id, occurrenceOutputLawID(t, "body"), 0, 0, 1, 0, uint32(len(operands)), keyspace.FamilyInvalid, keyspace.LiteralValue{}, false)
		if !ok {
			continue
		}
		catalog, _ := CatalogID(occurrenceOutputLawID(t, "schema"))
		inputs := make([]OccurrenceInput, len(operands))
		for index, operand := range operands {
			inputs[index], _ = NewOccurrenceInput(operand)
		}
		point, _ := NewOccurrencePoint(occurrenceOutputLawID(t, "point"))
		frozen, sealed := (Publication{
			Occurrences:      []Occurrence{occurrence},
			OccurrencePoints: []OccurrencePoint{point},
			OccurrenceInputs: inputs,
		}).Seal(catalog, identity.StoreID(1))
		program := Program{
			Frozen: frozen, ArtifactID: occurrenceOutputLawID(t, "artifact"),
			ProgramID: occurrenceOutputLawID(t, "program"), SchemaID: occurrenceOutputLawID(t, "schema"),
		}
		if !sealed || !program.Available() {
			t.Fatalf("publication for kind %d", kind)
		}
		if output, produces := program.OccurrenceOutputSemanticID(0); produces {
			t.Fatalf("kind %d published output %s, but it establishes no value", kind, output)
		}
	}

	if _, produces := (Program{}).OccurrenceOutputSemanticID(0); produces {
		t.Fatal("an unavailable program published an occurrence output")
	}
}

// A family that names its output in the operand vector cannot be sealed
// without that operand: the owner refuses the row rather than publishing an
// occurrence whose output is unreadable.
func TestOperandOutputOccurrenceRequiresItsOutputOperand(t *testing.T) {
	id := occurrenceOutputLawID(t, "short")
	for _, kind := range []OccurrenceKind{OccurrenceStorageBindTransfer, OccurrenceStorageWrite, OccurrenceIndexRead} {
		if _, ok := NewOccurrence(kind, id, occurrenceOutputLawID(t, "body"), 0, 0, 1, 0, occurrenceOutputOperand, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false); ok {
			t.Fatalf("kind %d sealed with no output operand", kind)
		}
	}
}
