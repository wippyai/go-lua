package assembly

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
)

func TestAssemblyControlFaultRequiresAValidOwner(t *testing.T) {
	c := newAssemblyCollector()
	if got := c.ControlFault(assemblyTestSpan(), 0, programsource.ControlFaultUndefinedGoto, 0, 0); got != 0 || c.err == nil {
		t.Fatalf("invalid control fault = %d with error %v", got, c.err)
	}
}

func TestAssemblySourceKeysKeepOwnerAndSpelling(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	name := c.Name(assemblyTestSpan(), body, "field")
	list := c.List(assemblyTestSpan(), body, 1)
	if keyspace.TermFamily(name) != keyspace.FamilyKey || keyspace.TermFamily(list) != keyspace.FamilyKey || name == list {
		t.Fatalf("source key terms = %d/%d, want distinct Key terms", name, list)
	}
}

func TestAssemblyLiteralTermsRetainDistinctOccurrences(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	first := c.String(assemblyTestSpan(), body, "same")
	second := c.String(assemblyTestSpan(), body, "same")
	if first == 0 || second == 0 || first == second {
		t.Fatalf("equal literal occurrences were not distinct: %d/%d", first, second)
	}
	value, ok := c.exactLiteral(first)
	if !ok || value.Kind != keyspace.LiteralString || value.String != "same" {
		t.Fatalf("exactLiteral = %#v/%t, want authored string", value, ok)
	}
}

func TestAssemblyBodyOrderAndEntryAreSingleAssignment(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	if body == 0 || !c.SetEntry(body) || c.Entry() != body {
		t.Fatalf("Body/Entry setup failed: body=%d entry=%d", body, c.Entry())
	}
	if !c.SetBody(body) {
		t.Fatal("SetBody rejected an empty authored Body sequence")
	}
	if c.SetBody(body) {
		t.Fatal("SetBody accepted a second fill")
	}
}

func TestAssemblySourceMintRejectsReservedFamiliesAndInvalidExactValues(t *testing.T) {
	c := newAssemblyCollector()
	if got := c.mint(keyspace.FamilyOutcome, assemblyTestSpan()); got != 0 || c.err == nil {
		t.Fatal("mint accepted the derived Outcome family")
	}
	if validRawExactCandidate(keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(math.NaN())}) {
		t.Fatal("NaN was accepted as an exact-key candidate")
	}
}
