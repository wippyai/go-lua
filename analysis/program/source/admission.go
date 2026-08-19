package source

import "github.com/wippyai/go-lua/analysis/program/keyspace"

// AdmitsDirectBodyFamily reports whether a Term family may occur directly in
// authored Body source order. Source owns this closed admission boundary so
// builders and consumers cannot drift into independent family switches.
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
