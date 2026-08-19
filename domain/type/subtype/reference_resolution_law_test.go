package subtype

import (
	"testing"

	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/transform"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
)

// sameBodiedRecursive builds mu X . {next: X?} under the given presentation
// name. Two calls differing only in the name produce the same body geometry
// under two distinct binders.
func sameBodiedRecursive(name string) *typ.Recursive {
	return typ.NewRecursive(name, func(self typ.Type) typ.Type {
		return typetable.NewRecord().Field("next", typeexpr.Optional(self)).Build()
	})
}

// TestReferenceVerdictIsIndependentOfBinderPresentationName states that a
// binder's Name is presentation and carries no proof weight: two recursive
// types with identical bodies under different names must answer a given
// reference identically, in both directions and in either construction order.
//
// Interning collapses same-bodied binders to one node holding whichever name
// the first allocation carried. A verdict that reads that field is therefore
// a verdict on allocation order.
func TestReferenceVerdictIsIndependentOfBinderPresentationName(t *testing.T) {
	ref := typ.NewRef("", "Node")

	for _, order := range []struct {
		name  string
		first string
		other string
	}{
		{name: "named binder first", first: "Node", other: "Other"},
		{name: "unnamed binder first", first: "Other", other: "Node"},
	} {
		t.Run(order.name, func(t *testing.T) {
			first := sameBodiedRecursive(order.first)
			other := sameBodiedRecursive(order.other)

			if got, want := IsSubtype(ref, first), IsSubtype(ref, other); got != want {
				t.Fatalf("ref <: same-bodied binders disagree: %q=%v %q=%v", order.first, got, order.other, want)
			}
			if got, want := IsSubtype(first, ref), IsSubtype(other, ref); got != want {
				t.Fatalf("same-bodied binders <: ref disagree: %q=%v %q=%v", order.first, got, order.other, want)
			}
		})
	}
}

// TestReferenceVerdictIsIndependentOfAliasPresentationName is the Alias
// restatement of the same law: an alias's Name is presentation, so two
// aliases over the same target must answer a reference identically.
func TestReferenceVerdictIsIndependentOfAliasPresentationName(t *testing.T) {
	ref := typ.NewRef("", "Count")
	named := typ.NewAlias("Count", typ.Integer)
	other := typ.NewAlias("Other", typ.Integer)

	if got, want := IsSubtype(ref, named), IsSubtype(ref, other); got != want {
		t.Fatalf("ref <: same-target aliases disagree: named=%v other=%v", got, want)
	}
	if got, want := IsSubtype(named, ref), IsSubtype(other, ref); got != want {
		t.Fatalf("same-target aliases <: ref disagree: named=%v other=%v", got, want)
	}
}

// TestResolvedReferenceProvesExactlyItsNode states that a reference proves
// what the node its declaration resolves to proves, and nothing beyond it.
// Resolution replaces the reference node by identity, the way subst.SelfRef
// and the manifest scoper resolve; an unresolved reference is not a licence
// to answer as though resolution had happened.
func TestResolvedReferenceProvesExactlyItsNode(t *testing.T) {
	node := sameBodiedRecursive("Node")
	ref := typ.NewRef("", "Node")
	resolved := transform.Rewrite(ref, func(candidate typ.Type) (typ.Type, bool) {
		if candidate == ref {
			return node, true
		}
		return nil, false
	})

	// A distinct declaration that merely spells the same name, plus the
	// structural shapes a caller might ask about.
	decoy := sameBodiedRecursive("Node")
	candidates := []typ.Type{
		node,
		decoy,
		typ.NewAlias("Node", typ.String),
		typetable.NewRecord().Field("next", typeexpr.Optional(node)).Build(),
		typ.String,
	}

	for _, candidate := range candidates {
		if IsSubtype(resolved, candidate) != IsSubtype(node, candidate) {
			t.Fatalf("resolved ref <: %s diverges from its node", candidate)
		}
		if IsSubtype(candidate, resolved) != IsSubtype(candidate, node) {
			t.Fatalf("%s <: resolved ref diverges from its node", candidate)
		}
		// The unresolved reference may never prove a relation its own
		// declaration cannot prove.
		if IsSubtype(ref, candidate) && !IsSubtype(node, candidate) {
			t.Fatalf("unresolved ref proves ref <: %s where its declaration does not", candidate)
		}
		if IsSubtype(candidate, ref) && !IsSubtype(candidate, node) {
			t.Fatalf("unresolved ref proves %s <: ref where its declaration does not", candidate)
		}
	}
}
