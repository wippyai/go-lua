// Package semanticpath is Flow's sole structural semantic-path authority.
//
// It deliberately sits below SourceControl and Causal.  A certificate is
// sealed once from Flow's exact Source/containment publication, and can issue
// one typed, destructive receipt to each consumer.  Consumers never receive
// a ContentID plane, so they cannot splice a sibling's paths into an
// otherwise authentic owner tuple.
package semanticpath

import (
	"errors"
	"fmt"
	"sync"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Certificate owns the body-qualified structural path planes for exactly one
// committed Source/Flow/Static/Module quartet.  Its rows are private and are
// released as narrow, one-shot consumer receipts only.
type Certificate struct {
	state *certificateState
}
type certificateState struct {
	mu            sync.Mutex
	sourceID      identity.ContentID
	flowID        identity.ContentID
	staticID      identity.ContentID
	moduleID      identity.ContentID
	body          []identity.ContentID
	roots         [keyspace.FamilyCount][]identity.ContentID
	terms         [keyspace.FamilyCount][]identity.ContentID
	vertexIssued  bool
	causalIssued  bool
	outcomeIssued bool
}

// Seal's failure classes remain intentionally narrow: assembly reports the
// exact failed structural invariant without exposing a way to construct a
// certificate.  The wrapped detail identifies the offending row or family.
var (
	ErrIssuanceRejected        = errors.New("semanticpath: commit issuance rejected")
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

// BodyPathAt, RootPathAt, and TermPathAt are Flow's narrow projections of
// the sole sealed certificate. They expose one already-issued identity, never
// an extendable plane or Term mapper.
func (c *Certificate) BodyPathAt(sourceID, flowID, staticID, moduleID identity.ContentID, ordinal uint32) (identity.ContentID, bool) {
	if !c.Matches(sourceID, flowID, staticID, moduleID) || ordinal == 0 || uint64(ordinal) > uint64(len(c.state.body)) {
		return identity.ContentID{}, false
	}
	id := c.state.body[ordinal-1]
	return id, id.Available()
}

func (c *Certificate) RootPathAt(sourceID, flowID, staticID, moduleID identity.ContentID, family keyspace.Family, ordinal uint32) (identity.ContentID, bool) {
	if !c.Matches(sourceID, flowID, staticID, moduleID) || family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 || uint64(ordinal) > uint64(len(c.state.roots[family])) {
		return identity.ContentID{}, false
	}
	id := c.state.roots[family][ordinal-1]
	return id, id.Available()
}

func (c *Certificate) TermPathAt(sourceID, flowID, staticID, moduleID identity.ContentID, family keyspace.Family, ordinal uint32) (identity.ContentID, bool) {
	if !c.Matches(sourceID, flowID, staticID, moduleID) || family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 || uint64(ordinal) >= uint64(len(c.state.terms[family])) {
		return identity.ContentID{}, false
	}
	id := c.state.terms[family][ordinal]
	return id, id.Available()
}

// Seal is the single Flow assembly cut for structural semantic identities.
// It derives its planes directly from exact Source/Authored/Body/Containment/
// Outcome proofs. No ContentID plane crosses this boundary, so an adjacent
// package cannot authenticate fabricated sibling paths by matching lengths.
func Seal(issuance *source.SemanticPathIssuance, cellRoles source.CellRoleCatalog, view source.View, authoredView authored.View, bodies *body.Result, bindings binding.Result, forest *containment.Result, outcomes *outcome.Result, flowID, staticID, moduleID identity.ContentID) (*Certificate, error) {
	sourceID := view.Identity().ContentID()
	if issuance == nil || !issuance.ConsumeSemanticPathIssuance(view) {
		return nil, ErrIssuanceRejected
	}
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() || authoredView.Cold().ContentID() != flowID {
		return nil, ErrOwnerMismatch
	}
	if !cellRoles.Matches(view) || cellRoles.CellCount() != view.Identity().FamilyCount(keyspace.FamilyCell) || cellRoles.CellCount() != authoredView.Storage().Cells().Count() || !binding.Matches(&bindings, sourceID, flowID) || bindings.CellCount() != cellRoles.CellCount() {
		return nil, fmt.Errorf("semanticpath: Cell role catalog or Binding denominator disagrees with exact owners")
	}
	if bodies != nil && bodies.BodyCount() != view.Identity().FamilyCount(keyspace.FamilyBody) {
		return nil, fmt.Errorf("%w: got %d, want %d", ErrBodyCardinalityMismatch, bodies.BodyCount(), view.Identity().FamilyCount(keyspace.FamilyBody))
	}
	planes, err := derive(view, cellRoles, authoredView, bodies, bindings, forest, outcomes, sourceID, flowID, staticID, moduleID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDeriveFailure, err)
	}
	if len(planes.body) != view.Identity().FamilyCount(keyspace.FamilyBody) {
		return nil, fmt.Errorf("%w: got %d, want %d", ErrBodyCardinalityMismatch, len(planes.body), view.Identity().FamilyCount(keyspace.FamilyBody))
	}
	certificate := &Certificate{state: &certificateState{sourceID: sourceID, flowID: flowID, staticID: staticID, moduleID: moduleID, body: append([]identity.ContentID(nil), planes.body...)}}
	for ordinal := range certificate.state.body {
		if !certificate.state.body[ordinal].Available() {
			return nil, fmt.Errorf("%w: ordinal %d path is unavailable", ErrInvalidBodyRow, ordinal+1)
		}
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := view.Identity().FamilyCount(family)
		if len(planes.roots[family]) != count || len(planes.terms[family]) != count+1 || planes.terms[family][0].Available() {
			return nil, fmt.Errorf("%w: family %d", ErrFamilyPlaneShape, family)
		}
		certificate.state.roots[family] = append([]identity.ContentID(nil), planes.roots[family]...)
		certificate.state.terms[family] = append([]identity.ContentID(nil), planes.terms[family]...)
		for ordinal := 0; ordinal < count; ordinal++ {
			// A direct root may be absent from the route plane only when it can
			// never be a causal term.  SourceControl independently proves the
			// relevant root/loop rows while consuming its receipt.
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

// VertexCatalogReceipt is an exact, destructive grant for SourceControl's
// vertex catalogue.  It contains no exported plane or public constructor.
type VertexCatalogReceipt struct {
	state *vertexCatalogReceiptState
}
type vertexCatalogReceiptState struct {
	mu          sync.Mutex
	certificate *certificateState
	used        bool
}

func (c *Certificate) IssueVertexCatalogReceipt() (*VertexCatalogReceipt, bool) {
	if c == nil || c.state == nil {
		return nil, false
	}
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	if c.state.vertexIssued {
		return nil, false
	}
	c.state.vertexIssued = true
	return &VertexCatalogReceipt{state: &vertexCatalogReceiptState{certificate: c.state}}, true
}

func (r *VertexCatalogReceipt) Consume(sourceID, flowID, staticID, moduleID identity.ContentID) (*VertexCatalogPaths, bool) {
	if r == nil || r.state == nil {
		return nil, false
	}
	state := r.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.used {
		return nil, false
	}
	certificate := state.certificate
	state.used = true
	state.certificate = nil
	if certificate == nil || !certificate.matches(sourceID, flowID, staticID, moduleID) {
		return nil, false
	}
	return &VertexCatalogPaths{certificate: certificate}, true
}

// OutcomePhaseReceipt is an exact, destructive grant for SourceControl's
// per-Outcome phase issuer.  It is deliberately separate from both the
// vertex-catalog and Causal receipts: a copied Certificate can issue neither
// a second receipt nor a fresh projection of the same Outcome plane.
type OutcomePhaseReceipt struct {
	state *outcomePhaseReceiptState
}

type outcomePhaseReceiptState struct {
	mu          sync.Mutex
	certificate *certificateState
	used        bool
}

func (c *Certificate) IssueOutcomePhaseReceipt() (*OutcomePhaseReceipt, bool) {
	if c == nil || c.state == nil {
		return nil, false
	}
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	if c.state.outcomeIssued {
		return nil, false
	}
	c.state.outcomeIssued = true
	return &OutcomePhaseReceipt{state: &outcomePhaseReceiptState{certificate: c.state}}, true
}

// Consume destructively clears the receipt before checking the caller's
// quartet.  Foreign or copied callers therefore burn the exact retry and
// cannot probe a live semantic Outcome plane.
func (r *OutcomePhaseReceipt) Consume(sourceID, flowID, staticID, moduleID identity.ContentID) (*OutcomePhasePaths, bool) {
	if r == nil || r.state == nil {
		return nil, false
	}
	state := r.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.used {
		return nil, false
	}
	certificate := state.certificate
	state.used = true
	state.certificate = nil
	if certificate == nil || !certificate.matches(sourceID, flowID, staticID, moduleID) {
		return nil, false
	}
	return &OutcomePhasePaths{certificate: certificate}, true
}

// OutcomePhasePaths is a read-only projection of the exact sealed Outcome
// path plane.  The caller supplies an Outcome key it already owns; no dense
// ordinal or caller-supplied path plane crosses this boundary.
type OutcomePhasePaths struct{ certificate *certificateState }

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

// VertexCatalogPaths is a read-only, owner-qualified projection.  It exposes
// only the three coordinates SourceControl needs to derive phase paths.
type VertexCatalogPaths struct{ certificate *certificateState }

func (p *VertexCatalogPaths) BodyCount() int {
	if p == nil || p.certificate == nil {
		return 0
	}
	return len(p.certificate.body)
}

func (p *VertexCatalogPaths) BodyAt(ordinal uint32) (identity.ContentID, bool) {
	if p == nil || p.certificate == nil || ordinal == 0 || uint64(ordinal) > uint64(len(p.certificate.body)) {
		return identity.ContentID{}, false
	}
	id := p.certificate.body[ordinal-1]
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

// CausalReceipt is a second exact, destructive grant.  Causal receives a
// distinct typed projection and cannot replay SourceControl's receipt.
type CausalReceipt struct {
	state *causalReceiptState
}
type causalReceiptState struct {
	mu          sync.Mutex
	certificate *certificateState
	used        bool
}

func (c *Certificate) IssueCausalReceipt() (*CausalReceipt, bool) {
	if c == nil || c.state == nil {
		return nil, false
	}
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	if c.state.causalIssued {
		return nil, false
	}
	c.state.causalIssued = true
	return &CausalReceipt{state: &causalReceiptState{certificate: c.state}}, true
}

func (r *CausalReceipt) Consume(sourceID, flowID, staticID, moduleID identity.ContentID) (*CausalPaths, bool) {
	if r == nil || r.state == nil {
		return nil, false
	}
	state := r.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.used {
		return nil, false
	}
	certificate := state.certificate
	state.used = true
	state.certificate = nil
	if certificate == nil || !certificate.matches(sourceID, flowID, staticID, moduleID) {
		return nil, false
	}
	return &CausalPaths{certificate: certificate}, true
}

// CausalPaths is intentionally a coordinate-pair projection rather than a
// raw Term mapper.  Causal validates a Term under its own owner fence then
// asks for the exact family/ordinal row it already owns.
type CausalPaths struct{ certificate *certificateState }

func (p *CausalPaths) At(family keyspace.Family, ordinal uint32) (identity.ContentID, bool) {
	if p == nil || p.certificate == nil || family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 || uint64(ordinal) >= uint64(len(p.certificate.terms[family])) {
		return identity.ContentID{}, false
	}
	id := p.certificate.terms[family][ordinal]
	return id, id.Available()
}
