package staticcheck

import (
	"errors"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/binding"
	"github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/flow/internal/containment"
	"github.com/wippyai/go-lua/program/flow/internal/directbinding"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
)

// Validate is the frozen receipt-producing leaf.  All input identities and
// Match fences are checked before any derived relation is used.  Scratch
// context and proof observations are discarded with the return value.
func Validate(
	sourceView source.View,
	flowView authored.View,
	staticView static.View,
	bodies *body.Result,
	bindings binding.Result,
	forest *containment.Result,
	scopeProof *containment.StaticScopeProof,
	direct *directbinding.Result,
	moduleID keyspace.ContentID,
	entry keyspace.Term,
) (static.CommitInput, error) {
	empty := static.CommitInput{}
	sourceID := sourceView.Identity().ContentID()
	flowID := flowView.Cold().ContentID()
	staticID := staticView.ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() {
		return empty, errors.New("program/flow/staticcheck: owner view is unavailable")
	}
	if !body.Matches(bodies, sourceID, flowID) {
		return empty, errors.New("program/flow/staticcheck: Body provenance mismatch")
	}
	if !binding.Matches(&bindings, sourceID, flowID) {
		return empty, errors.New("program/flow/staticcheck: Binding provenance mismatch")
	}
	if !containment.Matches(forest, sourceID, flowID, staticID, moduleID) {
		return empty, errors.New("program/flow/staticcheck: containment provenance mismatch")
	}
	if !scopeProof.Matches(sourceID, flowID, staticID, moduleID) {
		return empty, errors.New("program/flow/staticcheck: Static scope provenance mismatch")
	}
	if !directbinding.Matches(direct, sourceID, flowID, staticID, moduleID) {
		return empty, errors.New("program/flow/staticcheck: direct-binding provenance mismatch")
	}
	if err := validateDenominators(sourceView, staticView, bodies, forest, entry); err != nil {
		return empty, err
	}
	context, err := buildContext(sourceView, flowView, staticView, bodies, bindings, entry)
	if err != nil {
		return empty, err
	}
	if err := validateObservations(sourceView, flowView, staticView, bodies, forest, scopeProof, bindings, context); err != nil {
		return empty, err
	}
	if err := validatePublications(sourceView, flowView, staticView, bindings, direct, context); err != nil {
		return empty, err
	}
	receipt, err := canonicalReceipt(staticView)
	if err != nil {
		return empty, err
	}
	return receipt, nil
}

func validateDenominators(
	sourceView source.View,
	staticView static.View,
	bodies *body.Result,
	forest *containment.Result,
	entry keyspace.Term,
) error {
	identity := sourceView.Identity()
	if identity.Name() == "" || identity.TermCount() == 0 {
		return errors.New("program/flow/staticcheck: Source identity is unavailable")
	}
	if keyspace.TermFamily(entry) != keyspace.FamilyBody || keyspace.TermOrdinal(entry) == 0 ||
		int(keyspace.TermOrdinal(entry)) > identity.FamilyCount(keyspace.FamilyBody) {
		return errors.New("program/flow/staticcheck: invalid Entry Body")
	}
	if parent, hasParent := bodies.Parent(entry); hasParent || parent != 0 {
		return errors.New("program/flow/staticcheck: Entry Body has a parent")
	}
	if !forest.Contains(entry, entry) {
		return errors.New("program/flow/staticcheck: Entry Body is absent from containment")
	}
	if staticView.Operators().TypeOfs().Count() != identity.FamilyCount(keyspace.FamilyTypeOf) ||
		staticView.Operands().Annotations().Count() != identity.FamilyCount(keyspace.FamilyAnnotation) ||
		staticView.Publications().Count() != identity.FamilyCount(keyspace.FamilyTypePublication) {
		return errors.New("program/flow/staticcheck: Static receipt denominator mismatch")
	}
	return nil
}
