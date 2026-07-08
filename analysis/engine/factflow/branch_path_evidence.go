package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// BranchPathEvidenceKind identifies branch/postcondition path evidence that may be
// replayed into the persistent state at edge application time.
type BranchPathEvidenceKind uint8

const (
	BranchPathEvidenceUnknown BranchPathEvidenceKind = iota
	BranchPathEvidencePresence
	BranchPathEvidenceEqual
	BranchPathEvidenceNotEqual
	BranchPathEvidenceTruthy
	BranchPathEvidenceIndexInRange
	BranchPathEvidenceFrozenTable
)

// BranchPathEvidence describes one path-shaped fact emitted by branch/postcondition
// lowering. Unlike BranchPathRelation, which performs immediate edge-local
// narrowing, a BranchPathEvidence is replayed into the must/intersection state lane so
// later writes and reads can use the established presence or path-alias fact
// until invalidated by mutation.
type BranchPathEvidence struct {
	kind BranchPathEvidenceKind
	path path.Path

	presence    presence.Value
	hasPresence bool

	otherPath    path.Path
	hasOtherPath bool

	activeOnTrue             bool
	activeOnFalse            bool
	oppositeEdgeImpliesFalsy bool
}

// BranchPathEvidenceSet groups branch path evidence emitted at the same CFG point.
type BranchPathEvidenceSet struct {
	evidence []BranchPathEvidence
}

// NewBranchPathPresenceEvidenceOnEdge creates path-presence evidence for one
// branch edge.
func NewBranchPathPresenceEvidenceOnEdge(targetPath path.Path, value presence.Value, cond bool) BranchPathEvidence {
	return BranchPathEvidence{
		kind:          BranchPathEvidencePresence,
		path:          targetPath.Clone(),
		presence:      value,
		hasPresence:   true,
		activeOnTrue:  cond,
		activeOnFalse: !cond,
	}
}

// NewBranchPathTruthyEvidenceOnEdge records that targetPath is truthy on one
// branch edge. It is semantic edge evidence used for root-origin recovery; it
// is not replayed into the persistent path-proof state lane.
func NewBranchPathTruthyEvidenceOnEdge(targetPath path.Path, cond bool) BranchPathEvidence {
	return BranchPathEvidence{
		kind:          BranchPathEvidenceTruthy,
		path:          targetPath.Clone(),
		activeOnTrue:  cond,
		activeOnFalse: !cond,
	}
}

// NewBranchPathTruthyEvidenceWithOppositeOnEdge records truthiness on one edge
// of a direct truthiness condition. Unlike truthiness implied by a compound
// condition, a direct check also proves the opposite edge is falsy.
func NewBranchPathTruthyEvidenceWithOppositeOnEdge(targetPath path.Path, cond bool) BranchPathEvidence {
	out := NewBranchPathTruthyEvidenceOnEdge(targetPath, cond)
	out.oppositeEdgeImpliesFalsy = true
	return out
}

// NewBranchPathEqualityEvidenceOnEdge creates path-equality evidence for one
// branch edge.
func NewBranchPathEqualityEvidenceOnEdge(leftPath path.Path, rightPath path.Path, cond bool) BranchPathEvidence {
	return BranchPathEvidence{
		kind:          BranchPathEvidenceEqual,
		path:          leftPath.Clone(),
		otherPath:     rightPath.Clone(),
		hasOtherPath:  true,
		activeOnTrue:  cond,
		activeOnFalse: !cond,
	}
}

// NewBranchPathInequalityEvidenceOnEdge creates path-inequality evidence for
// one branch edge.
func NewBranchPathInequalityEvidenceOnEdge(leftPath path.Path, rightPath path.Path, cond bool) BranchPathEvidence {
	return BranchPathEvidence{
		kind:          BranchPathEvidenceNotEqual,
		path:          leftPath.Clone(),
		otherPath:     rightPath.Clone(),
		hasOtherPath:  true,
		activeOnTrue:  cond,
		activeOnFalse: !cond,
	}
}

// NewBranchIndexInRangeEvidenceOnEdge records that indexPath is a proven in-range
// index for arrayPath on one branch edge.
func NewBranchIndexInRangeEvidenceOnEdge(indexPath path.Path, arrayPath path.Path, cond bool) BranchPathEvidence {
	return BranchPathEvidence{
		kind:          BranchPathEvidenceIndexInRange,
		path:          indexPath.Clone(),
		otherPath:     arrayPath.Clone(),
		hasOtherPath:  true,
		activeOnTrue:  cond,
		activeOnFalse: !cond,
	}
}

// NewBranchFrozenTableEvidenceOnEdge records that targetPath resolves to a
// frozen table identity on one branch edge.
func NewBranchFrozenTableEvidenceOnEdge(targetPath path.Path, cond bool) BranchPathEvidence {
	return BranchPathEvidence{
		kind:          BranchPathEvidenceFrozenTable,
		path:          targetPath.Clone(),
		activeOnTrue:  cond,
		activeOnFalse: !cond,
	}
}

// NewBranchPathEvidenceSet creates a branch path evidence set.
func NewBranchPathEvidenceSet(evidence ...BranchPathEvidence) BranchPathEvidenceSet {
	return BranchPathEvidenceSet{evidence: copyBranchPathEvidenceSlice(evidence)}
}

// Kind returns the evidence kind.
func (p BranchPathEvidence) Kind() BranchPathEvidenceKind { return p.kind }

// Path returns the evidence primary path.
func (p BranchPathEvidence) Path() path.Path { return p.path.Clone() }

// PathRef returns the evidence primary path for immediate read-only use.
// Callers must not mutate or retain the returned path.
func (p BranchPathEvidence) PathRef() path.Path { return p.path }

// ActiveOnEdge reports whether this evidence is established on a branch edge.
func (p BranchPathEvidence) ActiveOnEdge(cond bool) bool {
	if cond {
		return p.activeOnTrue
	}
	return p.activeOnFalse
}

// OppositeEdgeImpliesFalsy reports whether truthiness on the active edge is
// known to prove falsiness on the opposite branch edge. This holds for direct
// conditions such as `if x` and `if not x`, but not for a truthy leaf implied by
// one edge of a compound condition like `a or b`.
func (p BranchPathEvidence) OppositeEdgeImpliesFalsy() bool {
	return p.oppositeEdgeImpliesFalsy
}

// Presence returns the path presence value, if this evidence carries one.
func (p BranchPathEvidence) Presence() (presence.Value, bool) {
	return p.presence, p.hasPresence
}

// OtherPath returns the secondary path for equality/inequality proofs.
func (p BranchPathEvidence) OtherPath() (path.Path, bool) {
	if !p.hasOtherPath {
		return path.Path{}, false
	}
	return p.otherPath.Clone(), true
}

// OtherPathRef returns the secondary path for immediate read-only use.
// Callers must not mutate or retain the returned path.
func (p BranchPathEvidence) OtherPathRef() (path.Path, bool) {
	if !p.hasOtherPath {
		return path.Path{}, false
	}
	return p.otherPath, true
}

func (p BranchPathEvidence) copy() BranchPathEvidence {
	p.path = p.path.Clone()
	p.otherPath = p.otherPath.Clone()
	return p
}

// Evidence returns the branch path evidence in deterministic order.
func (s BranchPathEvidenceSet) Evidence() []BranchPathEvidence {
	return copyBranchPathEvidenceSlice(s.evidence)
}

// ForEachEvidence visits evidence without copying the backing slice.
func (s BranchPathEvidenceSet) ForEachEvidence(fn func(BranchPathEvidence) bool) {
	if fn == nil {
		return
	}
	for _, proof := range s.evidence {
		if !fn(proof) {
			return
		}
	}
}

func (s BranchPathEvidenceSet) copy() BranchPathEvidenceSet {
	return BranchPathEvidenceSet{evidence: copyBranchPathEvidenceSlice(s.evidence)}
}

func copyBranchPathEvidenceMap(in map[cfg.Point]BranchPathEvidenceSet) map[cfg.Point]BranchPathEvidenceSet {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]BranchPathEvidenceSet, len(in))
	for point, set := range in {
		out[point] = set.copy()
	}
	return out
}

func copyBranchPathEvidenceSlice(in []BranchPathEvidence) []BranchPathEvidence {
	if len(in) == 0 {
		return nil
	}
	out := make([]BranchPathEvidence, len(in))
	for i, proof := range in {
		out[i] = proof.copy()
	}
	return out
}
