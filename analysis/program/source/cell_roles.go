package source

import (
	"crypto/sha256"
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

// CellRoleKind names Source's two authored Cell orders. Flow's Binding owns
// the full lexical role vocabulary and publishes each Cell's position inside
// its definition host; this kind is the discriminator that says which of the
// two authored orders that position indexes.
type CellRoleKind uint8

const (
	CellRoleInvalid CellRoleKind = iota
	CellRoleBind
	CellRoleFormal
)

type cellRoleAuthority struct {
	authority   *authority
	denominator uint32
	sealed      bool
}

// CellRoles is the immutable Source-owned Cell column exposed by a committed
// View. It exposes no Source row slices, maps, raw Cell values, or generic
// role constructor. The authority pointer is the exact committed Source
// owner fence; equal content from another Component is not interchangeable.
type CellRoles struct {
	authority *authority
	roles     *cellRoleAuthority
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

func buildCellRoleAuthority(a *authority) (*cellRoleAuthority, error) {
	if a == nil || !a.content.Available() {
		return nil, errors.New("program/source: Cell role authority owner unavailable")
	}
	return &cellRoleAuthority{authority: a, denominator: uint32(a.identity.familyCount(keyspace.FamilyCell)), sealed: true}, nil
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
