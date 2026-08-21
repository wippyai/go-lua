package source

import (
	"crypto/sha256"
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
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

// CellRoles is the immutable Source-owned Cell column exposed by a committed
// View. It exposes no Source row slices, maps, raw Cell values, or generic
// role constructor. The authority pointer is the exact committed Source
// owner fence; equal content from another Component is not interchangeable.
type CellRoles struct {
	authority *authority
	roles     *cellRoleAuthority
}

// CellRole is an opaque Source role witness. Its private index is only a
// cold lookup coordinate; no dense Cell ordinal enters a semantic identity.
type CellRole struct {
	roles    *cellRoleAuthority
	kind     CellRoleKind
	index    uint32
	position uint32
}

func (roles CellRoles) valid() bool {
	return roles.authority != nil && roles.roles != nil && roles.roles.sealed &&
		roles.roles.authority == roles.authority && roles.authority.content.Available()
}

// Matches is the exact View/authority fence required before semanticpath or a
// sibling cold consumer accepts these roles. Equal Source content is not
// sufficient: the committed authority pointer must be identical.
func (roles CellRoles) Matches(view View) bool {
	return roles.valid() && view.authority == roles.authority
}

// CellCount is Source's exact FamilyCell denominator. Flow must fence its
// authored Storage.Cells denominator against this scalar before joining rows.
func (roles CellRoles) CellCount() int {
	if !roles.valid() {
		return 0
	}
	return int(roles.roles.denominator)
}

// ExactIDForKey returns the stable Source-owned identity of one normalized
// exact atom. Key is a dense, cold selector only: the roles view neither copies
// nor enumerates the exact-atom plane.
func (roles CellRoles) ExactIDForKey(key keyspace.Key) (identity.ContentID, bool) {
	if !roles.valid() || key == 0 || uint64(key) > uint64(len(roles.authority.keys.exact.atoms)) {
		return identity.ContentID{}, false
	}
	value := roles.authority.keys.exact.atoms[key-1]
	id := exactAtomContentID(value)
	return id, id.Available()
}

// BindRoleForCell performs the exact O(1) inverse join from Flow's sealed
// Binding host to Source's authored Bind position. The Cell remains only a
// caller-provided equality witness; it is never returned or stored outside
// Source's private role state.
func (roles CellRoles) BindRoleForCell(bind, cell keyspace.Term) (CellRole, bool) {
	return roles.roleForCell(CellRoleBind, bind, cell)
}

// FormalRoleForCell is the corresponding exact Function-formal join.
func (roles CellRoles) FormalRoleForCell(function, cell keyspace.Term) (CellRole, bool) {
	return roles.roleForCell(CellRoleFormal, function, cell)
}

func (roles CellRoles) roleForCell(kind CellRoleKind, owner, cell keyspace.Term) (CellRole, bool) {
	if !roles.valid() || (kind != CellRoleBind && kind != CellRoleFormal) || !keyspace.ValidTerm(cell, keyspace.FamilyCell, int(roles.roles.denominator)) {
		return CellRole{}, false
	}
	local := roles.roles.locals[keyspace.TermOrdinal(cell)]
	if local.kind != kind || local.owner != owner {
		return CellRole{}, false
	}
	return CellRole{roles: roles.roles, kind: kind, index: local.index, position: local.position}, true
}

// Owns authenticates a role against this exact Source-owned Cell column.
func (roles CellRoles) Owns(role CellRole) bool {
	return roles.valid() && role.roles == roles.roles && role.roles.authority == roles.authority && role.Available()
}

func (role CellRole) Available() bool {
	if role.roles == nil || !role.roles.sealed || role.roles.authority == nil || !role.roles.authority.content.Available() {
		return false
	}
	a := role.roles.authority
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
	if !role.Available() || !keyspace.ValidTerm(cell, keyspace.FamilyCell, role.roles.authority.identity.familyCount(keyspace.FamilyCell)) {
		return false
	}
	a := role.roles.authority
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
	roles := &cellRoleAuthority{authority: a, denominator: uint32(a.identity.familyCount(keyspace.FamilyCell)), sealed: true}
	roles.locals = make([]cellRoleLocal, int(roles.denominator)+1)
	if err := installCellRoleLocals(roles, a.order.bindTerms, a.order.bindRanges, keyspace.FamilyBind, CellRoleBind); err != nil {
		return nil, err
	}
	if err := installCellRoleLocals(roles, a.order.formalTerms, a.order.formalRanges, keyspace.FamilyFunction, CellRoleFormal); err != nil {
		return nil, err
	}
	return roles, nil
}

func installCellRoleLocals(roles *cellRoleAuthority, terms []keyspace.Term, ranges []termRange, family keyspace.Family, kind CellRoleKind) error {
	if roles == nil {
		return errors.New("program/source: Cell role local order is unavailable")
	}
	previousEnd := uint32(0)
	for ownerOrdinal, row := range ranges {
		if !validRange(terms, row) || row.start != previousEnd {
			return errors.New("program/source: Cell role local range is invalid")
		}
		owner := keyspace.MakeTerm(family, uint32(ownerOrdinal+1))
		if owner == 0 {
			return errors.New("program/source: Cell role local owner is invalid")
		}
		for index := row.start; index < row.end; index++ {
			cell := terms[index]
			ordinal := keyspace.TermOrdinal(cell)
			position := index - row.start
			if !keyspace.ValidTerm(cell, keyspace.FamilyCell, int(roles.denominator)) || roles.locals[ordinal].kind != CellRoleInvalid {
				return errors.New("program/source: Cell role local membership is invalid")
			}
			roles.locals[ordinal] = cellRoleLocal{kind: kind, owner: owner, position: position, index: index}
		}
		previousEnd = row.end
	}
	if previousEnd != uint32(len(terms)) {
		return errors.New("program/source: Cell role local order is incomplete")
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
