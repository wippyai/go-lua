// Package admission owns the closed set of Term families that may occur
// directly in authored Body source order.
package admission

import "github.com/wippyai/go-lua/analysis/program/keyspace"

// AdmitsDirectBodyFamily reports whether a Term family may occur directly in
// authored Body source order. The admission policy is independent of any
// Source authority or lifecycle and is shared by Source validation and Lua
// lowering.
func AdmitsDirectBodyFamily(family keyspace.Family) bool {
	switch family {
	case keyspace.FamilyBody, keyspace.FamilyBind, keyspace.FamilyAssign,
		keyspace.FamilyCall, keyspace.FamilyBranch, keyspace.FamilyLoop,
		keyspace.FamilyReturn, keyspace.FamilyBreak, keyspace.FamilyGoto,
		keyspace.FamilyLabel, keyspace.FamilyControlFault,
		keyspace.FamilyTypeAlias, keyspace.FamilyTypeInterface:
		return true
	default:
		return false
	}
}
