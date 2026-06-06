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

// EntryContextsWithLiveContext converts a target set into entry contexts using
// live caller-projected entry evidence. Direct targets use the live entry context
// as-is. Closure targets overlay the live context on the closure's captured
// environment because closures capture mutable locations.
func EntryContextsWithLiveContext(targets TargetSet, live DirectEntryContext) []EntryContext {
	selected := SelectTargets(targets).Targets()
	out := make([]EntryContext, 0, len(selected))
	for _, target := range selected {
		entry := live(target.Ref())
		if closure, ok := target.Closure(); ok {
			entry = EntryContextFromClosureWithLiveContext(closure, entry)
		}
		out = append(out, entry)
	}
	return out
}

// SummaryEntryTargets converts target-specific entry contexts to the summary
// package's projection target shape.
func SummaryEntryTargets(targets TargetSet, direct DirectEntryContext) []summary.CallEntryTarget {
	contexts := EntryContexts(targets, direct)
	return SummaryTargetsFromEntryContexts(contexts)
}

// SummaryEntryTargetsWithLiveContext is SummaryEntryTargets for call sites where
// caller-projected live entry evidence must also be applied to closure targets.
func SummaryEntryTargetsWithLiveContext(targets TargetSet, live DirectEntryContext) []summary.CallEntryTarget {
	contexts := EntryContextsWithLiveContext(targets, live)
	return SummaryTargetsFromEntryContexts(contexts)
}

// SummaryTargetsFromEntryContexts converts canonical entry contexts to the
// summary package's target shape.
func SummaryTargetsFromEntryContexts(contexts []EntryContext) []summary.CallEntryTarget {
	out := make([]summary.CallEntryTarget, 0, len(contexts))
	for _, ctx := range contexts {
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
