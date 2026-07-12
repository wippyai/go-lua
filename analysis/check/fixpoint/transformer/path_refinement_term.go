package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
)

// PathRefinementTerm is symbolic must-preservation metadata for one unchanged
// parameter root. It is not itself an ordinary generic Summary fact: a
// concrete certified boundary decides whether the corresponding entry root
// refinement existed and may be republished. The first admitted slice is
// intentionally root-only and identity-preserving: Path must be $N and Value
// must be the same RootParam N. Descendants and computed values require
// explicit write/alias proofs and fail closed until that proof vocabulary
// exists.
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

// preservedParam reports the boundary parameter whose identity this metadata
// certifies. PathRefinementTerm is deliberately not an ordinary Summary fact:
// a concrete boundary decides whether the corresponding root refinement was
// present on entry and therefore whether it may be projected on return.
func (p PathRefinementTerm) preservedParam(arena *Arena) (uint32, bool) {
	if arena == nil || p.Path == 0 || int(p.Path) >= len(arena.paths) || p.Value == 0 || int(p.Value) >= len(arena.values) {
		return 0, false
	}
	node := arena.paths[p.Path]
	if node.root.Kind != RootParam || len(node.segments) != 0 {
		return 0, false
	}
	value := arena.values[p.Value]
	if value.op != valueRoot || value.root != node.root {
		return 0, false
	}
	return node.root.Index, true
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
