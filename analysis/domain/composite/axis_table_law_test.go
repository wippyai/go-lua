package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// TestAxisTableSeals is the axis surface's own totality law on the production
// inventory: the declaration root admits it alongside the rule surface, and
// the sealed view is the composed table.
func TestAxisTableSeals(t *testing.T) {
	table, failure := Table()
	if failure.Available() {
		t.Fatalf("declaration table rejected: contributor=%d entry=%x law=%d disposition=%s", failure.Contributor, failure.Entry, failure.Law, failure.Disposition)
	}
	view, viewOK := table.Surface(schema.SurfaceKindAxis)
	if !viewOK || view.Count() != AxisCount() {
		t.Fatalf("axis surface view = %d entries, table = %d", view.Count(), AxisCount())
	}
	if AxisCount() != axisPrincipalLimit-1 {
		t.Fatalf("axis table declares %d axes, the artifact catalog has %d factor lanes", AxisCount(), axisPrincipalLimit-1)
	}
}

// TestAxisTableDeclaresEveryWriterPrincipalOnce is the composition's coverage
// law. Every factor lane a rule can write is one declared axis, so a rule's
// principal always resolves to an axis and no lane has two owners.
func TestAxisTableDeclaresEveryWriterPrincipalOnce(t *testing.T) {
	seen := make(map[programartifact.RuleOutputKind]int, AxisCount())
	for position := 0; position < AxisCount(); position++ {
		principal, ok := AxisPrincipalAt(position)
		if !ok || principal == programartifact.RuleOutputInvalid {
			t.Fatalf("axis position %d has no writer principal", position)
		}
		seen[principal]++
		if seen[principal] != 1 {
			t.Fatalf("writer principal %d declared %d times", principal, seen[principal])
		}
	}
	for position := 0; position < RuleCount(); position++ {
		role, roleOK := RuleRoleAt(position)
		if !roleOK {
			t.Fatalf("rule position %d has no role", position)
		}
		principal := programartifact.RuleOutputKindFor(role)
		if seen[principal] != 1 {
			t.Fatalf("rule role %d writes lane %d, which no axis declares", role, principal)
		}
	}
}

// TestAxisTableDrivesEveryDerivedView is the drift law. Every projection the
// analyzer consumes - identity, semantic key, storage, cardinality, lifetime,
// and diagnostic classification - is computed from the sealed entry, so an
// axis that reaches the table is wired everywhere and a principal that is not
// in the table is classified nowhere.
func TestAxisTableDrivesEveryDerivedView(t *testing.T) {
	table, failure := Table()
	if failure.Available() {
		t.Fatalf("declaration table rejected: law=%d", failure.Law)
	}
	view, viewOK := table.Surface(schema.SurfaceKindAxis)
	if !viewOK {
		t.Fatal("sealed table published no axis surface")
	}
	bundle, bundleOK := vocabulary.New()
	if !bundleOK {
		t.Fatal("vocabulary")
	}
	semantics := make(map[identity.SemanticKey]bool, view.Count())
	for position := 0; position < view.Count(); position++ {
		entry, entryOK := view.At(position)
		principal, principalOK := AxisPrincipalAt(position)
		if !entryOK || !principalOK {
			t.Fatalf("axis position %d is not derivable from the sealed table", position)
		}
		id, idOK := AxisEntryID(principal)
		if !idOK || id != schema.NewEntryID(schema.SurfaceKindAxis, entry.Key()) {
			t.Fatalf("axis %q publishes an identity the sealed entry does not derive", entry.Key())
		}
		if _, known := view.ByID(id); !known {
			t.Fatalf("axis %q is not resolvable by its own identity", entry.Key())
		}
		if DiagnosticAxisForPrincipal(principal).String() != string(entry.Key()) {
			t.Fatalf("axis %q classifies as %q", entry.Key(), DiagnosticAxisForPrincipal(principal))
		}
		semantic, semanticOK := AxisSemantic(principal)
		if !semanticOK || !semantic.Available() {
			t.Fatalf("axis %q publishes no canonical identity", entry.Key())
		}
		if semantics[semantic] {
			t.Fatalf("axis %q shares a canonical identity", entry.Key())
		}
		semantics[semantic] = true
		// Every declared axis is a Link-bound factor over a dense ordinal key
		// space. A later inventory that is neither reads these fields rather
		// than assuming this shape.
		storage, storageOK := AxisStorage(principal)
		cardinality, cardinalityOK := AxisCardinality(principal)
		lifetime, lifetimeOK := AxisLifetime(principal)
		if !storageOK || storage != axis.StorageFactor {
			t.Fatalf("axis %q declares storage %d", entry.Key(), storage)
		}
		if !cardinalityOK || !cardinality.Available() {
			t.Fatalf("axis %q declares no key cardinality", entry.Key())
		}
		if !lifetimeOK || lifetime != axis.LifetimeLink {
			t.Fatalf("axis %q declares lifetime %d", entry.Key(), lifetime)
		}
	}
	// The canonical identity of an axis is the vocabulary's factor identity;
	// the table is not free to invent one.
	for principal, expected := range map[programartifact.RuleOutputKind]identity.SemanticKey{
		programartifact.RuleOutputValue:  bundle.ValueFactor,
		programartifact.RuleOutputPack:   bundle.PackFactor,
		programartifact.RuleOutputHeap:   bundle.HeapFactor,
		programartifact.RuleOutputCall:   bundle.CallFactor,
		programartifact.RuleOutputEffect: bundle.EffectFactor,
	} {
		semantic, ok := AxisSemantic(principal)
		if !ok || semantic != expected {
			t.Fatalf("axis for lane %d publishes %x, the vocabulary declares %x", principal, semantic.Digest(), expected.Digest())
		}
	}
	if _, ok := AxisSemantic(programartifact.RuleOutputInvalid); ok {
		t.Fatal("an undeclared lane resolved to an axis")
	}
	if DiagnosticAxisForPrincipal(programartifact.RuleOutputInvalid) != DiagnosticAxisUnknown {
		t.Fatal("an undeclared lane classified as a known axis")
	}
}

// TestAxisSurfaceIsSealedBeforeTheRuleSurface states the phase ordering the
// binding transaction depends on, over what the composition actually produces:
// the sealed table publishes both surfaces, and the cold pass refuses to
// declare or bind an axis outside the order that produces the principals a rule
// declaration receives.
func TestAxisSurfaceIsSealedBeforeTheRuleSurface(t *testing.T) {
	table, failure := Table()
	if failure.Available() {
		t.Fatalf("declaration table rejected: law=%d", failure.Law)
	}
	axes, axesOK := table.Surface(schema.SurfaceKindAxis)
	rules, rulesOK := table.Surface(schema.SurfaceKindRule)
	if !axesOK || !rulesOK || !axes.Available() || !rules.Available() {
		t.Fatal("sealed table does not publish both surfaces")
	}
	// The cold pass is the same ordering: a rule's declaration receives the
	// principals the axis pass produced, so an unbuilt axis table cannot yield
	// a rule fragment.
	fragments, _, declared := declareAxes(nil, vocabulary.Bundle{})
	if declared || fragments.available() {
		t.Fatal("axis pass declared without a schema builder")
	}
	if bound, _, ok := bindAxes(nil, fragments, LinkInputs{}); ok || bound.available() {
		t.Fatal("axis pass bound without a declared table")
	}
}
