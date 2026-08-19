package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestLifecycleViewExpiresEveryTypedProjectionAfterCommit(t *testing.T) {
	draft := primitiveDraft(t)
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	construction := finalizer.View()
	if !construction.Available() || construction.Types().Primitives().Count() != 1 {
		t.Fatal("claimed construction View did not expose its authored component")
	}
	if construction.References().Count() != 0 || construction.Declarations().Aliases().Count() != 0 ||
		construction.Publications().Count() != 0 || construction.Operands().Claims().Count() != 0 ||
		construction.Operators().TypeOfs().Count() != 0 || construction.Signatures().TypeFunctions().Count() != 0 ||
		construction.Contracts().Functions().Count() != 0 {
		t.Fatal("construction View exposed unexpected rows")
	}
	component, err := finalizer.Commit(CommitInput{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if construction.Available() || construction.Types().Primitives().Count() != 0 ||
		construction.References().Count() != 0 || construction.Declarations().Aliases().Count() != 0 ||
		construction.Publications().Count() != 0 || construction.Operands().Claims().Count() != 0 ||
		construction.Operators().TypeOfs().Count() != 0 || construction.Signatures().TypeFunctions().Count() != 0 ||
		construction.Contracts().Functions().Count() != 0 {
		t.Fatal("expired construction View retained a typed projection")
	}
	if !component.View().Available() || component.View().Types().Primitives().Count() != 1 {
		t.Fatal("published Component View lost its typed projection")
	}
}

// TestPoolBackedQueriesFailClosedOnAnUnavailableView proves every pool-backed
// cursor refuses a view that resolves to no component, rather than reaching
// into one that is not there.
func TestPoolBackedQueriesFailClosedOnAnUnavailableView(t *testing.T) {
	view := View{}
	term := func(family keyspace.Family) keyspace.Term { return keyspace.MakeTerm(family, 1) }
	for _, probe := range []struct {
		name string
		call func() bool
	}{
		{"reference source", func() bool { _, ok := view.References().SourceAt(term(keyspace.FamilyTypeRef), 0); return ok }},
		{"reference canonical", func() bool { _, ok := view.References().CanonicalAt(term(keyspace.FamilyTypeRef), 0); return ok }},
		{"alias param", func() bool {
			_, ok := view.Declarations().Aliases().ParamAt(term(keyspace.FamilyTypeAlias), 0)
			return ok
		}},
		{"interface extend", func() bool {
			_, ok := view.Declarations().Interfaces().ExtendAt(term(keyspace.FamilyTypeInterface), 0)
			return ok
		}},
		{"interface member", func() bool {
			_, ok := view.Declarations().Interfaces().MemberAt(term(keyspace.FamilyTypeInterface), 0)
			return ok
		}},
		{"union member", func() bool { _, ok := view.Types().Unions().MemberAt(term(keyspace.FamilyTypeUnion), 0); return ok }},
		{"intersection member", func() bool {
			_, ok := view.Types().Intersections().MemberAt(term(keyspace.FamilyTypeIntersection), 0)
			return ok
		}},
		{"generic arg", func() bool { _, ok := view.Types().Generics().ArgAt(term(keyspace.FamilyTypeGeneric), 0); return ok }},
		{"record field", func() bool { _, ok := view.Types().Records().FieldAt(term(keyspace.FamilyTypeRecord), 0); return ok }},
		{"signature type param", func() bool {
			_, ok := view.Signatures().TypeFunctions().TypeParamAt(term(keyspace.FamilyTypeFunction), 0)
			return ok
		}},
		{"signature parameter", func() bool {
			_, ok := view.Signatures().TypeFunctions().ParameterAt(term(keyspace.FamilyTypeFunction), 0)
			return ok
		}},
		{"signature return", func() bool {
			_, ok := view.Signatures().TypeFunctions().ReturnAt(term(keyspace.FamilyTypeFunction), 0)
			return ok
		}},
		{"contract type param", func() bool {
			_, ok := view.Contracts().Functions().TypeParamAt(term(keyspace.FamilyFunction), 0)
			return ok
		}},
		{"contract return", func() bool {
			_, ok := view.Contracts().Functions().ReturnAt(term(keyspace.FamilyFunction), 0)
			return ok
		}},
		{"call type argument", func() bool {
			_, ok := view.Contracts().Calls().TypeArgumentAt(term(keyspace.FamilyCall), 0)
			return ok
		}},
		{"annotation term", func() bool {
			_, ok := view.Operands().Annotations().ForAt(term(keyspace.FamilyTypePrimitive), 0)
			return ok
		}},
	} {
		t.Run(probe.name, func(t *testing.T) {
			if probe.call() {
				t.Fatal("pool-backed query accepted an unavailable view")
			}
		})
	}
}

func TestStaticQueryRootProjectsOwnedVerticalsAndFailsClosed(t *testing.T) {
	component := staticContentComponent(t, publicationFixture(t))
	view := component.View()
	if !view.Available() || view.Types().Primitives().Count() != 1 ||
		view.References().Count() != 2 || view.Declarations().Aliases().Count() != 1 ||
		view.Publications().Count() != 1 {
		t.Fatal("View did not project the authored top-level verticals")
	}
	var nilComponent *Component
	empty := nilComponent.View()
	if empty.Available() || empty.Types().Primitives().Count() != 0 ||
		empty.References().Count() != 0 || empty.Declarations().Aliases().Count() != 0 ||
		empty.Publications().Count() != 0 {
		t.Fatal("nil Component View did not fail closed")
	}
}
