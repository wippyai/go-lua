// Package staticcheck is Flow's final Static commit-input check.
//
// It joins the already sealed Source, authored Flow, Static, Body, Binding,
// containment, and direct-binding proofs.  The package intentionally returns
// only static.CommitInput: its lexical context and all validation scratch die
// with Validate, and no second Static authority is retained.
package staticcheck

import (
	"errors"

	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/accessgeometry"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

// Validate is the frozen commit-input-producing leaf. All input identities and
// Match fences are checked before any derived relation is used.  Scratch
// context and proof observations are discarded with the return value.
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
	if !accessgeometry.Matches(access, sourceID, flowID, staticID, moduleID) {
		return empty, errors.New("program/flow/staticcheck: access geometry provenance mismatch")
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
	if err := validatePublications(sourceView, flowView, staticView, bindings, access, context); err != nil {
		return empty, err
	}
	input, err := canonicalInput(staticView)
	if err != nil {
		return empty, err
	}
	return input, nil
}

// canonicalInput materializes the dense static commit input from the sealed
// view. The returned slices are fresh, so callers can hand the input to the
// finalizer without exposing Static's internal columns.
func canonicalInput(view staticquery.View) (static.CommitInput, error) {
	typeOfs := view.Operators().TypeOfs()
	annotations := view.Operands().Annotations()
	publications := view.Publications()
	input := static.CommitInput{
		TypeOf:       make([]keyspace.Term, typeOfs.Count()),
		Annotations:  make([]keyspace.Term, annotations.Count()),
		Publications: make([]keyspace.Term, publications.Count()),
	}
	for index := range input.TypeOf {
		term, ok := typeOfs.At(index)
		want := keyspace.MakeTerm(keyspace.FamilyTypeOf, uint32(index+1))
		if !ok || term != want {
			return static.CommitInput{}, errors.New("program/flow/staticcheck: TypeOf input is not canonical")
		}
		input.TypeOf[index] = term
	}
	for index := range input.Annotations {
		term, ok := annotations.At(index)
		want := keyspace.MakeTerm(keyspace.FamilyAnnotation, uint32(index+1))
		if !ok || term != want {
			return static.CommitInput{}, errors.New("program/flow/staticcheck: Annotation input is not canonical")
		}
		input.Annotations[index] = term
	}
	for index := range input.Publications {
		term, ok := publications.At(index)
		want := keyspace.MakeTerm(keyspace.FamilyTypePublication, uint32(index+1))
		if !ok || term != want {
			return static.CommitInput{}, errors.New("program/flow/staticcheck: Publication input is not canonical")
		}
		input.Publications[index] = term
	}
	return input, nil
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
