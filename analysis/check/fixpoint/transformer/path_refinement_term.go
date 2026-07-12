package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

// PathRefinementTerm is a symbolic normal-return refinement for one unchanged
// parameter root. The first admitted slice is intentionally root-only and
// identity-preserving: Path must be $N and Value must be the same RootParam N.
// Descendants and computed values require explicit write/alias proofs and fail
// closed until that proof vocabulary exists.
type PathRefinementTerm struct {
	Path  PathTerm
	Value ValueTerm
}

func (p PathRefinementTerm) validPreservedParamRoot(arena *Arena, shape Shape) bool {
	if arena == nil || !arena.validPath(p.Path, shape) ||
		!arena.validValue(p.Value, shape, make(map[ValueTerm]bool)) {
		return false
	}
	pathNode := arena.paths[p.Path]
	if pathNode.root.Kind != RootParam || len(pathNode.segments) != 0 {
		return false
	}
	valueNode := arena.values[p.Value]
	return valueNode.op == valueRoot && valueNode.root == pathNode.root
}

func (p PathRefinementTerm) placeholderPath(arena *Arena) (pathdom.Path, bool) {
	if arena == nil || p.Path == 0 || int(p.Path) >= len(arena.paths) {
		return pathdom.Path{}, false
	}
	node := arena.paths[p.Path]
	if node.root.Kind != RootParam || len(node.segments) != 0 {
		return pathdom.Path{}, false
	}
	return pathdom.NewPlaceholder(int(node.root.Index)), true
}

func (p PathRefinementTerm) canonical(arena *Arena) string {
	return fmt.Sprintf("%s=%s", arena.canonicalPath(p.Path), arena.canonicalValue(p.Value))
}

// rowPreservesRefinementRoots is deliberately conservative. Any path effect
// or any other normal-return fact family may encode mutation, invalidation,
// escape, or aliasing. Until those families carry an explicit non-interference
// proof, a preserved-root refinement cannot coexist with them. Allocation-only
// effects are disjoint and remain admissible.
func rowPreservesRefinementRoots(arena *EffectArena, row Row) bool {
	for _, effect := range row.Effects {
		if arena == nil || arena.Kind(effect) != EffectAllocationTemplate {
			return false
		}
	}
	facts := row.Output.NormalReturnFacts
	facts.BranchProofs = nil
	if !facts.Empty() {
		return false
	}
	// Scalar return values, entry obligations, branch proofs, and suspension do
	// not mutate or expose a parameter path. Every other Summary family is
	// rejected, including return aliases/flows, sink exposure, captured paths,
	// post-state parameter payloads, heap identity, and typestate.
	residual := row.Output.Clone()
	residual.Returns = nil
	residual.ParamObligations = nil
	residual.NormalReturnParams = nil
	residual.NormalReturnFacts = facts
	residual.MaySuspend = false
	return len(summary.PresentFactKinds(residual)) == 0 && residual.HeapKeySpace == nil
}
