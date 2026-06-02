package call

import (
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// SelectedEntryContext maps one selected call target to the exact callee entry
// context used for summary lookup. It is supplied by the driver because it owns
// caller-state axes; this package owns the target-selection/application shape.
type SelectedEntryContext func(SelectedTarget) EntryContext

// SummaryLookup reads the summary for one already-normalized callee entry
// context. Implementations may read the live recursive summary query or an
// immutable converged snapshot.
type SummaryLookup func(EntryContext) summary.Summary

// SummaryTargetInfo provides per-target metadata needed by summary projection
// without letting this package read driver/program state.
type SummaryTargetInfo struct {
	DeclaredReturns    func(SelectedTarget) bool
	SignatureReturns   func(SelectedTarget) []typ.Type
	SignatureRelations func(SelectedTarget) flow.ReturnRelations
}

// SummaryProjectionForTargets applies the canonical callable precedence rule,
// converts each selected target to an entry-context summary read, and returns the
// summary-owned projection carrier plus the selection fallback state.
func SummaryProjectionForTargets(
	targets TargetSet,
	entryContext SelectedEntryContext,
	lookup SummaryLookup,
	info SummaryTargetInfo,
) (summary.CallSummaryProjection, TargetSelection) {
	selection := SelectTargets(targets)
	selected := selection.Targets()
	if len(selected) == 0 || entryContext == nil || lookup == nil {
		return summary.CallSummaryProjection{}, selection
	}

	out := make([]summary.CallSummaryTarget, 0, len(selected))
	for _, target := range selected {
		ctx := entryContext(target)
		next := summary.CallSummaryTarget{
			Ref:     target.Ref(),
			Summary: lookup(ctx),
		}
		if info.DeclaredReturns != nil {
			next.DeclaredReturns = info.DeclaredReturns(target)
		}
		if info.SignatureReturns != nil {
			next.SignatureReturns = info.SignatureReturns(target)
		}
		if info.SignatureRelations != nil {
			next.SignatureRelations = info.SignatureRelations(target)
		}
		out = append(out, next)
	}
	return summary.CallSummaryProjection{Targets: out}, selection
}

// CallOutcomeForTargets builds the canonical selected-target call outcome once.
// Call-site policies should project return values, returned identities,
// relations, effects, and no-return facts from this value instead of rebuilding
// summary target sets independently.
func CallOutcomeForTargets(
	targets TargetSet,
	entryContext SelectedEntryContext,
	lookup SummaryLookup,
	info SummaryTargetInfo,
) CallOutcome {
	projection, selection := SummaryProjectionForTargets(targets, entryContext, lookup, info)
	return CallOutcome{Projection: projection, Selection: selection}
}
