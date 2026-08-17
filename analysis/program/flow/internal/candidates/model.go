// Package candidates seals the small, typed operator/control-candidate projection
// used by later Flow assembly.
//
// Candidates are existing authored Terms only.  The projection has no
// synthetic rows, generic candidate registry, branch/handler selection, or
// persistence representation.  Executability is the sole reachability gate;
// the authored operator/access relation supplies the candidate family. Every
// executable Read over a LensExact or LensKey is an IndexGet candidate,
// including a FieldName Read reused as a Call callee: Call ownership does not
// change the Read's candidate family. A GenericLoop bucket contains
// executable authored GenericFor rows whose header Values relation has at
// least one fixed member.
package candidates

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Result is the immutable derived candidate projection. Every bucket stores
// existing authored Terms in canonical family-ordinal order. The source-family
// class planes are private query authority: they make typed Contains queries
// constant time without a reverse map or a retained authored graph.
//
// The class planes are zero-based by authored family ordinal. The candidate
// buckets have no sentinel because their At methods are zero-based, like the
// authored relation views.
type Result struct {
	// Provenance is the narrow owner fence for this derived projection.
	// Candidates never retain Source, authored Flow, or executable owners;
	// these scalar identities are copied once at seal and checked at every
	// downstream composition boundary.
	sourceID identity.ContentID
	flowID   identity.ContentID
	staticID identity.ContentID
	moduleID identity.ContentID

	buckets bucketStore
	classes classStore
}

// bucketStore vertically composes the typed candidate slices so Result stays
// a narrow owner/projection envelope as candidate families grow.
type bucketStore struct {
	unaryNumeric []keyspace.Term
	length       []keyspace.Term
	arithmetic   []keyspace.Term
	bitwise      []keyspace.Term
	concat       []keyspace.Term
	equality     []keyspace.Term
	order        []keyspace.Term
	indexGet     []keyspace.Term
	indexSet     []keyspace.Term
	genericLoop  []keyspace.Term
}

// classStore holds the dense private query planes. A zero slot means no
// candidate; nonzero values are private, family-specific dispositions.
type classStore struct {
	// Each index is one byte per authored source ordinal. Zero means no
	// candidate; nonzero values are private, family-specific dispositions.
	// No executable or authored authority is retained after Seal.
	unaryClass  []uint8
	binaryClass []uint8
	readClass   []uint8
	writeClass  []uint8
	loopClass   []uint8
}

// available is the query fence for the immutable projection. A Result with
// plausible bucket/class storage but any unavailable Source, Flow, Static, or
// Module identity is not usable and must behave like an empty one.
func (r *Result) available() bool {
	return r != nil && r.sourceID.Available() && r.flowID.Available() && r.staticID.Available() && r.moduleID.Available()
}

// Matches reports whether r was sealed for the exact Source, authored Flow,
// Static, and Module identities supplied by the final assembly. Unavailable
// identities never match, including a malformed Result carrying plausible
// candidate slices.
func Matches(r *Result, sourceID, flowID, staticID, moduleID identity.ContentID) bool {
	return r != nil && sourceID.Available() && flowID.Available() && staticID.Available() && moduleID.Available() &&
		r.available() &&
		r.sourceID == sourceID && r.flowID == flowID && r.staticID == staticID && r.moduleID == moduleID
}

// Each bucket is a typed view.  The distinct types intentionally prevent a
// caller from mixing Terms from unrelated candidate families.
type UnaryNumeric struct{ result *Result }
type Length struct{ result *Result }
type Arithmetic struct{ result *Result }
type Bitwise struct{ result *Result }
type Concat struct{ result *Result }
type Equality struct{ result *Result }
type Order struct{ result *Result }
type IndexGet struct{ result *Result }
type IndexSet struct{ result *Result }
type GenericLoop struct{ result *Result }
