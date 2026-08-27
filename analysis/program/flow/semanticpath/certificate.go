// Package semanticpath is Flow's sole structural semantic-path authority.
//
// It deliberately sits below SourceControl and Causal. A certificate is
// sealed once from Flow's exact Source/containment publication and exposes
// narrow immutable views to its consumers. Consumers never receive a mutable
// ContentID plane, so they cannot splice a sibling's paths into an otherwise
// authentic owner tuple.
package semanticpath

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/outcome"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
)

// Certificate owns the body-qualified structural path planes for exactly one
// committed Source/Flow/Static/Module quartet. Its rows are private and are
// exposed only through narrow immutable consumer views.
type Certificate struct {
	state *certificateState
}
type certificateState struct {
	sourceID identity.ContentID
	flowID   identity.ContentID
	staticID identity.ContentID
	moduleID identity.ContentID
	roots    [keyspace.FamilyCount][]identity.ContentID
	terms    [keyspace.FamilyCount][]identity.ContentID
	atoms    [keyspace.FamilyCount][]region.Atom
}

// Seal's failure classes remain intentionally narrow: assembly reports the
// exact failed structural invariant without exposing a way to construct a
// certificate.  The wrapped detail identifies the offending row or family.
var (
	ErrDeriveFailure           = errors.New("semanticpath: derive failed")
	ErrOwnerMismatch           = errors.New("semanticpath: owner identities are unavailable or disagree")
	ErrBodyCardinalityMismatch = errors.New("semanticpath: Body cardinality mismatch")
	ErrInvalidBodyRow          = errors.New("semanticpath: invalid Body row")
	ErrFamilyPlaneShape        = errors.New("semanticpath: family plane shape or sentinel mismatch")
	ErrUncoveredOrdinal        = errors.New("semanticpath: uncovered real ordinal")
)

func (c *Certificate) Available() bool {
	return c != nil && c.state != nil && c.matches(c.state.sourceID, c.state.flowID, c.state.staticID, c.state.moduleID)
}

// Matches is the exact four-owner fence for every direct certificate
// projection. Equal ContentIDs alone are insufficient unless all four owner
// coordinates name this sealed certificate.
func (c *Certificate) Matches(sourceID, flowID, staticID, moduleID identity.ContentID) bool {
	return c.matches(sourceID, flowID, staticID, moduleID)
}

// BodyPathAt and TermPathAt are Flow's narrow projections of
// the sole sealed certificate. They expose one already-issued identity, never
// an extendable plane or Term mapper.
func (c *Certificate) BodyPathAt(sourceID, flowID, staticID, moduleID identity.ContentID, ordinal uint32) (identity.ContentID, bool) {
	paths := c.state.terms[keyspace.FamilyBody]
	if !c.Matches(sourceID, flowID, staticID, moduleID) || ordinal == 0 || uint64(ordinal) >= uint64(len(paths)) {
		return identity.ContentID{}, false
	}
	id := paths[ordinal]
	return id, id.Available()
}

func (c *Certificate) TermPathAt(sourceID, flowID, staticID, moduleID identity.ContentID, family keyspace.Family, ordinal uint32) (identity.ContentID, bool) {
	if !c.Matches(sourceID, flowID, staticID, moduleID) || family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 || uint64(ordinal) >= uint64(len(c.state.terms[family])) {
		return identity.ContentID{}, false
	}
	id := c.state.terms[family][ordinal]
	return id, id.Available()
}

// TermAtomAt returns the exact owner-issued neutral atom stored beside one
// semantic term at Flow seal. It never creates an atom from the returned
// semantic identity, so consumers cannot silently re-derive a proposition.
func (c *Certificate) TermAtomAt(sourceID, flowID, staticID, moduleID identity.ContentID, family keyspace.Family, ordinal uint32) (region.Atom, bool) {
	if !c.Matches(sourceID, flowID, staticID, moduleID) || family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 || uint64(ordinal) >= uint64(len(c.state.atoms[family])) {
		return region.Atom{}, false
	}
	atom := c.state.atoms[family][ordinal]
	return atom, atom.Available()
}

// Seal is the single Flow assembly cut for structural semantic identities.
// It derives its planes directly from exact Source/Authored/Body/Containment/
// Outcome proofs. No ContentID plane crosses this boundary, so an adjacent
// package cannot authenticate fabricated sibling paths by matching lengths.
func Seal(cellRoles source.CellRoles, view source.View, authoredView authored.View, bodies *body.Result, bindings binding.Result, forest *containment.Result, outcomes *outcome.Result, flowID, staticID, moduleID identity.ContentID) (*Certificate, error) {
	sourceID := view.Identity().ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() || authoredView.ContentID() != flowID {
		return nil, ErrOwnerMismatch
	}
	if !cellRoles.Matches(view) || cellRoles.CellCount() != view.Identity().FamilyCount(keyspace.FamilyCell) || cellRoles.CellCount() != authoredView.Storage().Cells().Count() || !binding.Matches(&bindings, sourceID, flowID) || bindings.CellCount() != cellRoles.CellCount() {
		return nil, fmt.Errorf("semanticpath: Cell roles or Binding denominator disagrees with exact owners")
	}
	if bodies != nil && bodies.BodyCount() != view.Identity().FamilyCount(keyspace.FamilyBody) {
		return nil, fmt.Errorf("%w: got %d, want %d", ErrBodyCardinalityMismatch, bodies.BodyCount(), view.Identity().FamilyCount(keyspace.FamilyBody))
	}
	planes, err := derive(view, cellRoles, authoredView, bodies, bindings, forest, outcomes, sourceID, flowID, staticID, moduleID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDeriveFailure, err)
	}
	bodyTerms := planes.terms[keyspace.FamilyBody]
	if len(bodyTerms) != view.Identity().FamilyCount(keyspace.FamilyBody)+1 {
		return nil, fmt.Errorf("%w: got %d, want %d", ErrBodyCardinalityMismatch, len(bodyTerms)-1, view.Identity().FamilyCount(keyspace.FamilyBody))
	}
	certificate := &Certificate{state: &certificateState{sourceID: sourceID, flowID: flowID, staticID: staticID, moduleID: moduleID}}
	for ordinal := 1; ordinal < len(bodyTerms); ordinal++ {
		if !bodyTerms[ordinal].Available() {
			return nil, fmt.Errorf("%w: ordinal %d path is unavailable", ErrInvalidBodyRow, ordinal)
		}
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := view.Identity().FamilyCount(family)
		if len(planes.roots[family]) != count || len(planes.terms[family]) != count+1 || planes.terms[family][0].Available() {
			return nil, fmt.Errorf("%w: family %d", ErrFamilyPlaneShape, family)
		}
		certificate.state.roots[family] = append([]identity.ContentID(nil), planes.roots[family]...)
		certificate.state.terms[family] = append([]identity.ContentID(nil), planes.terms[family]...)
		certificate.state.atoms[family] = make([]region.Atom, len(planes.terms[family]))
		for ordinal := 1; ordinal < len(planes.terms[family]); ordinal++ {
			path := planes.terms[family][ordinal]
			if !path.Available() {
				continue
			}
			atom, atomOK := region.NewAtom(path)
			if !atomOK {
				return nil, fmt.Errorf("%w: family %d ordinal %d atom is unavailable", ErrFamilyPlaneShape, family, ordinal)
			}
			certificate.state.atoms[family][ordinal] = atom
		}
		for ordinal := 0; ordinal < count; ordinal++ {
			// A direct root may be absent from the route plane only when it can
			// never be a causal term.  SourceControl independently proves the
			// relevant root/loop rows while consuming its immutable view.
			if !certificate.state.roots[family][ordinal].Available() && !certificate.state.terms[family][ordinal+1].Available() {
				return nil, fmt.Errorf("%w: family %d ordinal %d has no root or term path", ErrUncoveredOrdinal, family, ordinal+1)
			}
		}
	}
	return certificate, nil
}

func (c *Certificate) matches(sourceID, flowID, staticID, moduleID identity.ContentID) bool {
	return c != nil && c.state != nil && c.state.matches(sourceID, flowID, staticID, moduleID)
}
func (c *certificateState) matches(sourceID, flowID, staticID, moduleID identity.ContentID) bool {
	return c != nil && c.sourceID == sourceID && c.flowID == flowID && c.staticID == staticID && c.moduleID == moduleID &&
		sourceID.Available() && flowID.Available() && staticID.Available() && moduleID.Available()
}

// VertexCatalog returns the immutable structural paths used by SourceControl's
// vertex catalogue. The quartet is checked at the boundary and checked again
// by the receiving owner before any path is consumed.
func (c *Certificate) VertexCatalog(sourceID, flowID, staticID, moduleID identity.ContentID) (*VertexCatalogPaths, bool) {
	if c == nil || !c.matches(sourceID, flowID, staticID, moduleID) {
		return nil, false
	}
	return &VertexCatalogPaths{certificate: c.state}, true
}

// OutcomePhases returns the immutable Outcome path view used by SourceControl
// to materialize its separate runtime phase schedule.
func (c *Certificate) OutcomePhases(sourceID, flowID, staticID, moduleID identity.ContentID) (*OutcomePhasePaths, bool) {
	if c == nil || !c.matches(sourceID, flowID, staticID, moduleID) {
		return nil, false
	}
	return &OutcomePhasePaths{certificate: c.state}, true
}

// Causal returns the immutable term-path view used by Causal route assembly.
func (c *Certificate) Causal(sourceID, flowID, staticID, moduleID identity.ContentID) (*CausalPaths, bool) {
	if c == nil || !c.matches(sourceID, flowID, staticID, moduleID) {
		return nil, false
	}
	return &CausalPaths{certificate: c.state}, true
}

// OutcomePhasePaths is a read-only projection of the exact sealed Outcome
// path plane.  The caller supplies an Outcome key it already owns; no dense
// ordinal or caller-supplied path plane crosses this boundary.
type OutcomePhasePaths struct{ certificate *certificateState }

// Matches is the exact owner fence for a SourceControl phase view.
func (p *OutcomePhasePaths) Matches(sourceID, flowID, staticID, moduleID identity.ContentID) bool {
	return p != nil && p.certificate != nil && p.certificate.matches(sourceID, flowID, staticID, moduleID)
}

func (p *OutcomePhasePaths) Count() int {
	if p == nil || p.certificate == nil || len(p.certificate.terms[keyspace.FamilyOutcome]) == 0 {
		return 0
	}
	return len(p.certificate.terms[keyspace.FamilyOutcome]) - 1
}

func (p *OutcomePhasePaths) At(term keyspace.Term) (identity.ContentID, bool) {
	if p == nil || p.certificate == nil || keyspace.TermFamily(term) != keyspace.FamilyOutcome {
		return identity.ContentID{}, false
	}
	ordinal := keyspace.TermOrdinal(term)
	paths := p.certificate.terms[keyspace.FamilyOutcome]
	if ordinal == 0 || uint64(ordinal) >= uint64(len(paths)) {
		return identity.ContentID{}, false
	}
	id := paths[ordinal]
	return id, id.Available()
}

// VertexCatalogPaths is a read-only, owner-qualified projection of the
// parent-issued paths needed by Flow's sealing consumers.
type VertexCatalogPaths struct{ certificate *certificateState }

// Matches is the exact owner fence for a SourceControl vertex view.
func (p *VertexCatalogPaths) Matches(sourceID, flowID, staticID, moduleID identity.ContentID) bool {
	return p != nil && p.certificate != nil && p.certificate.matches(sourceID, flowID, staticID, moduleID)
}

func (p *VertexCatalogPaths) BodyCount() int {
	if p == nil || p.certificate == nil {
		return 0
	}
	paths := p.certificate.terms[keyspace.FamilyBody]
	if len(paths) == 0 {
		return 0
	}
	return len(paths) - 1
}

func (p *VertexCatalogPaths) BodyAt(ordinal uint32) (identity.ContentID, bool) {
	if p == nil || p.certificate == nil || ordinal == 0 || uint64(ordinal) >= uint64(len(p.certificate.terms[keyspace.FamilyBody])) {
		return identity.ContentID{}, false
	}
	id := p.certificate.terms[keyspace.FamilyBody][ordinal]
	return id, id.Available()
}

func (p *VertexCatalogPaths) RootAt(family keyspace.Family, ordinal uint32) (identity.ContentID, bool) {
	if p == nil || p.certificate == nil || family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 || uint64(ordinal) > uint64(len(p.certificate.roots[family])) {
		return identity.ContentID{}, false
	}
	id := p.certificate.roots[family][ordinal-1]
	return id, id.Available()
}

func (p *VertexCatalogPaths) TermAt(family keyspace.Family, ordinal uint32) (identity.ContentID, bool) {
	if p == nil || p.certificate == nil || family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 || uint64(ordinal) >= uint64(len(p.certificate.terms[family])) {
		return identity.ContentID{}, false
	}
	id := p.certificate.terms[family][ordinal]
	return id, id.Available()
}

// CausalPaths is intentionally a coordinate-pair projection rather than a
// raw Term mapper.  Causal validates a Term under its own owner fence then
// asks for the exact family/ordinal row it already owns.
type CausalPaths struct{ certificate *certificateState }

// Matches is the exact owner fence for a Causal path view.
func (p *CausalPaths) Matches(sourceID, flowID, staticID, moduleID identity.ContentID) bool {
	return p != nil && p.certificate != nil && p.certificate.matches(sourceID, flowID, staticID, moduleID)
}

func (p *CausalPaths) At(family keyspace.Family, ordinal uint32) (identity.ContentID, bool) {
	if p == nil || p.certificate == nil || family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 || uint64(ordinal) >= uint64(len(p.certificate.terms[family])) {
		return identity.ContentID{}, false
	}
	id := p.certificate.terms[family][ordinal]
	return id, id.Available()
}
