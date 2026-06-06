package call

import "github.com/wippyai/go-lua/compiler/check/canonical/summary"

// DirectEntryContext builds the entry context for a direct callee target.
type DirectEntryContext func(ref summary.FuncRef) EntryContext

// SummaryEntryTargetsWithLiveContext converts a target set into the summary
// package's projection target shape. Direct targets use caller-projected live
// entry evidence as-is. Closure targets overlay that live context on the
// closure's captured environment because closures capture mutable locations.
func SummaryEntryTargetsWithLiveContext(targets TargetSet, live DirectEntryContext) []summary.CallEntryTarget {
	selected := SelectTargets(targets).Targets()
	out := make([]summary.CallEntryTarget, 0, len(selected))
	for _, target := range selected {
		ctx := live(target.Ref())
		if closure, ok := target.Closure(); ok {
			ctx = EntryContextFromClosureWithLiveContext(closure, ctx)
		}
		out = append(out, summary.CallEntryTarget{
			Ref:               ctx.Ref(),
			EntryCells:        ctx.CaptureCells(),
			EntryFunctionRefs: ctx.FunctionRefs(),
			EntryClosureRefs:  ctx.ClosureRefs(),
			EntryFacts:        ctx.EntryFacts(),
		})
	}
	return out
}
