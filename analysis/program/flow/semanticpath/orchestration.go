package semanticpath

// This package is the only constructor for the semantic-path planes. The
// public certificate is a consumer of the result; it must not be able to
// accept caller-supplied rows. The derivation is split by owned plane below,
// while this file keeps the owner-quartet orchestration in one place.

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/outcome"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// derivedPlanes is intentionally private. certificate.go is in this package
// and may consume the planes, while no downstream owner can manufacture one.
type derivedPlanes struct {
	edges           [keyspace.FamilyCount][]edgeDescriptor
	rootDescriptors [keyspace.FamilyCount][]identity.ContentID
	body            []identity.ContentID
	roots           [keyspace.FamilyCount][]identity.ContentID
	terms           [keyspace.FamilyCount][]identity.ContentID
}

// derive builds every body-qualified structural plane in one owner-fenced
// operation. The source and authored views provide the canonical dense
// denominators; body, containment, and outcome results provide the exact
// sealed relations. No identity plane is accepted from the caller.
func derive(sourceView source.View, cellRoles source.CellRoles, authoredView authored.View, bodies *body.Result, bindings binding.Result, forest *containment.Result, outcomes *outcome.Result, sourceID, flowID, staticID, moduleID identity.ContentID) (derivedPlanes, error) {
	var out derivedPlanes
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() ||
		sourceView.Identity().ContentID() != sourceID || authoredView.ContentID() != flowID {
		return out, errors.New("semanticpath: owner identities are unavailable or disagree")
	}
	if bodies == nil || !body.Matches(bodies, sourceID, flowID) {
		return out, errors.New("semanticpath: Body result does not match Source/Flow")
	}
	if forest == nil || !containment.Matches(forest, sourceID, flowID, staticID, moduleID) {
		return out, errors.New("semanticpath: containment result does not match owner quartet")
	}
	if outcomes == nil || !outcome.Matches(outcomes, sourceID, flowID, staticID, moduleID) {
		return out, errors.New("semanticpath: Outcome result does not match owner quartet")
	}
	if bodies.BodyCount() != sourceView.Identity().FamilyCount(keyspace.FamilyBody) {
		return out, errors.New("semanticpath: Body denominator disagrees with Source")
	}
	if !cellRoles.Matches(sourceView) || cellRoles.CellCount() != authoredView.Storage().Cells().Count() || !binding.Matches(&bindings, sourceID, flowID) || bindings.CellCount() != cellRoles.CellCount() {
		return out, errors.New("semanticpath: Cell roles or Binding disagrees with exact owners")
	}

	edges, err := deriveEdges(sourceView, authoredView, forest)
	if err != nil {
		return out, err
	}
	if err := mergeContainmentRoles(sourceView, forest, &edges); err != nil {
		return out, err
	}
	rootDescriptors, err := deriveRootDescriptors(sourceView, authoredView, bodies)
	if err != nil {
		return out, err
	}
	resolver := structuralResolver{source: sourceView, forest: forest, edges: edges, descriptors: rootDescriptors, memo: make(map[keyspace.Term]identity.ContentID), visiting: make(map[keyspace.Term]bool)}
	bodyPaths, err := deriveBodyPaths(sourceView, authoredView, bodies, forest, edges, rootDescriptors, &resolver)
	if err != nil {
		return out, err
	}
	rootPaths, err := deriveRootPaths(sourceView, bodies, bodyPaths, rootDescriptors)
	if err != nil {
		return out, err
	}
	resolver.body = bodyPaths
	termPaths, err := deriveTermPaths(sourceView, cellRoles, authoredView, bindings, bodies, forest, outcomes, edges, bodyPaths, rootDescriptors, rootPaths, &resolver)
	if err != nil {
		return out, err
	}
	out.edges, out.rootDescriptors, out.body, out.roots, out.terms = edges, rootDescriptors, bodyPaths, rootPaths, termPaths
	return out, nil
}
