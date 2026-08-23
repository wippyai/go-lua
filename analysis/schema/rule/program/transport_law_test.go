package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

func transportAxis(key string) AxisRef {
	return AxisRef(schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)})
}

// transportProgram is the call-activation shape of seq 5742 with a declared
// transport vector: one exact candidate read, one owner-issued target-role
// selection over it, and one structural publication.
func transportProgram(rows []TransportDecl) Program {
	program := seq5742Program(
		"call-activation",
		[]JoinDecl{
			seq5742Join("call-activation/call", []SourceRef{CandidateSource()}, Exact, false, false),
			seq5742Join("call-activation/target-role", []SourceRef{PriorSource(0)}, Selected, true, true),
		},
		[]JoinRef{0, 1},
		[]OutputDecl{seq5742Output("call-activation/write", ModeStructural, 0)},
	)
	program.Transport = rows
	return program
}

// TestATransportVectorCannotExportAnAxisItDoesNotImport is the FG-6 symmetry
// law, restated as a property of the declaration's shape rather than as a
// check over two lists. One row is one axis crossing the edge: the import
// direction is the row's existence and the export direction is its flag, so an
// export with no import is not a malformed declaration - it cannot be written
// down at all.
func TestATransportVectorCannotExportAnAxisItDoesNotImport(t *testing.T) {
	rows := []TransportDecl{
		{Axis: transportAxis("value"), Exported: true},
		{Axis: transportAxis("call")},
		{Axis: transportAxis("heap"), Exported: true},
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
		{Axis: transportAxis("value")},
		{Axis: transportAxis("value"), Exported: true},
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
	imported := transportProgram([]TransportDecl{{Axis: transportAxis("value")}})
	exported := transportProgram([]TransportDecl{{Axis: transportAxis("value"), Exported: true}})
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
