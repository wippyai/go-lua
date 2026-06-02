package call

import (
	"github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/flow"
)

// TargetSet is a normalized callee-resolution result set.
//
// Direct refs are resolved from function-identity facts or static function
// facts. Closure refs are resolved independently and carry their captured entry
// context. The authoritative bits distinguish an absent axis from an axis that
// was present but top/unknown.
type TargetSet struct {
	directRefs           []summary.FuncRef
	directAuthoritative  bool
	closureRefs          []flow.ClosureRef
	closureAuthoritative bool
}

// NewTargetSet constructs a canonical target set with deterministic order,
// deduplication, and no caller-visible storage aliases.
func NewTargetSet(
	directRefs []summary.FuncRef,
	directAuthoritative bool,
	closureRefs []flow.ClosureRef,
	closureAuthoritative bool,
) TargetSet {
	return TargetSet{
		directRefs:           ref.UniqueSortedFuncRefs(directRefs),
		directAuthoritative:  directAuthoritative,
		closureRefs:          flow.ClosureRefSetOf(closureRefs...).Refs(),
		closureAuthoritative: closureAuthoritative,
	}
}

// DirectRefs returns a copy of resolved direct function refs.
func (set TargetSet) DirectRefs() []summary.FuncRef {
	return append([]summary.FuncRef(nil), set.directRefs...)
}

// SingleDirect returns the single deterministic direct callee when present.
func (set TargetSet) SingleDirect() (summary.FuncRef, bool) {
	if len(set.directRefs) != 1 {
		return summary.FuncRef{}, false
	}
	return set.directRefs[0], true
}

// DirectAuthoritative reports whether FunctionRefs was present for the call path.
func (set TargetSet) DirectAuthoritative() bool {
	return set.directAuthoritative
}

// ClosureRefs returns a copy of resolved closure refs.
func (set TargetSet) ClosureRefs() []flow.ClosureRef {
	return append([]flow.ClosureRef(nil), set.closureRefs...)
}

// ClosureAuthoritative reports whether ClosureRefs was present for the call path.
func (set TargetSet) ClosureAuthoritative() bool {
	return set.closureAuthoritative
}

// HasFiniteClosureTargets reports whether ClosureRefs resolved to concrete
// closure callees. ClosureRefs top/unknown is authoritative but has no finite
// targets, so it must not suppress direct-call fallback.
func (set TargetSet) HasFiniteClosureTargets() bool {
	return set.closureAuthoritative && len(set.closureRefs) > 0
}

// HasFiniteDirectTargets reports whether FunctionRefs or static facts resolved
// concrete direct callees. Static refs are finite targets even when FunctionRefs
// was absent and therefore non-authoritative.
func (set TargetSet) HasFiniteDirectTargets() bool {
	return len(set.directRefs) > 0
}

// UseClosureTargets is the canonical callable precedence rule: finite closure
// targets carry captured entry context and dominate coarse direct refs.
func (set TargetSet) UseClosureTargets() bool {
	return set.HasFiniteClosureTargets()
}

// UseDirectTargets reports whether consumers should use direct refs after the
// finite-closure-dominates rule has been applied.
func (set TargetSet) UseDirectTargets() bool {
	return !set.UseClosureTargets() && set.HasFiniteDirectTargets()
}
