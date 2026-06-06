package call

import (
	"github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/flow"
)

// SelectedTarget is one callee after applying the canonical callable precedence
// rule. It is target identity only: no summary reads, no entry-value projection,
// and no driver/program state.
type SelectedTarget struct {
	ref       summary.FuncRef
	closure   flow.ClosureRef
	isClosure bool
}

// Ref returns the selected callee identity.
func (t SelectedTarget) Ref() summary.FuncRef {
	return t.ref
}

// Closure returns the selected closure value and whether this target came from
// the ClosureRefs axis.
func (t SelectedTarget) Closure() (flow.ClosureRef, bool) {
	return t.closure, t.isClosure
}

// IsClosure reports whether this target came from the ClosureRefs axis.
func (t SelectedTarget) IsClosure() bool {
	return t.isClosure
}

// TargetSelection is the canonical selected callee set plus the fallback policy
// implied by the resolved target axes.
type TargetSelection struct {
	targets              []SelectedTarget
	closureAuthoritative bool
}

// SelectTargets applies the canonical callable precedence rule: finite closure
// targets win because they carry captured entry context; otherwise finite direct
// targets are used.
func SelectTargets(targets TargetSet) TargetSelection {
	return TargetSelection{
		targets:              selectTargets(targets),
		closureAuthoritative: targets.ClosureAuthoritative(),
	}
}

// Targets returns the selected callees after target precedence.
func (s TargetSelection) Targets() []SelectedTarget {
	return append([]SelectedTarget(nil), s.targets...)
}

// HasTargets reports whether the selection has at least one concrete callee.
func (s TargetSelection) HasTargets() bool {
	return len(s.targets) > 0
}

// HasClosureTargets reports whether the selected concrete callees came from the
// closure-value axis.
func (s TargetSelection) HasClosureTargets() bool {
	for _, target := range s.targets {
		if target.IsClosure() {
			return true
		}
	}
	return false
}

// BlocksTypeFallback reports whether the closure axis was authoritative but
// yielded no selected concrete target. In that state a type-based fallback would
// ignore the more precise closure-value evidence.
func (s TargetSelection) BlocksTypeFallback() bool {
	return !s.HasTargets() && s.closureAuthoritative
}

// AllowsCallbackFallback reports whether syntactic callback/spec fallback can be
// composed with the selected direct-call effects. Closure targets already carry
// exact entry context; an authoritative empty closure axis blocks fallback too.
func (s TargetSelection) AllowsCallbackFallback() bool {
	return !s.HasClosureTargets() && !s.BlocksTypeFallback()
}

// SelectionNeverReturns reports whether every selected concrete target is proven
// no-return. Empty selections, mixed returning/no-return selections, and
// closure-authoritative misses are not no-return proof and must not prune the
// continuation.
func SelectionNeverReturns(selection TargetSelection, hasNoReturn func(summary.FuncRef) bool) bool {
	if !selection.HasTargets() || hasNoReturn == nil {
		return false
	}
	for _, target := range selection.targets {
		if !hasNoReturn(target.Ref()) {
			return false
		}
	}
	return true
}

func selectTargets(targets TargetSet) []SelectedTarget {
	if targets.UseClosureTargets() {
		closures := targets.ClosureRefs()
		out := make([]SelectedTarget, 0, len(closures))
		for _, closure := range closures {
			out = append(out, SelectedTarget{
				ref:       ref.FromFlow(closure.Ref),
				closure:   closure,
				isClosure: true,
			})
		}
		return out
	}
	directRefs := targets.DirectRefs()
	out := make([]SelectedTarget, 0, len(directRefs))
	for _, direct := range directRefs {
		out = append(out, SelectedTarget{ref: direct})
	}
	return out
}
