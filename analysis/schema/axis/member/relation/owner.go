// Package relation owns the neutral bind-time surface of generated axis
// relations. Declaration vocabulary stays in the parent member package;
// construction imports this child only when it must reduce sealed owner rows
// to dense coordinates.
package relation

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// SourceColumn is an immutable, typed dense column published by a generated
// relation owner at schema materialization time. Its backing storage is
// private; execution can only index an owner-issued ordinal through At.
//
// Every row carries the outcome its materializer concluded. A zero-input fold
// is a total function over the candidate directory, but not every candidate
// yields a fact: an owner-issued row may be a sealed absence. Storing the
// outcome beside the value is what lets the Z form conclude that absence
// without removing the candidate from the directory the occurrence inventory
// is derived from.
type SourceColumn[V any] struct {
	values   []V
	outcomes []structure.ReductionOutcome
	sealed   bool
}

// NewSourceColumn seals one owner-materialized value slice by taking an
// independent copy. The returned column has no mutation operation. Values and
// outcomes are one row table: a length disagreement, an unavailable outcome,
// or an outcome that carries a value it must not is refused.
func NewSourceColumn[V any](values []V, outcomes []structure.ReductionOutcome) (SourceColumn[V], bool) {
	if len(values) != len(outcomes) {
		return SourceColumn[V]{}, false
	}
	for _, outcome := range outcomes {
		if !outcome.Available() || outcome == structure.Refuse {
			return SourceColumn[V]{}, false
		}
	}
	return SourceColumn[V]{
		values:   append([]V(nil), values...),
		outcomes: append([]structure.ReductionOutcome(nil), outcomes...),
		sealed:   true,
	}, true
}

// Valid distinguishes a deliberately sealed empty column from a missing
// materialization.  That distinction is load-bearing: an empty source family
// is a valid closed-world fact, while an omitted relation is not a source
// column at all.
func (column SourceColumn[V]) Valid() bool { return column.sealed }

// Count is the exact sealed dense width.
func (column SourceColumn[V]) Count() int {
	if !column.sealed {
		return 0
	}
	return len(column.values)
}

// At indexes the column directly by the owner-issued dense candidate ordinal.
// The outcome is the row's own disposition: a Concrete row stages its value,
// and any other admitted outcome stages nothing.
func (column SourceColumn[V]) At(index uint32) (V, structure.ReductionOutcome, bool) {
	if !column.sealed || uint64(index) >= uint64(len(column.values)) {
		var zero V
		return zero, structure.ReductionOutcome(0), false
	}
	return column.values[index], column.outcomes[index], true
}

// Clone returns an independent sealed column for a Program-bound runtime
// factor.  It makes the ownership break explicit even though the source
// column is immutable: runtime data has no backing slice retained by the
// cold relation owner.
func (column SourceColumn[V]) Clone() SourceColumn[V] {
	if !column.sealed {
		return SourceColumn[V]{}
	}
	clone, _ := NewSourceColumn(column.values, column.outcomes)
	return clone
}

// SourceColumns is a bind-only typed view implemented by generated relation
// owners that materialize zero-input facts.  The engine copies these sealed
// values into the bound Factor once; it never retains this provider or calls
// back into an owner while solving.
//
// RelationCount is the owner-issued relation ordinal extent, not the number
// of materialized columns.  A sparse materialization (for example relation 2
// only) therefore remains unambiguous.
type SourceColumns[V any] interface {
	RelationCount() int
	SourceFactColumn(relationOrdinal uint32) (SourceColumn[V], bool)
}

// Owner resolves an occurrence into its owner-issued dense candidate and
// projects that candidate into an axis-local coordinate. Domain values never
// cross this boundary, and implementations are not retained after Program
// construction.
//
// Addressing is a property of the relation, not of the owner: a mounted
// relation resolves a mount-qualified occurrence, while a global relation
// resolves an occurrence alone and refuses a mount. Each relation arm decides
// which of the two it is.
type Owner interface {
	// CandidateCount and CandidateAt publish the candidate SET one occurrence
	// carries. Cardinality is a property of the relation: an ordinary keyed
	// relation answers one, while an activation occurrence answers one row per
	// admitted body route. A surface that could only name "the candidate" made
	// the wide answer inexpressible, so there is no scalar spelling beside
	// this pair.
	CandidateCount(relationOrdinal uint32, mount, occurrence identity.ContentID) (int, bool)
	CandidateAt(relationOrdinal uint32, mount, occurrence identity.ContentID, index int) (uint32, bool)
	// MemberCount and MemberAt address one nested ordered member set - a
	// bounded port list - under one row of its parent relation. The ordinal is
	// the port's address, so "export port k" is a row rather than a
	// variable-length projection result.
	MemberCount(relationOrdinal, parentCandidateOrdinal uint32) (int, bool)
	MemberAt(relationOrdinal, parentCandidateOrdinal uint32, ordinal int) (uint32, bool)
	// KeyVectorCount and KeyVectorAt publish the ordered dense key vector one
	// row of this directory carries: the coordinates of ANOTHER axis that row
	// was constructed from. It is the second span a whole-vector read can be
	// taken over, and it is answered here rather than by the read's own axis
	// because the row is the only place those coordinates are grouped - the
	// axis they belong to issued them one at a time and groups them nowhere.
	//
	// The coordinate is dense in the axis it spans, which is the axis that
	// issued it. This owner passes it through and normalizes nothing.
	KeyVectorCount(relationOrdinal, candidateOrdinal uint32) (int, bool)
	KeyVectorAt(relationOrdinal, candidateOrdinal uint32, ordinal int) (uint32, bool)
	Project(relationOrdinal, projectionOrdinal, candidateOrdinal uint32) (uint32, bool)
}

// Outcome reads one candidate row's settled disposition from the local an
// attribute projection issued. The five-valued vocabulary is ordinal-addressed
// by structure, so the column needs no typed storage of its own: the attribute
// role is what says the local is a vocabulary ordinal rather than a factor
// surface index.
//
// Zero is the absent local and settles nothing. A branch that settles no
// declared member is a branch whose disposition the relation never published,
// which is a refusal to read rather than a default.
func Outcome(local uint32) (structure.ReductionOutcome, bool) {
	if local == 0 || local > uint32(^uint16(0)) {
		return structure.ReductionOutcome(0), false
	}
	outcome := structure.ReductionOutcome(local - 1)
	if !outcome.Available() || uint32(outcome.Ordinal()) != local {
		return structure.ReductionOutcome(0), false
	}
	return outcome, true
}

// OccurrenceDirectory is the sealed occurrence inventory of a global relation:
// the axis's own statement of which occurrences exist for it. A mounted
// relation has no such directory - its occurrences are the artifact's rows -
// so only owners that declare at least one global relation implement this.
type OccurrenceDirectory interface {
	OccurrenceCount(relationOrdinal uint32) (int, bool)
	OccurrenceIDAt(relationOrdinal uint32, index int) (identity.ContentID, bool)
}

// IdentityProjection is the owner surface of a relation whose rows carry
// owner-issued content identities. Project above answers a LOCAL: the address
// of a row this analyzer minted, which a uint32 carries. An identity is not an
// address - it names a subject the analyzer did not mint, a module, a body
// path, the semantic axis a role is issued under - and no dense width carries
// one, so a projection declared in the member.Identity role is read here.
//
// It is a separate interface for the reason OccurrenceDirectory above is: only
// owners that declare at least one identity projection implement it, and an
// axis publishing only locals stays a complete Owner rather than carrying a
// refusing method for a capability it never declared.
type IdentityProjection interface {
	// ProjectIdentity answers one candidate row's owner-issued identity: the
	// canonical digest, and the frame it was issued under. A content identity
	// is issued under no frame and answers zero; a semantic axis answers the
	// frame its owner minted it at, which is what reconstitutes the key.
	//
	// One call answers both because a digest without its frame is not an
	// identity - SemanticKey refuses version zero - so a second call for the
	// frame would be a second authority over one value, free to disagree with
	// the first. A pair the owner declares no identity row for refuses: an
	// absent identity and the identity of nothing are different statements.
	ProjectIdentity(relationOrdinal, projectionOrdinal, candidateOrdinal uint32) (identity.ContentID, uint64, bool)
}
