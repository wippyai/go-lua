package static

import "github.com/wippyai/go-lua/analysis/program/keyspace"

// localContainmentProof is the one immutable local relation image retained
// while a Static Draft is claimed. It has no direct-return evidence, generic
// edge records, or Component pointer; terminal Finalizer actions clear the
// Draft's only reference to it.
type localContainmentProof struct {
	parents     [keyspace.FamilyCount][]keyspace.Term
	fieldOwners []keyspace.Term
}
