package static

import "github.com/wippyai/go-lua/analysis/program/keyspace"

// CommitInput is the complete canonical Static input supplied at the
// publication boundary. Each stream is the dense Term denominator for its
// authored relation, in canonical ordinal order. The input is validation-only:
// a successful Commit retains none of these slices.
type CommitInput struct {
	TypeOf       []keyspace.Term
	Annotations  []keyspace.Term
	Publications []keyspace.Term
}
