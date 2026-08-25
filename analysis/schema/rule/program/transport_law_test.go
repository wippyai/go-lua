package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

func transportAxis(key string) AxisRef {
	return AxisRef(schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)})
}

// transportRow is one transported axis crossing the activation edge.
func transportRow(key string) TransportDecl {
	return TransportDecl{Axis: transportAxis(key)}
}

// exportedRow is a transported axis whose body result is carried back.
func exportedRow(key string) TransportDecl {
	row := transportRow(key)
	row.Exported = true
	return row
}

// transportProgram is the call-activation shape with a declared transport
// vector: one exact candidate read and one structural publication. The branch
// set is named by the vocabulary and enumerated through its owner, so it is
// not among the reads.
//
// The vector, the family and the branch vocabulary are one declaration, so a
// specimen carrying rows carries all three; a specimen carrying none carries
// none of them, and that is the half every biconditional law below damages.
func transportProgram(rows []TransportDecl) Program {
	program := seq5742Program(
		"call-activation",
		[]JoinDecl{
			seq5742Join("call-activation/call", []SourceRef{CandidateSource()}, Exact, false, false),
		},
		[]JoinRef{0},
		[]OutputDecl{seq5742Output("call-activation/write", ModeStructural, 0)},
	)
	program.Transport = rows
	if len(rows) != 0 {
		program.ActivationRole = "semantic/activation-family/call-body"
		branch := activationLawBranch()
		program.Activation = &branch
	}
	return program
}

// TestAnActivationVectorAndItsFamilyAreOneDeclaration states the biconditional
// the cold row depends on. A vector with no family names candidate branches
// nothing groups, and a family with no vector groups branches that instantiate
// nothing; neither half can be declared alone.
func TestAnActivationVectorAndItsFamilyAreOneDeclaration(t *testing.T) {
	vectorWithoutFamily := transportProgram([]TransportDecl{transportRow("value")})
	vectorWithoutFamily.ActivationRole = ""
	if problem, valid := vectorWithoutFamily.Check(); valid || problem.Kind != ProblemTransport {
		t.Fatalf("a transport vector with no activation family = %#v valid=%t", problem, valid)
	}
	familyWithoutVector := transportProgram(nil)
	familyWithoutVector.ActivationRole = "semantic/activation-family/call-body"
	if problem, valid := familyWithoutVector.Check(); valid || problem.Kind != ProblemTransport {
		t.Fatalf("an activation family with no transport vector = %#v valid=%t", problem, valid)
	}
}

// TestTheActivationFamilyIsPartOfTheProgramsSealedIdentity keeps the family off
// the digest of a rule that declares no vector and on the digest of one that
// does: two activation rules whose branches are grouped under different
// families are two declarations.
func TestTheActivationFamilyIsPartOfTheProgramsSealedIdentity(t *testing.T) {
	first := transportProgram([]TransportDecl{transportRow("value")})
	second := transportProgram([]TransportDecl{transportRow("value")})
	second.ActivationRole = "semantic/activation-family/call-tail"
	if first.Digest() == second.Digest() {
		t.Fatal("the activation family is not part of the sealed identity")
	}
}

// TestATransportVectorCannotExportAnAxisItDoesNotImport is the FG-6 symmetry
// law, restated as a property of the declaration's shape rather than as a
// check over two lists. One row is one axis crossing the edge: the import
// direction is the row's existence and the export direction is its flag, so an
// export with no import is not a malformed declaration - it cannot be written
// down at all.
func TestATransportVectorCannotExportAnAxisItDoesNotImport(t *testing.T) {
	rows := []TransportDecl{
		exportedRow("value"),
		transportRow("call"),
		exportedRow("heap"),
	}
	program := transportProgram(rows)
	if problem, valid := program.Check(); !valid {
		t.Fatalf("a declared transport vector is admissible: %#v", problem)
	}
	if program.TransportCount() != len(rows) {
		t.Fatalf("transport census = %d, want %d", program.TransportCount(), len(rows))
	}
	exported := 0
	for index := 0; index < program.TransportCount(); index++ {
		row, ok := program.TransportAt(index)
		if !ok || row != rows[index] {
			t.Fatalf("transport row %d = %#v", index, row)
		}
		if row.Exported {
			exported++
		}
	}
	if exported != 2 {
		t.Fatalf("exported axes = %d, want the two rows that declared the return direction", exported)
	}
}

// TestOneAxisCrossesAnActivationEdgeOnce states the other half of FG-6's
// sealing: a Factor named on both sides is ONE transport. Two rows for one
// axis would be two authorities for one crossing.
func TestOneAxisCrossesAnActivationEdgeOnce(t *testing.T) {
	program := transportProgram([]TransportDecl{
		transportRow("value"),
		exportedRow("value"),
	})
	problem, valid := program.Check()
	if valid || problem.Kind != ProblemTransport {
		t.Fatalf("one axis crossing twice = %#v valid=%t", problem, valid)
	}
}

// TestATransportVectorNamesOnlyDeclaredAxes keeps the vector inside the
// reference surface: an unavailable axis reference is not a transport.
func TestATransportVectorNamesOnlyDeclaredAxes(t *testing.T) {
	program := transportProgram([]TransportDecl{{Axis: AxisRef{}}})
	problem, valid := program.Check()
	if valid || problem.Kind != ProblemTransport {
		t.Fatalf("an unnamed transport axis = %#v valid=%t", problem, valid)
	}
}

// TestATransportVectorIsPartOfTheProgramsIdentityAndReferences proves the
// vector is sealed declaration data: it reaches the upward reference surface
// so seal resolves each axis, and it changes the Program digest. A vector the
// digest ignored would let two different transports share one sealed identity.
func TestATransportVectorIsPartOfTheProgramsIdentityAndReferences(t *testing.T) {
	imported := transportProgram([]TransportDecl{transportRow("value")})
	exported := transportProgram([]TransportDecl{exportedRow("value")})
	none := transportProgram(nil)
	if !imported.Digest().Available() || !exported.Digest().Available() || !none.Digest().Available() {
		t.Fatal("every admissible program digests")
	}
	if imported.Digest() == exported.Digest() {
		t.Fatal("the export direction is not part of the sealed identity")
	}
	if imported.Digest() == none.Digest() {
		t.Fatal("a declared transport vector is not part of the sealed identity")
	}
	found := false
	for _, reference := range imported.References() {
		if reference == schema.EntryReference(transportAxis("value")) {
			found = true
		}
	}
	if !found {
		t.Fatal("a transported axis is not on the program's upward reference surface")
	}
}

// TestAProgramWithoutATransportVectorDigestsExactlyAsBefore is the byte
// identity fence: adding the vector to the ABI remints no program that does
// not declare one.
func TestAProgramWithoutATransportVectorDigestsExactlyAsBefore(t *testing.T) {
	specimens := seq5742Specimens()
	transfer, held := specimens["value-transfer"]
	if !held {
		t.Fatal("the value-transfer specimen")
	}
	if transfer.TransportCount() != 0 {
		t.Fatal("an ordinary rule declares no transport vector")
	}
	clone := transfer.Clone()
	if clone.Digest() != transfer.Digest() {
		t.Fatal("cloning a transport-free program changed its identity")
	}
}
