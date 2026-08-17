package flow

import "github.com/wippyai/go-lua/analysis/program/keyspace"

// OwnerAt returns the Body owner carried by a direct Flow row. It is a
// read-only cross-owner witness used by Source orchestration; it never hands
// out a mutable row or a sibling store.
func (r *Rows) OwnerAt(family keyspace.Family, index int) (keyspace.Term, bool) {
	if r == nil || index < 0 {
		return 0, false
	}
	switch family {
	case keyspace.FamilyBind:
		if index < len(r.storage.binds) {
			return r.storage.binds[index].Owner, true
		}
	case keyspace.FamilyAssign:
		if index < len(r.storage.assigns) {
			return r.storage.assigns[index].Owner, true
		}
	case keyspace.FamilyCall:
		if index < len(r.calls.rows) {
			return r.calls.rows[index].Owner, true
		}
	case keyspace.FamilyBranch:
		if index < len(r.control.branches) {
			return r.control.branches[index].Owner, true
		}
	case keyspace.FamilyLoop:
		if index < len(r.control.loops) {
			return r.control.loops[index].Owner, true
		}
	case keyspace.FamilyReturn:
		if index < len(r.control.returns) {
			return r.control.returns[index].Owner, true
		}
	case keyspace.FamilyBreak:
		if index < len(r.control.breaks) {
			return r.control.breaks[index].Owner, true
		}
	case keyspace.FamilyGoto:
		if index < len(r.control.gotos) {
			return r.control.gotos[index].Owner, true
		}
	case keyspace.FamilyLabel:
		if index < len(r.control.labels) {
			return r.control.labels[index].Owner, true
		}
	}
	return 0, false
}
