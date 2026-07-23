package equation

import "testing"

func testBody(seed byte) BodyID  { var body BodyID; body[0] = seed; return body }
func testID(seed byte) ContentID { var id ContentID; id[0] = seed; return id }

func TestArtifactCanonicalContentIgnoresInputOrder(t *testing.T) {
	body := testBody(1)
	entry := EntryParameter{Body: body, Name: "entry"}
	makeEquation := func(name string) Equation {
		return Equation{Target: Coordinate{Body: body, Name: name}, Entry: entry,
			Guards:     []Guard{{Body: body, Encoding: []byte("z")}, {Body: body, Encoding: []byte("a")}},
			Occurrence: Occurrence{Kind: "environment-write", ContractID: testID(1)}, KernelID: "factapply/environment-write/v1",
			Operands: []Operand{{Role: "state", Term: ClosedTerm([]byte("state"))}, {Role: "flow", Term: ClosedTerm([]byte("flow"))}, {Role: "guard", Term: ClosedTerm([]byte("guard"))}},
		}
	}
	left := Artifact{Equations: []Equation{makeEquation("b"), makeEquation("a")}}
	right := Artifact{Equations: []Equation{makeEquation("a"), makeEquation("b")}}
	if left.ContentID() != right.ContentID() {
		t.Fatal("equation artifact retained input order")
	}
}

func TestArtifactRejectsUnboundEntryTerm(t *testing.T) {
	body := testBody(1)
	artifact := Artifact{Equations: []Equation{{Target: Coordinate{Body: body, Name: "out"}, Entry: EntryParameter{Body: body, Name: "entry"}, Occurrence: Occurrence{Kind: "apply", ContractID: testID(1)}, KernelID: "factapply/apply/v1", Operands: []Operand{{Role: "entry", Term: Term{Encoding: []byte("other"), Entry: true}}}}}}
	if artifact.CanonicalBytes() != nil {
		t.Fatal("artifact accepted an unbound entry parameter")
	}
}

func TestArtifactCanonicalContentRetainsReadinessDependencies(t *testing.T) {
	body := testBody(7)
	entry := EntryParameter{Body: body, Name: "entry"}
	base := func(name string, dependencies []Coordinate) Equation {
		return Equation{Target: Coordinate{Body: body, Name: name}, Entry: entry, Dependencies: dependencies, Occurrence: Occurrence{Kind: "entry", ContractID: testID(7)}, KernelID: "kernel", Operands: []Operand{{Role: "entry", Term: EntryTerm(entry)}}}
	}
	without := Artifact{Equations: []Equation{base("a", nil), base("b", nil)}}
	with := Artifact{Equations: []Equation{base("a", nil), base("b", []Coordinate{{Body: body, Name: "a"}})}}
	if without.ContentID() == with.ContentID() {
		t.Fatal("readiness dependency did not affect artifact content")
	}
}
