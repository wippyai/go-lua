package call

import "github.com/wippyai/go-lua/compiler/check/canonical/summary"

// DirectEntryContext builds the entry context for a direct callee target.
type DirectEntryContext func(ref summary.FuncRef) EntryContext

// EntryContexts converts a target set into target-specific entry contexts using
// the canonical target precedence rule. Closure targets carry their own captured
// entry axes; direct targets derive entry axes from the caller state via direct.
func EntryContexts(targets TargetSet, direct DirectEntryContext) []EntryContext {
	selected := SelectTargets(targets).Targets()
	out := make([]EntryContext, 0, len(selected))
	for _, target := range selected {
		if closure, ok := target.Closure(); ok {
			out = append(out, EntryContextFromClosure(target.Ref(), closure, nil))
			continue
		}
		out = append(out, direct(target.Ref()))
	}
	return out
}

// SummaryEntryTargets converts target-specific entry contexts to the summary
// package's projection target shape.
func SummaryEntryTargets(targets TargetSet, direct DirectEntryContext) []summary.CallEntryTarget {
	contexts := EntryContexts(targets, direct)
	out := make([]summary.CallEntryTarget, 0, len(contexts))
	for _, ctx := range contexts {
		out = append(out, summary.CallEntryTarget{
			Ref:               ctx.Ref(),
			EntryCells:        ctx.CaptureCells(),
			EntryFunctionRefs: ctx.FunctionRefs(),
			EntryClosureRefs:  ctx.ClosureRefs(),
		})
	}
	return out
}
