package equation

import (
	"errors"
	"testing"
)

func environmentDraft() Draft {
	body := testBody(1)
	return Draft{Target: Coordinate{Body: body, Name: "mid:write:1"}, Entry: EntryParameter{Body: body, Name: "entry"}, Occurrence: Occurrence{Kind: "environment-write", ContractID: testID(2)}, Operands: []Operand{{Role: MustOperandRole("flow"), Term: ClosedTerm([]byte("flow"))}, {Role: MustOperandRole("state"), Term: ClosedTerm([]byte("values"))}, {Role: MustOperandRole("guard"), Term: ClosedTerm([]byte("guard"))}}}
}

func TestSkeletonRejectsUnimplementedKind(t *testing.T) {
	_, err := Skeleton().Compile(Source{Drafts: []Draft{{Target: environmentDraft().Target, Entry: environmentDraft().Entry, Occurrence: Occurrence{Kind: "apply", ContractID: testID(2)}, Operands: []Operand{{Role: MustOperandRole("flow"), Term: ClosedTerm([]byte("flow"))}, {Role: MustOperandRole("entry"), Term: EntryTerm(environmentDraft().Entry)}, {Role: MustOperandRole("node-entry"), Term: ClosedTerm([]byte("node"))}, {Role: MustOperandRole("callee-outcome"), Term: ClosedTerm([]byte("outcome"))}, {Role: MustOperandRole("guard"), Term: ClosedTerm([]byte("guard"))}}}}})
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
