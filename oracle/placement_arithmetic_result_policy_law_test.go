package oracle

import "testing"

// This file states what a Value binary-arithmetic occurrence is allowed to
// answer when Value's own narrowing reaches an exact pair the Program did not
// pre-enumerate.
//
// The Value atom universe is sealed from Program's exact-scalar closure, so an
// exact product is representable only where Program proved the occurrence's
// operand images finite and enumerated the image. Three placement fixtures sit
// exactly on the other side of that line. Two of them carry an unbounded
// counter recurrence, where the finite image does not exist at all; the third
// adds one to a table field Program treats as opaque while Value's record
// narrowing still knows the field's literal. Each of them reaches a successful
// exact pair whose product has no sealed atom.
//
// There are only three answers available to that pair, and two of them are
// unavailable by rule. Minting an atom for the product would extend the atom
// universe at solve time, which is a global atom table under another name and
// cannot terminate for a recurrence. Refusing the fold is what the analyzer
// did: the whole solve stops at execution/malformed on the arithmetic rule,
// which is the state these three fixtures pin. The remaining answer is the one
// the occurrence's own policy names - Program declared the image open, so the
// result is the already-sealed numeric relation and nothing narrower.
//
// The fixtures are named here rather than swept for, because each one is a
// different reason the image is open, and a sweep that lost one of the three
// would still be green.
var arithmeticResultPolicyLawFixtures = []string{
	// A recurrence whose counter grows without bound.
	"placement/bridge-main-event-loop",
	// A recurrence over a list under mutation.
	"placement/list-inbox-clean",
	// An opaque table-field read that Value narrows to a literal.
	"placement/sealpoint-post-send-mutation",
}

// TestOpenArithmeticOccurrenceCompletesItsSolve states the property at the
// only altitude where it is observable: the fixture solves. An occurrence
// whose exact image Program left open answers the sealed numeric relation, so
// nothing along the arithmetic rule refuses and the run reaches a complete
// analysis.
func TestOpenArithmeticOccurrenceCompletesItsSolve(t *testing.T) {
	for _, name := range arithmeticResultPolicyLawFixtures {
		name := name
		t.Run(name, func(t *testing.T) {
			run, class, err := corpusHarnessExecuteDetached(t, corpusHarnessFixture(t, name), corpusHarnessDiagnosticMode())
			if err != nil {
				t.Fatalf("fixture %s did not complete its solve (%s): %v", name, class, err)
			}
			if run == nil {
				t.Fatalf("fixture %s produced no run", name)
			}
		})
	}
}
