package issuance

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
)

// TestSubjectLivenessCandidateSourceBindsTheCanonicalEntry keeps the lowering
// attached to the owner declaration. The typed source is valid only when the
// canonical relation is the exact optional occurrence-to-liveness join with
// the canonical true predicate; a same-shaped relation key elsewhere cannot
// mint this source by convention.
func TestSubjectLivenessCandidateSourceBindsTheCanonicalEntry(t *testing.T) {
	entries, entriesOK := Entries()
	if !entriesOK {
		t.Fatal("canonical Program entries refused construction")
	}
	var relation *schemaissuance.Entry
	var targetRow *schemaissuance.Entry
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.Key() == RelationOccurrenceSubjectLiveness {
			relation = entry
		}
		if entry.Key() == RowSubjectLivenessSpan {
			targetRow = entry
		}
	}
	if relation == nil || targetRow == nil {
		t.Fatal("canonical liveness relation or target row is missing")
	}
	if targetRow.Kind() != schemaissuance.KindRowSpace {
		t.Fatalf("liveness target kind = %v, want row space", targetRow.Kind())
	}
	if relation.Kind() != schemaissuance.KindRelation {
		t.Fatalf("liveness source kind = %v, want relation", relation.Kind())
	}
	if relation.Space() != RowOccurrence {
		t.Fatalf("liveness source space = %q, want occurrence row", relation.Space())
	}
	if relation.Target() != RowSubjectLivenessSpan {
		t.Fatalf("liveness source target = %q, want subject-liveness row", relation.Target())
	}
	if relation.Cardinality() != schemaissuance.CardinalityOptional {
		t.Fatalf("liveness source cardinality = %v, want optional", relation.Cardinality())
	}
	joins := relation.Joins()
	if len(joins) != 1 || joins[0] != (schemaissuance.JoinField{
		Source: FieldOccurrenceID, Target: FieldSubjectLivenessSpanID, Missing: schemaissuance.JoinMissingNoEdge,
	}) {
		t.Fatalf("liveness source joins = %+v, want the canonical identity join", joins)
	}
	if relation.ProgramLen() != 1 || relation.Result() != 1 {
		t.Fatalf("liveness source predicate shape = program length %d/result %d, want 1/1", relation.ProgramLen(), relation.Result())
	}
	instruction, instructionOK := relation.InstructionAt(0)
	if !instructionOK || instruction.Op != schemaissuance.OpLiteral || instruction.Out != 1 ||
		instruction.Args != [6]uint16{} || instruction.Ref != "" || instruction.Aux != "" ||
		instruction.Type != schemaissuance.BoolType() || instruction.Literal != 1 {
		t.Fatalf("liveness source predicate = %+v, want canonical true literal", instruction)
	}
	source, sourceOK := CandidateSourceFor(RelationOccurrenceSubjectLiveness)
	if !sourceOK || !source.Available() {
		t.Fatal("canonical liveness relation did not produce a typed source")
	}
	if source.Relation != RelationOccurrenceSubjectLiveness || source.Subject.PackagePath != lifecyclePackagePath || source.Subject.Name != "MountedSubjectLiveness" ||
		source.Redeem.PackagePath != lifecyclePackagePath || source.Redeem.Name != "RedeemSubjectLiveness" ||
		source.Redeem.Receiver.Name != "" || source.Redeem.ReceiverPointer || source.Redeem.ResultIndex != 0 {
		t.Fatalf("canonical liveness lowering = %+v, want lifecycle source", source)
	}
}

// TestCandidateSourceRejectsUnsupportedCanonicalTargetAndRelation keeps the
// target-row selector closed. A canonical relation with another target, or an
// unknown relation key, cannot fall through to the liveness lowering.
func TestCandidateSourceRejectsUnsupportedCanonicalTargetAndRelation(t *testing.T) {
	for _, relation := range []schema.Key{
		RelationOccurrenceCall,
		"program-relation/not-canonical",
	} {
		if source, sourceOK := CandidateSourceFor(relation); sourceOK || source.Available() {
			t.Fatalf("unsupported candidate source %q admitted: %+v/%t", relation, source, sourceOK)
		}
	}
}
