package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// BranchProofKind identifies a branch/postcondition proof fact that may be
// replayed into the persistent state at edge application time.
type BranchProofKind uint8

const (
	BranchProofUnknown BranchProofKind = iota
	BranchProofPathPresence
	BranchProofPathEqual
	BranchProofPathNotEqual
	BranchProofPathTruthy
	BranchProofIndexInRange
)

// BranchProof describes one path-shaped fact emitted by branch/postcondition
// lowering. Unlike BranchPathRelation, which performs immediate edge-local
// narrowing, a BranchProof is replayed into the must/intersection state lane so
// later writes and reads can use the established presence or path-alias fact
// until invalidated by mutation.
type BranchProof struct {
	kind BranchProofKind
	path path.Path

	presence    presence.Value
	hasPresence bool

	otherPath    path.Path
	hasOtherPath bool

	activeOnTrue  bool
	activeOnFalse bool
}

// BranchProofSet groups branch proof facts emitted at the same CFG point.
type BranchProofSet struct {
	proofs []BranchProof
}

// NewBranchPathPresenceProof creates a path-presence proof fact.
func NewBranchPathPresenceProof(targetPath path.Path, value presence.Value) BranchProof {
	return NewBranchPathPresenceProofForEdges(targetPath, value, true, true)
}

// NewBranchPathPresenceProofForEdges creates a path-presence proof fact for
// the selected branch edges.
func NewBranchPathPresenceProofForEdges(
	targetPath path.Path,
	value presence.Value,
	activeOnTrue bool,
	activeOnFalse bool,
) BranchProof {
	return BranchProof{
		kind:          BranchProofPathPresence,
		path:          copyPath(targetPath),
		presence:      value,
		hasPresence:   true,
		activeOnTrue:  activeOnTrue,
		activeOnFalse: activeOnFalse,
	}
}

// NewBranchPathPresenceProofOnEdge creates a path-presence proof fact for one
// branch edge.
func NewBranchPathPresenceProofOnEdge(targetPath path.Path, value presence.Value, cond bool) BranchProof {
	return NewBranchPathPresenceProofForEdges(targetPath, value, cond, !cond)
}

// NewBranchPathTruthyProofOnEdge records that targetPath is truthy on one
// branch edge. It is semantic edge evidence used for root-origin recovery; it
// is not replayed into the persistent path-proof state lane.
func NewBranchPathTruthyProofOnEdge(targetPath path.Path, cond bool) BranchProof {
	return BranchProof{
		kind:          BranchProofPathTruthy,
		path:          copyPath(targetPath),
		activeOnTrue:  cond,
		activeOnFalse: !cond,
	}
}

// NewBranchPathEqualityProof creates a path-equality proof fact.
func NewBranchPathEqualityProof(leftPath path.Path, rightPath path.Path) BranchProof {
	return NewBranchPathEqualityProofForEdges(leftPath, rightPath, true, true)
}

// NewBranchPathEqualityProofForEdges creates a path-equality proof fact for
// the selected branch edges.
func NewBranchPathEqualityProofForEdges(
	leftPath path.Path,
	rightPath path.Path,
	activeOnTrue bool,
	activeOnFalse bool,
) BranchProof {
	return BranchProof{
		kind:          BranchProofPathEqual,
		path:          copyPath(leftPath),
		otherPath:     copyPath(rightPath),
		hasOtherPath:  true,
		activeOnTrue:  activeOnTrue,
		activeOnFalse: activeOnFalse,
	}
}

// NewBranchPathEqualityProofOnEdge creates a path-equality proof fact for one
// branch edge.
func NewBranchPathEqualityProofOnEdge(leftPath path.Path, rightPath path.Path, cond bool) BranchProof {
	return NewBranchPathEqualityProofForEdges(leftPath, rightPath, cond, !cond)
}

// NewBranchPathInequalityProof creates a path-inequality proof fact.
func NewBranchPathInequalityProof(leftPath path.Path, rightPath path.Path) BranchProof {
	return NewBranchPathInequalityProofForEdges(leftPath, rightPath, true, true)
}

// NewBranchPathInequalityProofForEdges creates a path-inequality proof fact for
// the selected branch edges.
func NewBranchPathInequalityProofForEdges(
	leftPath path.Path,
	rightPath path.Path,
	activeOnTrue bool,
	activeOnFalse bool,
) BranchProof {
	return BranchProof{
		kind:          BranchProofPathNotEqual,
		path:          copyPath(leftPath),
		otherPath:     copyPath(rightPath),
		hasOtherPath:  true,
		activeOnTrue:  activeOnTrue,
		activeOnFalse: activeOnFalse,
	}
}

// NewBranchPathInequalityProofOnEdge creates a path-inequality proof fact for
// one branch edge.
func NewBranchPathInequalityProofOnEdge(leftPath path.Path, rightPath path.Path, cond bool) BranchProof {
	return NewBranchPathInequalityProofForEdges(leftPath, rightPath, cond, !cond)
}

// NewBranchIndexInRangeProofOnEdge records that indexPath is a proven in-range
// index for arrayPath on one branch edge.
func NewBranchIndexInRangeProofOnEdge(indexPath path.Path, arrayPath path.Path, cond bool) BranchProof {
	return BranchProof{
		kind:          BranchProofIndexInRange,
		path:          copyPath(indexPath),
		otherPath:     copyPath(arrayPath),
		hasOtherPath:  true,
		activeOnTrue:  cond,
		activeOnFalse: !cond,
	}
}

// NewBranchProofSet creates a branch proof set.
func NewBranchProofSet(proofs ...BranchProof) BranchProofSet {
	return BranchProofSet{proofs: copyBranchProofSlice(proofs)}
}

// Kind returns the proof kind.
func (p BranchProof) Kind() BranchProofKind { return p.kind }

// Path returns the proof's primary path.
func (p BranchProof) Path() path.Path { return copyPath(p.path) }

// ActiveOnEdge reports whether this proof is established on a branch edge.
func (p BranchProof) ActiveOnEdge(cond bool) bool {
	if cond {
		return p.activeOnTrue
	}
	return p.activeOnFalse
}

// Presence returns the path presence proof value, if this proof carries one.
func (p BranchProof) Presence() (presence.Value, bool) {
	return p.presence, p.hasPresence
}

// OtherPath returns the secondary path for equality/inequality proofs.
func (p BranchProof) OtherPath() (path.Path, bool) {
	if !p.hasOtherPath {
		return path.Path{}, false
	}
	return copyPath(p.otherPath), true
}

func (p BranchProof) copy() BranchProof {
	p.path = copyPath(p.path)
	p.otherPath = copyPath(p.otherPath)
	return p
}

// Proofs returns the branch proofs in deterministic order.
func (s BranchProofSet) Proofs() []BranchProof {
	return copyBranchProofSlice(s.proofs)
}

func (s BranchProofSet) copy() BranchProofSet {
	return BranchProofSet{proofs: copyBranchProofSlice(s.proofs)}
}

func copyBranchProofMap(in map[cfg.Point]BranchProofSet) map[cfg.Point]BranchProofSet {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]BranchProofSet, len(in))
	for point, set := range in {
		out[point] = set.copy()
	}
	return out
}

func copyBranchProofSlice(in []BranchProof) []BranchProof {
	if len(in) == 0 {
		return nil
	}
	out := make([]BranchProof, len(in))
	for i, proof := range in {
		out[i] = proof.copy()
	}
	return out
}
