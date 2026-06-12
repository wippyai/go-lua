package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// BranchProofKind identifies a branch/postcondition proof fact.
type BranchProofKind uint8

const (
	BranchProofUnknown BranchProofKind = iota
	BranchProofPathPresence
	BranchProofPathEqual
	BranchProofPathNotEqual
)

// BranchProof describes one proof fact emitted by branch/postcondition
// lowering. It is path-shaped transfer evidence, not application state.
type BranchProof struct {
	kind BranchProofKind
	path path.Path

	presence    presence.Value
	hasPresence bool

	otherPath    path.Path
	hasOtherPath bool
}

// BranchProofSet groups branch proof facts emitted at the same CFG point.
type BranchProofSet struct {
	proofs []BranchProof
}

// NewBranchPathPresenceProof creates a path-presence proof fact.
func NewBranchPathPresenceProof(targetPath path.Path, value presence.Value) BranchProof {
	return BranchProof{
		kind:        BranchProofPathPresence,
		path:        copyPath(targetPath),
		presence:    value,
		hasPresence: true,
	}
}

// NewBranchPathEqualityProof creates a path-equality proof fact.
func NewBranchPathEqualityProof(leftPath path.Path, rightPath path.Path) BranchProof {
	return BranchProof{
		kind:         BranchProofPathEqual,
		path:         copyPath(leftPath),
		otherPath:    copyPath(rightPath),
		hasOtherPath: true,
	}
}

// NewBranchPathInequalityProof creates a path-inequality proof fact.
func NewBranchPathInequalityProof(leftPath path.Path, rightPath path.Path) BranchProof {
	return BranchProof{
		kind:         BranchProofPathNotEqual,
		path:         copyPath(leftPath),
		otherPath:    copyPath(rightPath),
		hasOtherPath: true,
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
