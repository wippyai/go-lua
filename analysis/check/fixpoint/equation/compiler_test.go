package equation

import (
	"errors"
	"testing"
)

func environmentDraft() Draft {
	body := testBody(1)
	return Draft{Target: Coordinate{Body: body, Name: "mid:write:1"}, Entry: EntryParameter{Body: body, Name: "entry"}, Occurrence: Occurrence{Kind: "environment-write", ContractID: testID(2)}, Operands: []Operand{{Role: "flow", Term: ClosedTerm([]byte("flow"))}, {Role: "state", Term: ClosedTerm([]byte("values"))}, {Role: "guard", Term: ClosedTerm([]byte("guard"))}}}
}

func TestSkeletonRejectsUnimplementedKind(t *testing.T) {
	_, err := Skeleton().Compile(Source{Drafts: []Draft{{Target: environmentDraft().Target, Entry: environmentDraft().Entry, Occurrence: Occurrence{Kind: "apply", ContractID: testID(2)}, Operands: []Operand{{Role: "flow", Term: ClosedTerm([]byte("flow"))}, {Role: "entry", Term: EntryTerm(environmentDraft().Entry)}, {Role: "node-entry", Term: ClosedTerm([]byte("node"))}, {Role: "callee-outcome", Term: ClosedTerm([]byte("outcome"))}, {Role: "guard", Term: ClosedTerm([]byte("guard"))}}}}})
	if !errors.Is(err, ErrUnimplementedLowering) {
		t.Fatalf("error = %v, want unimplemented lowering", err)
	}
}

func TestEnvironmentWriteExemplarBindsExistingKernel(t *testing.T) {
	compiler, err := Skeleton().With("environment-write", "transformer/formal-environment-write/v1")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := compiler.Compile(Source{Drafts: []Draft{environmentDraft()}})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Equations) != 1 || artifact.Equations[0].KernelID != "transformer/formal-environment-write/v1" {
		t.Fatalf("unexpected lowered artifact: %#v", artifact)
	}
}

func TestAuditHarnessRejectsPartialPublication(t *testing.T) {
	verified := false
	err := RunAndVerify(
		func() (Execution, error) { return Execution{Published: true}, nil },
		func(AccessRecord) error {
			verified = true
			return nil
		},
	)
	if err != ErrPartialExecution {
		t.Fatalf("Complete=false, Published=true, err=nil produced %v, want %v", err, ErrPartialExecution)
	}
	if verified {
		t.Fatal("partial execution reached audit verification")
	}
}
