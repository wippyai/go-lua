package static

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// StaticTypes is the post-commit hot view over Static's complete authored type
// forest. It retains only the published Component pointer; construction Views
// deliberately do not carry that pointer and therefore expose an expired view.
type StaticTypes struct{ component *Component }

// StaticTypeRef is an owner-bound capability for one published Static type.
// The owner and Term remain private so callers can transport only the checked
// capability and recover its local Term. Term itself is deliberately a local
// (family, ordinal) encoding: it carries no Component provenance, so passing
// the result to another StaticTypes.Ref may bind the same encoding there.
type StaticTypeRef struct {
	component *Component
	term      keyspace.Term
}

// StaticTypes returns the post-commit Static type capability. A claimed
// construction View has no Component pointer and therefore returns a zero
// capability that cannot leak an enduring reference.
func (view View) StaticTypes() StaticTypes { return StaticTypes{component: view.component} }

// ContentID returns the authored Static identity through a lifecycle-bound
// construction View. A claimed View exposes the draft's identity; once its
// Finalizer commits or aborts (including an invalid terminal input), the
// same copied View returns an unavailable identity. A published Component
// View remains identity-bearing because it has no construction state.
func (view View) ContentID() identity.ContentID {
	component := view.componentOf()
	if component == nil {
		return identity.ContentID{}
	}
	return component.contentID
}

// Count returns the complete canonical Static type forest cardinality.
func (types StaticTypes) Count() int {
	component := types.component
	if component == nil || !component.contentID.Available() {
		return 0
	}
	return component.StaticTypeTermCount()
}

// At returns one owner-bound capability in the Component's existing canonical
// family order. No second Term list or identity is materialized.
func (types StaticTypes) At(index int) (StaticTypeRef, bool) {
	component := types.component
	if component == nil || !component.contentID.Available() {
		return StaticTypeRef{}, false
	}
	term, ok := component.StaticTypeTermAt(index)
	if !ok || !component.StaticTypeTerm(term) {
		return StaticTypeRef{}, false
	}
	return StaticTypeRef{component: component, term: term}, true
}

// Ref validates and binds one raw Component-local static type Term. Terms are
// local (family, ordinal) encodings rather than owner-provenanced identities:
// the same encoding from another Component may be rebound here. Nil,
// wrong-family, malformed, and out-of-range Terms fail closed.
func (types StaticTypes) Ref(term keyspace.Term) (StaticTypeRef, bool) {
	component := types.component
	if component == nil || !component.StaticTypeTerm(term) {
		return StaticTypeRef{}, false
	}
	return StaticTypeRef{component: component, term: term}, true
}

// Term recovers the checked local Term and discards the owner binding. A zero
// ref or a ref whose Component is no longer available returns the zero Term;
// callers that pass the result to another StaticTypes view are requesting a
// fresh local binding there.
func (ref StaticTypeRef) Term() keyspace.Term {
	if ref.component == nil || !ref.component.StaticTypeTerm(ref.term) {
		return 0
	}
	return ref.term
}
