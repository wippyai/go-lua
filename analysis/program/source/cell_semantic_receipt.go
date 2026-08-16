package source

import (
	"crypto/sha256"
	"errors"
	"sync"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/internal/framing"
)

// CellRoleKind is the closed Source-owned portion of Cell provenance. Flow
// adds its own vararg/capture/loop roles later; Source only authenticates
// exact global atoms and authored Bind/Formal order.
type CellRoleKind uint8

const (
	CellRoleInvalid CellRoleKind = iota
	CellRoleBind
	CellRoleFormal
)

type cellRoleAuthority struct {
	authority   *authority
	denominator uint32
	locals      []cellRoleLocal
	sealed      bool
}

// cellRoleLocal is Source-private inverse order evidence. It makes the
// narrow Bind/Formal join O(1) for the one semanticpath consumer without
// releasing a Cell term, source slice, or generic mapping to siblings.
type cellRoleLocal struct {
	kind     CellRoleKind
	owner    keyspace.Term
	position uint32
	index    uint32
}

type cellRoleIssuanceState struct {
	mu        sync.Mutex
	authority *authority
	roles     *cellRoleAuthority
	used      bool
}

// CellRoleIssuance is a copy-safe, terminal receipt for the Source-owned Cell
// role catalog. It is issued only by SemanticPathIssuance and consumed once
// against the exact committed Source View.
type CellRoleIssuance struct{ state *cellRoleIssuanceState }

// CellRoleCatalog is the immutable narrow Source receipt consumed by Flow's
// cold Cell join. It exposes no Source row slices, maps, raw Cell values, or
// generic role constructor.
type CellRoleCatalog struct{ state *cellRoleIssuanceState }

// CellRole is an opaque Source role witness. Its private index is only a
// cold lookup coordinate; no dense Cell ordinal enters a semantic identity.
type CellRole struct {
	state    *cellRoleIssuanceState
	kind     CellRoleKind
	index    uint32
	position uint32
}

// IssueCellRoles grants the one Source-owned Cell role receipt. Copies of the
// parent issuance share the issuance fence and cannot grant a second copy.
func (issuance *SemanticPathIssuance) IssueCellRoles(view View) (*CellRoleIssuance, bool) {
	if issuance == nil || issuance.state == nil {
		return nil, false
	}
	state := issuance.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.used || state.cellRolesIssued {
		return nil, false
	}
	state.cellRolesIssued = true
	if state.authority == nil || view.authority != state.authority || state.cellRoles == nil || !state.cellRoles.sealed {
		return nil, false
	}
	return &CellRoleIssuance{state: &cellRoleIssuanceState{authority: state.authority, roles: state.cellRoles}}, true
}

// Consume transfers the exact role catalog. The attempt is destructive even
// when the View is foreign, matching SemanticPathIssuance's terminal fence.
func (issuance *CellRoleIssuance) Consume(view View) (CellRoleCatalog, bool) {
	if issuance == nil || issuance.state == nil {
		return CellRoleCatalog{}, false
	}
	state := issuance.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.used {
		return CellRoleCatalog{}, false
	}
	state.used = true
	if state.authority == nil || state.roles == nil || !state.roles.sealed || view.authority != state.authority {
		state.authority = nil
		state.roles = nil
		return CellRoleCatalog{}, false
	}
	return CellRoleCatalog{state: state}, true
}

func (catalog CellRoleCatalog) valid() bool {
	return catalog.state != nil && catalog.state.used && catalog.state.authority != nil && catalog.state.roles != nil && catalog.state.roles.sealed &&
		catalog.state.roles.authority == catalog.state.authority && catalog.state.authority.content.Available()
}

// Matches is the exact View/authority fence required before semanticpath or a
// sibling cold consumer accepts this catalog. Equal Source content is not
// sufficient: the committed authority pointer must be identical.
func (catalog CellRoleCatalog) Matches(view View) bool {
	return catalog.valid() && view.authority == catalog.state.authority
}

// CellCount is Source's exact FamilyCell denominator. Flow must fence its
// authored Storage.Cells denominator against this scalar before joining rows.
func (catalog CellRoleCatalog) CellCount() int {
	if !catalog.valid() {
		return 0
	}
	return int(catalog.state.roles.denominator)
}

// ExactIDForKey returns the stable Source-owned identity of one normalized
// exact atom. Key is a dense, cold selector only: the catalog neither copies
// nor enumerates the exact-atom plane.
func (catalog CellRoleCatalog) ExactIDForKey(key keyspace.Key) (identity.ContentID, bool) {
	if !catalog.valid() || key == 0 || uint64(key) > uint64(len(catalog.state.authority.keys.exact.atoms)) {
		return identity.ContentID{}, false
	}
	value := catalog.state.authority.keys.exact.atoms[key-1]
	id := exactAtomContentID(value)
	return id, id.Available()
}

// BindRoleForCell performs the exact O(1) inverse join from Flow's sealed
// Binding host to Source's authored Bind position. The Cell remains only a
// caller-provided equality witness; it is never returned or stored outside
// Source's private catalog state.
func (catalog CellRoleCatalog) BindRoleForCell(bind, cell keyspace.Term) (CellRole, bool) {
	return catalog.roleForCell(CellRoleBind, bind, cell)
}

// FormalRoleForCell is the corresponding exact Function-formal join.
func (catalog CellRoleCatalog) FormalRoleForCell(function, cell keyspace.Term) (CellRole, bool) {
	return catalog.roleForCell(CellRoleFormal, function, cell)
}

func (catalog CellRoleCatalog) roleForCell(kind CellRoleKind, owner, cell keyspace.Term) (CellRole, bool) {
	if !catalog.valid() || (kind != CellRoleBind && kind != CellRoleFormal) || !keyspace.ValidTerm(cell, keyspace.FamilyCell, int(catalog.state.roles.denominator)) {
		return CellRole{}, false
	}
	local := catalog.state.roles.locals[keyspace.TermOrdinal(cell)]
	if local.kind != kind || local.owner != owner {
		return CellRole{}, false
	}
	return CellRole{state: catalog.state, kind: kind, index: local.index, position: local.position}, true
}

// Owns authenticates a role against this exact catalog state.
func (catalog CellRoleCatalog) Owns(role CellRole) bool {
	return catalog.valid() && role.state == catalog.state && role.state.authority == catalog.state.authority && role.Available()
}

func (role CellRole) Available() bool {
	if role.state == nil || role.state.roles == nil || !role.state.roles.sealed || role.state.authority == nil || !role.state.authority.content.Available() {
		return false
	}
	a := role.state.authority
	switch role.kind {
	case CellRoleBind:
		return uint64(role.index) < uint64(len(a.order.bindTerms)) && a.validFamilyTerm(a.order.bindTerms[role.index], keyspace.FamilyCell)
	case CellRoleFormal:
		return uint64(role.index) < uint64(len(a.order.formalTerms)) && a.validFamilyTerm(a.order.formalTerms[role.index], keyspace.FamilyCell)
	default:
		return false
	}
}

func (role CellRole) Kind() CellRoleKind {
	if !role.Available() {
		return CellRoleInvalid
	}
	return role.kind
}

// Position returns the owner-local Bind/Formal position.
func (role CellRole) Position() (int, bool) {
	if !role.Available() {
		return 0, false
	}
	return int(role.position), true
}

// MatchesCell is the narrow cold join witness: it authenticates a Flow Cell
// term without exposing the Source-owned Cell term to downstream consumers.
func (role CellRole) MatchesCell(cell keyspace.Term) bool {
	if !role.Available() || !keyspace.ValidTerm(cell, keyspace.FamilyCell, int(role.state.authority.identity.counts[keyspace.FamilyCell])) {
		return false
	}
	a := role.state.authority
	switch role.kind {
	case CellRoleBind:
		return a.order.bindTerms[role.index] == cell
	case CellRoleFormal:
		return a.order.formalTerms[role.index] == cell
	default:
		return false
	}
}

func buildCellRoleAuthority(a *authority) (*cellRoleAuthority, error) {
	if a == nil || !a.content.Available() {
		return nil, errors.New("program/source: Cell role authority owner unavailable")
	}
	roles := &cellRoleAuthority{authority: a, denominator: a.identity.counts[keyspace.FamilyCell], sealed: true}
	roles.locals = make([]cellRoleLocal, int(roles.denominator)+1)
	if len(a.order.bindTerms) != len(a.order.bindOwners) || len(a.order.formalTerms) != len(a.order.formalOwners) {
		return nil, errors.New("program/source: Cell role order is incomplete")
	}
	if err := installCellRoleLocals(roles, a.order.bindTerms, a.order.bindOwners, keyspace.FamilyBind, CellRoleBind); err != nil {
		return nil, err
	}
	if err := installCellRoleLocals(roles, a.order.formalTerms, a.order.formalOwners, keyspace.FamilyFunction, CellRoleFormal); err != nil {
		return nil, err
	}
	return roles, nil
}

func installCellRoleLocals(roles *cellRoleAuthority, terms, owners []keyspace.Term, family keyspace.Family, kind CellRoleKind) error {
	if roles == nil || len(terms) != len(owners) {
		return errors.New("program/source: Cell role local order is unavailable")
	}
	var lastOwner keyspace.Term
	var position uint32
	for index, cell := range terms {
		owner := owners[index]
		if keyspace.TermFamily(owner) != family || keyspace.TermOrdinal(owner) == 0 {
			return errors.New("program/source: Cell role local owner is invalid")
		}
		if owner != lastOwner {
			lastOwner, position = owner, 0
		}
		ordinal := keyspace.TermOrdinal(cell)
		if !keyspace.ValidTerm(cell, keyspace.FamilyCell, int(roles.denominator)) || roles.locals[ordinal].kind != CellRoleInvalid {
			return errors.New("program/source: Cell role local membership is invalid")
		}
		roles.locals[ordinal] = cellRoleLocal{kind: kind, owner: owner, position: position, index: uint32(index)}
		position++
	}
	return nil
}

func exactAtomContentID(value keyspace.LiteralValue) (id identity.ContentID) {
	h := sha256.New()
	var writer framing.Writer
	if writer.Reset(h, "program/source/exact-atom-semantic", 1) != nil || !contentExactValue(&writer, value) || writer.Finish() != nil {
		return identity.ContentID{}
	}
	if sum := h.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}
