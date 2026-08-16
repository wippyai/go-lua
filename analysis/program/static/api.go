package static

import "github.com/wippyai/go-lua/analysis/program/keyspace"

// CommitInput is the complete canonical Static receipt supplied at the
// publication boundary. Each stream is the dense Term denominator for its
// authored relation, in canonical ordinal order. The receipt is validation
// input only: a successful Commit retains none of these slices.
type CommitInput struct {
	TypeOf       []keyspace.Term
	Annotations  []keyspace.Term
	Publications []keyspace.Term
}
