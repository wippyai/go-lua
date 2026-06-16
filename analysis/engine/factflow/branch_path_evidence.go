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

	activeOnTrue  bool
	activeOnFalse bool
}

// BranchPathEvidenceSet groups branch path evidence emitted at the same CFG point.
type BranchPathEvidenceSet struct {
	evidence []BranchPathEvidence
}

// NewBranchPathPresenceEvidence creates path-presence evidence.
func NewBranchPathPresenceEvidence(targetPath path.Path, value presence.Value) BranchPathEvidence {
	return NewBranchPathPresenceEvidenceForEdges(targetPath, value, true, true)
}

// NewBranchPathPresenceEvidenceForEdges creates path-presence evidence for
// the selected branch edges.
func NewBranchPathPresenceEvidenceForEdges(
	targetPath path.Path,
	value presence.Value,
	activeOnTrue bool,
	activeOnFalse bool,
) BranchPathEvidence {
	return BranchPathEvidence{
		kind:          BranchPathEvidencePresence,
		path:          copyPath(targetPath),
		presence:      value,
		hasPresence:   true,
		activeOnTrue:  activeOnTrue,
		activeOnFalse: activeOnFalse,
	}
}

// NewBranchPathPresenceEvidenceOnEdge creates path-presence evidence for one
// branch edge.
func NewBranchPathPresenceEvidenceOnEdge(targetPath path.Path, value presence.Value, cond bool) BranchPathEvidence {
	return NewBranchPathPresenceEvidenceForEdges(targetPath, value, cond, !cond)
}

// NewBranchPathTruthyEvidenceOnEdge records that targetPath is truthy on one
// branch edge. It is semantic edge evidence used for root-origin recovery; it
// is not replayed into the persistent path-proof state lane.
func NewBranchPathTruthyEvidenceOnEdge(targetPath path.Path, cond bool) BranchPathEvidence {
	return BranchPathEvidence{
		kind:          BranchPathEvidenceTruthy,
		path:          copyPath(targetPath),
		activeOnTrue:  cond,
		activeOnFalse: !cond,
	}
}

// NewBranchPathEqualityEvidence creates path-equality evidence.
func NewBranchPathEqualityEvidence(leftPath path.Path, rightPath path.Path) BranchPathEvidence {
	return NewBranchPathEqualityEvidenceForEdges(leftPath, rightPath, true, true)
}

// NewBranchPathEqualityEvidenceForEdges creates path-equality evidence for
// the selected branch edges.
func NewBranchPathEqualityEvidenceForEdges(
	leftPath path.Path,
	rightPath path.Path,
	activeOnTrue bool,
	activeOnFalse bool,
) BranchPathEvidence {
	return BranchPathEvidence{
		kind:          BranchPathEvidenceEqual,
		path:          copyPath(leftPath),
		otherPath:     copyPath(rightPath),
		hasOtherPath:  true,
		activeOnTrue:  activeOnTrue,
		activeOnFalse: activeOnFalse,
	}
}

// NewBranchPathEqualityEvidenceOnEdge creates path-equality evidence for one
// branch edge.
func NewBranchPathEqualityEvidenceOnEdge(leftPath path.Path, rightPath path.Path, cond bool) BranchPathEvidence {
	return NewBranchPathEqualityEvidenceForEdges(leftPath, rightPath, cond, !cond)
}

// NewBranchPathInequalityEvidence creates path-inequality evidence.
func NewBranchPathInequalityEvidence(leftPath path.Path, rightPath path.Path) BranchPathEvidence {
	return NewBranchPathInequalityEvidenceForEdges(leftPath, rightPath, true, true)
}

// NewBranchPathInequalityEvidenceForEdges creates path-inequality evidence for
// the selected branch edges.
func NewBranchPathInequalityEvidenceForEdges(
	leftPath path.Path,
	rightPath path.Path,
	activeOnTrue bool,
	activeOnFalse bool,
) BranchPathEvidence {
	return BranchPathEvidence{
		kind:          BranchPathEvidenceNotEqual,
		path:          copyPath(leftPath),
		otherPath:     copyPath(rightPath),
		hasOtherPath:  true,
		activeOnTrue:  activeOnTrue,
		activeOnFalse: activeOnFalse,
	}
}

// NewBranchPathInequalityEvidenceOnEdge creates path-inequality evidence for
// one branch edge.
func NewBranchPathInequalityEvidenceOnEdge(leftPath path.Path, rightPath path.Path, cond bool) BranchPathEvidence {
	return NewBranchPathInequalityEvidenceForEdges(leftPath, rightPath, cond, !cond)
}

// NewBranchIndexInRangeEvidenceOnEdge records that indexPath is a proven in-range
// index for arrayPath on one branch edge.
func NewBranchIndexInRangeEvidenceOnEdge(indexPath path.Path, arrayPath path.Path, cond bool) BranchPathEvidence {
	return BranchPathEvidence{
		kind:          BranchPathEvidenceIndexInRange,
		path:          copyPath(indexPath),
		otherPath:     copyPath(arrayPath),
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
		path:          copyPath(targetPath),
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
func (p BranchPathEvidence) Path() path.Path { return copyPath(p.path) }

// ActiveOnEdge reports whether this evidence is established on a branch edge.
func (p BranchPathEvidence) ActiveOnEdge(cond bool) bool {
	if cond {
		return p.activeOnTrue
	}
	return p.activeOnFalse
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
	return copyPath(p.otherPath), true
}

func (p BranchPathEvidence) copy() BranchPathEvidence {
	p.path = copyPath(p.path)
	p.otherPath = copyPath(p.otherPath)
	return p
}

// Evidence returns the branch path evidence in deterministic order.
func (s BranchPathEvidenceSet) Evidence() []BranchPathEvidence {
	return copyBranchPathEvidenceSlice(s.evidence)
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
