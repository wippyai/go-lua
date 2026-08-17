// Package binaryprimitive owns Flow's narrow primitive-binary projection.
//
// The projection contains only executable arithmetic, bitwise, equality, and
// order Binary rows.  It is a derived view over the already sealed Source,
// authored Flow, candidate, and causal authorities; it does not retain any
// of those owners or introduce a second semantic vocabulary.
package binaryprimitive

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Operation is the raw authored Binary row behind a Primitive.  The operands
// are deliberately not normalized here: normalization belongs only to the
// comparison projection and must remain visible as a separate derived fact.
type Operation struct {
	Owner       keyspace.Term
	Op          kind.BinaryOp
	Left, Right keyspace.Term
}

// Comparison is the optional branch interpretation of a Primitive.  Branch
// and its two bodies are authored identities. Left and Right are the ordered
// comparison operands after the closed relational normalization law. Invert
// is true only for ~=.
type Comparison struct {
	Branch              keyspace.Term
	TrueBody, FalseBody keyspace.Term
	Left, Right         keyspace.Term
	Invert              bool
}

type primitiveRow struct {
	source     keyspace.Term
	operation  Operation
	comparison Comparison
	hasCompare bool
}

// Result is the immutable primitive-binary projection.  The four content
// identities are its complete provenance fence.  slots is one-based by the
// authored Binary ordinal; a zero slot means that the Binary is not one of the
// retained primitive candidates. buckets keep the same canonical ordinal
// order as authored/candidate rows and contain no newly minted identities.
type Result struct {
	sourceID identity.ContentID
	flowID   identity.ContentID
	staticID identity.ContentID
	moduleID identity.ContentID

	slots      []uint32
	primitives []primitiveRow
	buckets    bucketStore
}

type bucketStore struct {
	arithmetic []keyspace.Term
	bitwise    []keyspace.Term
	equality   []keyspace.Term
	order      []keyspace.Term
}

// Primitive is an opaque occurrence handle.  Callers can inspect only the
// typed projections exposed by this package; the backing slot and Result are
// intentionally private so a consumer cannot manufacture or mix rows.
type Primitive struct {
	result *Result
	slot   uint32
}

func (r *Result) available() bool {
	return r != nil && r.sourceID.Available() && r.flowID.Available() &&
		r.staticID.Available() && r.moduleID.Available()
}

// Matches reports whether r belongs to the exact Source, authored Flow,
// Static, and Module identities supplied by assembly. Unavailable identities
// never match, even if a malformed result happens to contain rows.
func Matches(r *Result, sourceID, flowID, staticID, moduleID identity.ContentID) bool {
	return r != nil && r.available() && sourceID.Available() && flowID.Available() &&
		staticID.Available() && moduleID.Available() && r.sourceID == sourceID &&
		r.flowID == flowID && r.staticID == staticID && r.moduleID == moduleID
}

// Arithmetic, Bitwise, Equality, and Order are the four closed typed buckets
// retained by this projection.
type Arithmetic struct{ result *Result }
type Bitwise struct{ result *Result }
type Equality struct{ result *Result }
type Order struct{ result *Result }

// Arithmetic returns the primitive arithmetic bucket.
func (r *Result) Arithmetic() Arithmetic { return Arithmetic{result: r} }

// Bitwise returns the primitive bitwise bucket.
func (r *Result) Bitwise() Bitwise { return Bitwise{result: r} }

// Equality returns the primitive equality bucket.
func (r *Result) Equality() Equality { return Equality{result: r} }

// Order returns the primitive relational-order bucket.
func (r *Result) Order() Order { return Order{result: r} }
