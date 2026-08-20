// Package staticcheck is Flow's final Static semantic check.
//
// It joins the already sealed Source, authored Flow, Static, Body, Binding,
// containment, and direct-binding proofs. Its lexical context and all
// validation scratch die with Validate; no second Static authority is
// retained.
package staticcheck

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/accessgeometry"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
)

// Validate is the final semantic check. All input identities and Match fences
// are checked before any derived relation is used. Scratch context and proof
// observations are discarded when it returns.
func Validate(
	sourceView source.View,
	flowView authored.View,
	staticView staticquery.View,
	bodies *body.Result,
	bindings binding.Result,
	forest *containment.Result,
	scopeProof *containment.StaticScopeProof,
	access *accessgeometry.Result,
	moduleID identity.ContentID,
	entry keyspace.Term,
) error {
	sourceID := sourceView.Identity().ContentID()
	flowID := flowView.Cold().ContentID()
	staticID := staticView.ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() {
		return errors.New("program/flow/staticcheck: owner view is unavailable")
	}
	if !body.Matches(bodies, sourceID, flowID) {
		return errors.New("program/flow/staticcheck: Body provenance mismatch")
	}
	if !binding.Matches(&bindings, sourceID, flowID) {
		return errors.New("program/flow/staticcheck: Binding provenance mismatch")
	}
	if !containment.Matches(forest, sourceID, flowID, staticID, moduleID) {
		return errors.New("program/flow/staticcheck: containment provenance mismatch")
	}
	if !scopeProof.Matches(sourceID, flowID, staticID, moduleID) {
		return errors.New("program/flow/staticcheck: Static scope provenance mismatch")
	}
	if !accessgeometry.Matches(access, sourceID, flowID, staticID, moduleID) {
		return errors.New("program/flow/staticcheck: access geometry provenance mismatch")
	}
	if err := validateDenominators(sourceView, staticView, bodies, forest, entry); err != nil {
		return err
	}
	context, err := buildContext(sourceView, flowView, staticView, bodies, bindings, entry)
	if err != nil {
		return err
	}
	if err := validateObservations(sourceView, flowView, staticView, bodies, forest, scopeProof, bindings, context); err != nil {
		return err
	}
	if err := validatePublications(sourceView, flowView, staticView, bindings, access, context); err != nil {
		return err
	}
	return nil
}

func validateDenominators(
	sourceView source.View,
	staticView staticquery.View,
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
		return errors.New("program/flow/staticcheck: Static input denominator mismatch")
	}
	return nil
}
