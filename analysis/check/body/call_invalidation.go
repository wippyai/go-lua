package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// CallPathInvalidation is a concrete path invalidation caused by a call after
// substituting the callee's placeholder paths onto the caller's receiver and
// arguments.
type CallPathInvalidation struct {
	Path          pathdom.Path
	RootRebinding bool
}

// CallMayInvalidateTrackedPath reports whether the call at point may invalidate
// target. Unknown call effects are treated conservatively as invalidating when
// the call references target through its receiver or arguments.
func (r *Result) CallMayInvalidateTrackedPath(point cfg.Point, target pathdom.Path) bool {
	if r == nil || target.IsEmpty() {
		return false
	}
	site, hasSite := r.CallSite(point)
	if !hasSite {
		return false
	}
	view, ok := r.CallView(point)
	if !ok {
		return false
	}
	fact, _ := view.Borrowed()
	if !r.callFactReferencesTrackedPath(site, fact, target) {
		return false
	}
	outcome, hasOutcome := r.CallOutcomeAt(point)
	if !hasOutcome {
		if r.CallSiteHasExactEmptyGuardInvalidationSummary(site) {
			return false
		}
		return true
	}
	if r.CallOutcomeHasCovariantExposureForTarget(site, outcome, target) {
		return true
	}
	if !r.CallOutcomeHasExactGuardInvalidationSummary(site, outcome, true) {
		return true
	}
	if CallOutcomeHasGlobalGuardInvalidation(outcome) {
		return true
	}
	invalidated, ok := r.CallOutcomeGuardInvalidationPaths(site, outcome)
	if !ok {
		return true
	}
	for _, invalidation := range invalidated {
		if target.Overlaps(invalidation.Path) {
			return true
		}
	}
	return false
}

// CallMayInvalidateTrackedPathBetween reports whether any call reachable from
// from and able to reach to may invalidate target. The destination point is
// excluded because this query answers whether target survived up to that point.
func (r *Result) CallMayInvalidateTrackedPathBetween(from, to cfg.Point, target pathdom.Path) bool {
	if r == nil || target.IsEmpty() {
		return false
	}
	graph := r.Graph()
	if graph == nil {
		return false
	}
	for _, candidate := range graph.RPO() {
		if candidate == to {
			continue
		}
		if !r.PointCanReach(from, candidate) || !r.PointCanReach(candidate, to) {
			continue
		}
		if r.CallMayInvalidateTrackedPath(candidate, target) {
			return true
		}
	}
	return false
}

// CallMayInvalidateGuardFact reports whether the call at point can invalidate a
// guard fact about target. Guard facts are stricter than tracked path reads:
// an open call may mutate captured locals or global state even when target is
// not passed as an explicit argument. Exact empty signatures and exact
// invalidation summaries are treated as authority.
func (r *Result) CallMayInvalidateGuardFact(point cfg.Point, target pathdom.Path) bool {
	if r == nil || target.IsEmpty() {
		return false
	}
	site, hasSite := r.CallSite(point)
	if !hasSite {
		return false
	}
	outcome, hasOutcome := r.CallOutcomeAt(point)
	if !hasOutcome {
		if r.CallSiteHasExactEmptyGuardInvalidationSummary(site) {
			return false
		}
		return !receiverRootGuardFactSurvivesOpenMethodCall(site, target)
	}
	if r.CallOutcomeHasCovariantExposureForTarget(site, outcome, target) {
		return true
	}
	if !r.CallOutcomeHasExactGuardInvalidationSummary(site, outcome, false) {
		return !receiverRootGuardFactSurvivesOpenMethodCall(site, target)
	}
	if CallOutcomeHasGlobalGuardInvalidation(outcome) {
		return true
	}
	invalidated, ok := r.CallOutcomeGuardInvalidationPaths(site, outcome)
	if !ok {
		return true
	}
	for _, invalidation := range invalidated {
		if invalidation.RootRebinding {
			if target.HasPrefix(invalidation.Path) {
				return true
			}
			continue
		}
		if callInvalidationPathClearsGuardFact(invalidation.Path, target) {
			return true
		}
	}
	return false
}

func receiverRootGuardFactSurvivesOpenMethodCall(site factflow.CallSite, target pathdom.Path) bool {
	receiver, ok := site.ReceiverPath()
	return site.MethodName() != "" &&
		ok &&
		!receiver.IsEmpty() &&
		len(receiver.Segments) == 0 &&
		target.Equal(receiver)
}

func callInvalidationPathClearsGuardFact(invalidated, target pathdom.Path) bool {
	if invalidated.IsEmpty() || target.IsEmpty() {
		return false
	}
	if len(invalidated.Segments) == 0 {
		return target.HasStrictPrefix(invalidated)
	}
	return target.HasPrefix(invalidated)
}

// CallOutcomeHasCovariantExposureForTarget reports whether outcome exposes a
// caller argument through a wider mutable view overlapping target. Such an
// exposure invalidates recovered path evidence: even without a direct write
// summary, the caller cannot keep using a dominating narrow declaration or
// guard as proof for the exposed object.
func (r *Result) CallOutcomeHasCovariantExposureForTarget(site factflow.CallSite, outcome callpayload.CallOutcome, target pathdom.Path) bool {
	if r == nil || target.IsEmpty() || len(outcome.ParamExposures) == 0 {
		return false
	}
	bindings := r.callGuardArgumentBindings(site)
	for _, exposure := range outcome.ParamExposures {
		exposed, ok := exposure.Source.Substitute(bindings)
		if !ok || exposed.IsEmpty() {
			continue
		}
		if target.Overlaps(exposed) || exposed.Overlaps(target) {
			return true
		}
	}
	return false
}

// CallSiteHasExactEmptyGuardInvalidationSummary reports whether a call site has
// a declared pure signature with no operational invalidations.
func (r *Result) CallSiteHasExactEmptyGuardInvalidationSummary(site factflow.CallSite) bool {
	if r == nil {
		return false
	}
	sig, ok := r.CallSignature(site)
	return ok && sig.OperationalEffects == nil && sig.Effect.Pure()
}

func (r *Result) callFactReferencesTrackedPath(site factflow.CallSite, fact CallFact, target pathdom.Path) bool {
	if receiver, ok := site.ReceiverPath(); ok && target.Overlaps(receiver) {
		return true
	}
	found := false
	site.ForEachArgumentSource(func(_ int, source factflow.ValueSource) bool {
		argPath, ok := r.valueSourcePath(source)
		if ok && target.Overlaps(argPath) {
			found = true
			return false
		}
		return true
	})
	if found {
		return true
	}
	for _, arg := range fact.Args {
		argPath, ok := r.ExpressionPath(arg)
		if ok && target.Overlaps(argPath) {
			return true
		}
	}
	return false
}

// CallOutcomeHasExactGuardInvalidationSummary reports whether outcome carries
// enough effect authority to decide path invalidation precisely for a referenced
// receiver or argument.
func (r *Result) CallOutcomeHasExactGuardInvalidationSummary(site factflow.CallSite, outcome callpayload.CallOutcome, trustResolvedSummaryAuthority bool) bool {
	sig, hasSignature := r.CallSignature(site)
	hasOperationalEffects := hasSignature &&
		sig.OperationalEffects != nil &&
		!sig.OperationalEffects.IsEmpty()
	hasPureSignature := hasSignature &&
		sig.OperationalEffects == nil &&
		sig.Effect.Pure()
	// A known return value can make PostReturnAuthority true; it is not proof
	// that side effects are complete. Exact empty summary authority is only a
	// no-mutation proof for a tracked argument/receiver path the call actually
	// received, not for captured locals or globals outside the call boundary.
	if trustResolvedSummaryAuthority && !hasSignature && outcome.PostReturnAuthority {
		return true
	}
	if trustResolvedSummaryAuthority && !hasSignature && r.callSiteHasLocalFunctionDefinition(site) {
		return true
	}
	return CallOutcomeHasExplicitGuardInvalidation(outcome) ||
		hasOperationalEffects ||
		hasPureSignature ||
		(hasSignature && sig.Effect.IsClosed())
}

func (r *Result) callSiteHasLocalFunctionDefinition(site factflow.CallSite) bool {
	return r != nil && site.CalleeSymbol() != 0 && r.IsFunctionDefinitionTarget(site.CalleeSymbol())
}

// CallOutcomeHasExplicitGuardInvalidation reports whether outcome explicitly
// describes path or global invalidations.
func CallOutcomeHasExplicitGuardInvalidation(outcome callpayload.CallOutcome) bool {
	return len(outcome.ParamPathInvalidations) != 0 ||
		len(outcome.NormalReturnFacts.PathInvalidations) != 0 ||
		CallOutcomeHasGlobalGuardInvalidation(outcome)
}

// CallOutcomeHasGlobalGuardInvalidation reports whether outcome carries a
// mutation effect that is not scoped to a substituted path.
func CallOutcomeHasGlobalGuardInvalidation(outcome callpayload.CallOutcome) bool {
	for _, delta := range outcome.NormalReturnFacts.EffectDeltas {
		if delta.Kind == effectdelta.Mutation &&
			delta.Target.IsEmpty() &&
			!callboundary.IsPathInvalidationEffectSite(delta.Site) {
			return true
		}
	}
	return false
}

// CallOutcomeGuardInvalidationPaths substitutes the invalidation paths carried
// by outcome onto the caller's argument/receiver paths.
func (r *Result) CallOutcomeGuardInvalidationPaths(site factflow.CallSite, outcome callpayload.CallOutcome) ([]CallPathInvalidation, bool) {
	paramBindings := r.callGuardArgumentBindings(site)
	callBindings := r.callGuardCallBindings(site)
	var out []CallPathInvalidation
	appendSubstituted := func(bindings []pathdom.Path, target pathdom.Path, rootRebinding bool) {
		substituted, ok := target.Substitute(bindings)
		if !ok || substituted.IsEmpty() {
			return
		}
		out = append(out, CallPathInvalidation{Path: substituted, RootRebinding: rootRebinding})
	}
	for _, invalidation := range outcome.ParamPathInvalidations {
		appendSubstituted(paramBindings, invalidation.Path, false)
	}
	for _, invalidation := range outcome.NormalReturnFacts.PathInvalidations {
		appendSubstituted(callBindings, invalidation.Path, concreteRootInvalidation(invalidation.Path))
	}
	for _, delta := range outcome.NormalReturnFacts.EffectDeltas {
		if delta.Kind != effectdelta.Mutation ||
			callboundary.IsPathInvalidationEffectSite(delta.Site) ||
			callboundary.IsPathStructuralPreservingInvalidationEffectSite(delta.Site) {
			continue
		}
		appendSubstituted(callBindings, delta.Target, false)
	}
	return out, true
}

func concreteRootInvalidation(target pathdom.Path) bool {
	return !target.IsPlaceholder() && target.Symbol != 0 && len(target.Segments) == 0
}

func (r *Result) callGuardArgumentBindings(site factflow.CallSite) []pathdom.Path {
	if r == nil {
		return nil
	}
	var bindings []pathdom.Path
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		sourcePath, ok := r.valueSourcePath(source)
		if !ok || sourcePath.IsEmpty() {
			return true
		}
		for len(bindings) <= i {
			bindings = append(bindings, pathdom.Path{})
		}
		bindings[i] = sourcePath
		return true
	})
	return bindings
}

func (r *Result) callGuardCallBindings(site factflow.CallSite) []pathdom.Path {
	if r == nil {
		return nil
	}
	var bindings []pathdom.Path
	offset := 0
	if receiverPath, ok := site.ReceiverPath(); ok {
		bindings = appendPathBinding(bindings, 0, receiverPath)
		offset = 1
	}
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		sourcePath, ok := r.valueSourcePath(source)
		if !ok || sourcePath.IsEmpty() {
			return true
		}
		bindings = appendPathBinding(bindings, i+offset, sourcePath)
		return true
	})
	return bindings
}

func appendPathBinding(bindings []pathdom.Path, index int, value pathdom.Path) []pathdom.Path {
	if index < 0 || value.IsEmpty() {
		return bindings
	}
	for len(bindings) <= index {
		bindings = append(bindings, pathdom.Path{})
	}
	bindings[index] = value
	return bindings
}
