package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
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
}

// TestAxisTableDeclaresEveryWriterPrincipalOnce is the composition's coverage
// law. An axis is a writer principal, so one principal is one declared axis:
// the inventory's keys are distinct, and every slot the table hands out
// resolves back to the axis declared at it.
func TestAxisTableDeclaresEveryWriterPrincipalOnce(t *testing.T) {
	seen := make(map[schema.Key]int, AxisCount())
	for position := 0; position < AxisCount(); position++ {
		key, ok := AxisKeyAt(position)
		if !ok || key == "" {
			t.Fatalf("axis position %d declares no key", position)
		}
		seen[key]++
		if seen[key] != 1 {
			t.Fatalf("writer principal %q declared %d times", key, seen[key])
		}
		slot, slotOK := axisSlotForKey(key)
		if !slotOK || slot != position+1 {
			t.Fatalf("axis %q resolves to slot %d at position %d", key, slot, position)
		}
	}
	if len(seen) == 0 {
		t.Fatal("the axis table declares no writer principal")
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
	roles, rolesOK := SemanticRoles()
	if !rolesOK {
		t.Fatal("semantic role vocabulary")
	}
	semantics := make(map[identity.SemanticKey]bool, view.Count())
	for position := 0; position < view.Count(); position++ {
		entry, entryOK := view.At(position)
		key, keyOK := AxisKeyAt(position)
		if !entryOK || !keyOK || key != entry.Key() {
			t.Fatalf("axis position %d is not derivable from the sealed table", position)
		}
		id, idOK := AxisEntryID(key)
		if !idOK || id != schema.NewEntryID(schema.SurfaceKindAxis, entry.Key()) {
			t.Fatalf("axis %q publishes an identity the sealed entry does not derive", entry.Key())
		}
		if _, known := view.ByID(id); !known {
			t.Fatalf("axis %q is not resolvable by its own identity", entry.Key())
		}
		if DiagnosticAxisForKey(key).String() != string(entry.Key()) {
			t.Fatalf("axis %q classifies as %q", entry.Key(), DiagnosticAxisForKey(key))
		}
		semantic, semanticOK := AxisSemantic(key)
		if !semanticOK || !semantic.Available() {
			t.Fatalf("axis %q publishes no canonical identity", entry.Key())
		}
		if semantics[semantic] {
			t.Fatalf("axis %q shares a canonical identity", entry.Key())
		}
		semantics[semantic] = true
		// Every declared axis states where its facts live, how its key space is
		// shaped, and how long it is valid for, and a consumer reads those
		// fields rather than assuming one shape for the inventory. What the
		// storage settles is the rest: a factor axis is bound with one Link
		// binding and dies with it, and an engine-published axis is not bound at
		// all.
		storage, storageOK := AxisStorage(key)
		cardinality, cardinalityOK := AxisCardinality(key)
		lifetime, lifetimeOK := AxisLifetime(key)
		if !storageOK || !storage.Available() {
			t.Fatalf("axis %q declares storage %d", entry.Key(), storage)
		}
		if !cardinalityOK || !cardinality.Available() {
			t.Fatalf("axis %q declares no key cardinality", entry.Key())
		}
		if !lifetimeOK || !lifetime.Available() {
			t.Fatalf("axis %q declares lifetime %d", entry.Key(), lifetime)
		}
		if storage == axis.StorageFactor && lifetime != axis.LifetimeLink {
			t.Fatalf("factor axis %q declares lifetime %d, not the Link binding it is bound with", entry.Key(), lifetime)
		}
	}
	// The canonical identity of an axis is the identity the vocabulary declares
	// for its coordinate space; the table is not free to invent one.
	for key, role := range map[schema.Key]schema.Key{
		axisKeyValue:                 "semantic/factor/value",
		axisKeyPack:                  "semantic/factor/pack",
		axisKeyHeap:                  "semantic/factor/heap",
		axisKeyCall:                  "semantic/factor/call",
		axisKeyEffect:                "semantic/factor/effect",
		axisKeyExecutionReachability: "semantic/axis/execution-reachability",
		axisKeyChannelSelectCase:     "semantic/axis/channel-select-case",
	} {
		expected, expectedOK := roles.Key(role)
		semantic, ok := AxisSemantic(key)
		if !expectedOK || !ok || semantic != expected {
			t.Fatalf("axis %q publishes %x, the vocabulary declares role %q", key, semantic.Digest(), role)
		}
	}
	if _, ok := AxisSemantic("no-such-axis"); ok {
		t.Fatal("an undeclared key resolved to an axis")
	}
	if DiagnosticAxisForKey("no-such-axis") != DiagnosticAxisUnknown {
		t.Fatal("an undeclared key classified as a known axis")
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
	fragments, _, declared := declareAxes(nil, vocabulary.Roles{})
	if declared || fragments.available(registry.axes) {
		t.Fatal("axis pass declared without a schema builder")
	}
	if bound, _, ok := bindAxes(nil, fragments, LinkInputs{}); ok || bound.available(registry.axes) {
		t.Fatal("axis pass bound without a declared table")
	}
}
