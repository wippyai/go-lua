package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// IndexKeyDescriptor is the AST-free description of an indexed read's key, used
// by the store-side index-read refinement. It carries the container path plus
// the key shape (a numeric index variable with an offset, a relational key path,
// a length expression, or a literal index), so presence narrowing can be decided
// against the solved numeric/key facts without re-reading the AST.
type IndexKeyDescriptor struct {
	ContainerPath constraint.Path

	HasVar    bool
	VarName   string
	VarOffset int64

	HasKeyPath bool
	KeyPath    constraint.Path

	HasLenExpr  bool
	LenExprPath constraint.Path
	LenOffset   int64

	HasLiteral   bool
	LiteralIndex int64
}

// refineIndexReadAt refines an indexed read's result presence against the solved
// numeric/key facts at p. It removes nil only when the index is provably in
// range; otherwise result is returned unchanged (nil retained — sound). It reads
// existing numeric proofs (bounds, length refs, key presence) and never seeds new
// length facts, so it cannot affect fixpoint convergence.
func (s *Solution) refineIndexReadAt(p cfg.Point, container, result typ.Type, desc IndexKeyDescriptor) typ.Type {
	if s == nil || result == nil {
		return result
	}

	// Key-presence: a proven key membership removes the missing-key nil.
	if desc.HasKeyPath && !desc.ContainerPath.IsEmpty() && !desc.KeyPath.IsEmpty() &&
		s.HasKeyOf(p, desc.ContainerPath, desc.KeyPath) {
		if refined, ok := removeFlowNil(result); ok {
			return refined
		}
	}

	// Fixed-arity tuple (or union of tuples): numeric index proven within
	// [1, arity] is present. The arity is the min element count across non-nil
	// tuple members.
	if desc.HasVar {
		if arity, ok := narrow.TupleArity(container); ok {
			if lower, upper, ok := s.BoundsAt(p, desc.VarName); ok {
				lower += desc.VarOffset
				upper += desc.VarOffset
				if lower >= 1 && upper <= arity {
					if refined, ok := removeFlowNil(result); ok {
						return refined
					}
				}
			}
		}
	}

	// Sequence indexed by a variable bounded by its own length (i <= #arr).
	if desc.HasVar && !desc.ContainerPath.IsEmpty() {
		if lower, _, ok := s.BoundsAt(p, desc.VarName); ok && lower+desc.VarOffset >= 1 {
			if arrKey, lenOffset, ok := s.ArrayLenBoundWithOffsetAt(p, desc.VarName); ok &&
				string(desc.ContainerPath.Key()) == arrKey && lenOffset <= -desc.VarOffset {
				if refined := narrow.RefineSequenceIndex(container, result, lower+desc.VarOffset); refined != nil {
					return refined
				}
			}
		}
	}

	// Sequence indexed by a length expression (#arr + k) on the same container.
	if desc.HasLenExpr && !desc.ContainerPath.IsEmpty() && desc.LenExprPath.Equal(desc.ContainerPath) {
		// A fixed-arity tuple's #t resolves to its static arity directly.
		if arity, ok := narrow.TupleArity(container); ok {
			if refined := narrow.RefineLengthIndex(container, result, arity, desc.LenOffset); refined != nil {
				return refined
			}
		}
		if lower, _, ok := s.LengthBoundsAt(p, desc.ContainerPath); ok {
			if refined := narrow.RefineLengthIndex(container, result, lower, desc.LenOffset); refined != nil {
				return refined
			}
		}
	}

	// Sequence indexed by a literal within a proven length lower bound.
	if desc.HasLiteral && desc.LiteralIndex >= 1 && !desc.ContainerPath.IsEmpty() {
		if lower, _, ok := s.LengthBoundsAt(p, desc.ContainerPath); ok && lower >= desc.LiteralIndex {
			if refined := narrow.RefineSequenceIndex(container, result, desc.LiteralIndex); refined != nil {
				return refined
			}
		}
	}

	return result
}

// removeFlowNil drops a flow-uncertainty nil from t, returning false when t is
// not a flow-uncertain optional or removal would not change the type.
func removeFlowNil(t typ.Type) (typ.Type, bool) {
	if !narrow.NilPresenceIsOnlyFlowUncertainty(t) {
		return nil, false
	}
	refined := narrow.RemoveNil(t)
	if refined == nil || typ.IsNever(refined) || typ.TypeEquals(refined, t) {
		return nil, false
	}
	return refined, true
}
