package diagnostics

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valuerefine "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/typenarrow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type literalConstraint struct {
	target  path.Path
	value   typ.Type
	negated bool
	span    diagnostic.Span
}

type runtimeTypeConstraint struct {
	target path.Path
	name   string
	span   diagnostic.Span
}

type guardFactOrigin struct {
	target path.Path
	span   diagnostic.Span
}

type guardEnv struct {
	unreachable    bool
	constraints    []literalConstraint
	typeChecks     []runtimeTypeConstraint
	present        []path.Path
	presentOrigins []guardFactOrigin
	truthy         []path.Path
	truthyOrigins  []guardFactOrigin
	falsy          []path.Path
	falsyOrigins   []guardFactOrigin
	nilPaths       []path.Path
	nilOrigins     []guardFactOrigin
}

// diagnosticGuardCache owns guard-environment fixpoint results for one
// diagnostic production run. It keeps the cache lifetime explicit: diagnostics
// share the expensive guard fixpoint while one run is producing evidence, and
// the whole cache is discarded when the run returns.
type diagnosticGuardCache struct {
	byResult map[*body.Result]map[cfg.Point]guardEnv
	builds   int
}

func newDiagnosticGuardCache() *diagnosticGuardCache {
	return &diagnosticGuardCache{}
}

func (c *diagnosticGuardCache) environments(result *body.Result) map[cfg.Point]guardEnv {
	if c == nil {
		return guardEnvironments(result)
	}
	if c.byResult != nil {
		if envs, ok := c.byResult[result]; ok {
			return envs
		}
	}
	envs := guardEnvironments(result)
	if c.byResult == nil {
		c.byResult = make(map[*body.Result]map[cfg.Point]guardEnv, 1)
	}
	c.byResult[result] = envs
	c.builds++
	return envs
}

func guardEnvReachableAt(envs map[cfg.Point]guardEnv, point cfg.Point) bool {
	env, ok := envs[point]
	return !ok || !env.unreachable
}

func applyExpressionEdgeGuards(result *body.Result, point cfg.Point, expr ast.Expr, cond bool, env guardEnv) (guardEnv, bool) {
	if result == nil || expr == nil {
		return env, true
	}
	implied := result.ExpressionImpliedChecksOnEdge(expr, cond)
	if len(implied) == 0 {
		return env, true
	}
	span := ast.SpanOf(expr)
	for _, check := range implied {
		if check.Check.Kind == branchcond.CheckNone {
			continue
		}
		if branchGuardContradictsKnownValue(result, point, check.Polarity, check.Check) {
			return guardEnv{}, false
		}
		env = applyBranchGuard(env, check.Check, check.Polarity, span)
	}
	return env, true
}

func guardEnvironments(result *body.Result) map[cfg.Point]guardEnv {
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	in := make(map[cfg.Point]guardEnv)
	out := make(map[cfg.Point]guardEnv)
	for _, point := range graph.RPO() {
		in[point] = guardEnv{unreachable: true}
		out[point] = guardEnv{unreachable: true}
	}
	changed := true
	for changed {
		changed = false
		for _, point := range graph.RPO() {
			nextIn := joinPredecessorGuardEnvs(result, graph, point, out)
			nextOut := applyGuardNode(result, point, nextIn)
			if !canonicalGuardEnvEqual(in[point], nextIn) {
				in[point] = nextIn
				changed = true
			}
			if !canonicalGuardEnvEqual(out[point], nextOut) {
				out[point] = nextOut
				changed = true
			}
		}
	}
	return in
}

func joinPredecessorGuardEnvs(result *body.Result, graph cfg.Graph, point cfg.Point, out map[cfg.Point]guardEnv) guardEnv {
	preds := cfg.PredecessorsReadOnly(graph, point)
	if len(preds) == 0 {
		return guardEnv{}
	}
	var env guardEnv
	for i, pred := range preds {
		edgeEnv := applyGuardEdge(result, graph, pred, point, out[pred])
		if i == 0 {
			env = edgeEnv
			continue
		}
		env = joinGuardEnvs(env, edgeEnv)
	}
	return env
}

func applyGuardEdge(result *body.Result, graph cfg.Graph, from, to cfg.Point, env guardEnv) guardEnv {
	cond, ok := graph.EdgeCond(from, to)
	if !ok {
		return env
	}
	if result == nil {
		return env
	}
	fact, ok := result.BranchCondition(from)
	if !ok {
		return env
	}
	span := ast.SpanOf(fact.Condition)
	if !span.Valid() {
		span = ast.SpanOf(fact.Stmt)
	}
	if branchGuardContradictsKnownValue(result, from, cond, fact.Check) {
		return guardEnv{unreachable: true}
	}
	checks := result.BranchConditionChecksOnEdge(fact, cond)
	if len(checks) == 0 {
		checks = []branchcond.Check{fact.Check}
	}
	for _, check := range checks {
		if check.Kind == branchcond.CheckNone {
			continue
		}
		if branchGuardContradictsKnownValue(result, from, cond, check) {
			return guardEnv{unreachable: true}
		}
		env = applyBranchGuard(env, check, cond, span)
	}
	return env
}

func branchGuardContradictsKnownValue(result *body.Result, point cfg.Point, cond bool, check branchcond.Check) bool {
	if result == nil || check.Path.IsEmpty() {
		return false
	}
	reg := result.Registry()
	if reg == nil {
		return false
	}
	if branchPathComparisonContradictsKnownOrigin(result, point, cond, check) {
		return true
	}
	value, ok := newDiagnosticQuery(result).PathValueBeforeBoundary(point, check.Path)
	if !ok {
		return false
	}
	if newDiagnosticQuery(result).ValueHasUntrustedTopOrigin(value) {
		return false
	}
	if branchGuardValueContradicts(reg, value, cond, check) {
		return true
	}
	if branchGuardRequiresTruthy(check, cond) {
		if !valuerefine.CanBeTruthy(reg, value) && indexedDescendantProjectionCanSatisfyGuard(result, point, check.Path, true) {
			return false
		}
		return !valuerefine.CanBeTruthy(reg, value)
	}
	if branchGuardRequiresFalsy(check, cond) {
		if !valuerefine.CanBeFalsy(reg, value) && indexedDescendantProjectionCanSatisfyGuard(result, point, check.Path, false) {
			return false
		}
		return !valuerefine.CanBeFalsy(reg, value)
	}
	return false
}

func branchGuardValueContradicts(reg *axis.Registry, value product.Value, cond bool, check branchcond.Check) bool {
	switch check.Kind {
	case branchcond.CheckLiteralEqual:
		lit, ok := check.LiteralValue()
		if !ok {
			return false
		}
		if cond {
			return valueContradictsConstraint(reg, value, typevalue.WithWitness(reg, typevalue.FromType(reg, lit), lit))
		}
		return valueDefinitelyLiteral(reg, value, lit)
	case branchcond.CheckLiteralNot:
		lit, ok := check.LiteralValue()
		if !ok {
			return false
		}
		if !cond {
			return valueContradictsConstraint(reg, value, typevalue.WithWitness(reg, typevalue.FromType(reg, lit), lit))
		}
		return valueDefinitelyLiteral(reg, value, lit)
	case branchcond.CheckTypeEqual, branchcond.CheckTypeNot:
		if check.TypeName == "" {
			return false
		}
		tag, ok := runtimekind.ParseTag(check.TypeName)
		if !ok {
			return false
		}
		match := check.Kind == branchcond.CheckTypeEqual
		if !cond {
			match = !match
		}
		refinement := typenarrow.UnmatchRefinement(reg, tag)
		if match {
			refinement = typenarrow.MatchRefinement(reg, tag)
		}
		constraint, ok := refinement.Constraint()
		if !ok {
			return false
		}
		return valueContradictsConstraint(reg, value, constraint)
	default:
		return false
	}
}

func valueContradictsConstraint(reg *axis.Registry, value, constraint product.Value) bool {
	if reg == nil {
		return false
	}
	refined := valuerefine.MeetConstraint(reg, value, constraint)
	return product.Equal(reg, refined, product.Bottom(reg)) || presence.Equal(product.PresenceOf(refined), presence.Bottom())
}

func valueDefinitelyLiteral(reg *axis.Registry, value product.Value, lit typ.Type) bool {
	if reg == nil || lit == nil {
		return false
	}
	t, ok := typevalue.TypeOf(reg, value)
	return ok && t != nil && subtype.IsSubtype(t, lit)
}

func indexedDescendantProjectionCanSatisfyGuard(result *body.Result, point cfg.Point, p path.Path, truthy bool) bool {
	if result == nil || p.Symbol == 0 || !pathHasIndexSegment(p) {
		return false
	}
	reg := result.Registry()
	if reg == nil {
		return false
	}
	rootValue, ok := newDiagnosticQuery(result).PathValueBeforeBoundary(point, p.RootOnly())
	if !ok || newDiagnosticQuery(result).ValueHasUntrustedTopOrigin(rootValue) {
		return false
	}
	rootType, ok := typevalue.TypeOf(reg, rootValue)
	if !ok || rootType == nil {
		return false
	}
	projectedType, ok := luatypeprojection.ApplySegments(rootType, p.Segments)
	if !ok || projectedType == nil {
		return false
	}
	projectedValue := typevalue.WithWitness(reg, typevalue.FromType(reg, projectedType), projectedType)
	if truthy {
		return valuerefine.CanBeTruthy(reg, projectedValue)
	}
	return valuerefine.CanBeFalsy(reg, projectedValue)
}

func pathHasIndexSegment(p path.Path) bool {
	for _, seg := range p.Segments {
		if seg.Kind == segment.SegmentIndexString || seg.Kind == segment.SegmentIndexInt {
			return true
		}
	}
	return false
}

func branchGuardRequiresTruthy(check branchcond.Check, cond bool) bool {
	return (check.Kind == branchcond.CheckTruthy && cond) ||
		(check.Kind == branchcond.CheckFalsy && !cond)
}

func branchGuardRequiresFalsy(check branchcond.Check, cond bool) bool {
	return (check.Kind == branchcond.CheckTruthy && !cond) ||
		(check.Kind == branchcond.CheckFalsy && cond)
}

func branchPathComparisonContradictsKnownOrigin(result *body.Result, point cfg.Point, cond bool, check branchcond.Check) bool {
	equal, ok := branchPathComparisonRequiresEquality(check, cond)
	if !ok {
		return false
	}
	return branchPathOriginComparisonContradicts(result, point, check.Path, check.OtherPath, equal) ||
		branchPathOriginComparisonContradicts(result, point, check.OtherPath, check.Path, equal)
}

func branchPathComparisonRequiresEquality(check branchcond.Check, cond bool) (bool, bool) {
	switch check.Kind {
	case branchcond.CheckPathEqual:
		return cond, true
	case branchcond.CheckPathNot:
		return !cond, true
	default:
		return false, false
	}
}

func branchPathOriginComparisonContradicts(result *body.Result, point cfg.Point, parentPath, constraintPath path.Path, equal bool) bool {
	if result == nil || parentPath.IsEmpty() || constraintPath.IsEmpty() || len(parentPath.Segments) == 0 {
		return false
	}
	reg := result.Registry()
	if reg == nil {
		return false
	}
	query := newDiagnosticQuery(result)
	rootValue, ok := query.PathValueBeforeBoundary(point, parentPath.RootOnly())
	if !ok || query.ValueHasUntrustedTopOrigin(rootValue) {
		return false
	}
	constraintValue, ok := query.PathValueBeforeBoundary(point, constraintPath)
	if !ok || query.ValueHasUntrustedTopOrigin(constraintValue) {
		return false
	}
	rootOrigin, ok := typevalue.VariantOriginOfValue(reg, nil, rootValue)
	if !ok {
		return false
	}
	constraintOrigin, ok := typevalue.VariantOriginOfValue(reg, nil, constraintValue)
	if !ok {
		return false
	}
	cases, ok := variant.NarrowOriginByPath(
		rootOrigin.Family(),
		rootOrigin.CasesRef(),
		parentPath.Segments,
		constraintOrigin.Family(),
		constraintOrigin.CasesRef(),
		equal,
	)
	return ok && len(cases) == 0
}

func applyGuardNode(result *body.Result, point cfg.Point, env guardEnv) guardEnv {
	if result == nil {
		return env
	}
	if env.unreachable {
		return env
	}
	if _, ok := result.Call(point); ok {
		env = applyCallGuardInvalidation(result, point, env)
	}
	if fact, ok := result.LocalAssignment(point); ok && fact.HasSymbol {
		return env.withoutFactsForPath(path.NewPath(fact.Symbol, fact.Name))
	}
	fact, ok := result.OrdinaryAssignment(point)
	if !ok {
		return env
	}
	if directDynamicIndexAssignment(fact) {
		return env.withoutDescendantFacts()
	}
	if fact.HasPath && !fact.Path.IsEmpty() {
		if len(fact.Path.Segments) > 0 {
			return env.withoutFactsForPathAssignment(fact.Path)
		}
		return env.withoutFactsForPath(fact.Path)
	}
	return env
}

func directDynamicIndexAssignment(fact semantics.OrdinaryAssignmentFact) bool {
	if !fact.HasContainerPath {
		return false
	}
	attr, ok := fact.Target.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		return false
	}
	switch attr.Key.(type) {
	case *ast.StringExpr, *ast.NumberExpr:
		return false
	default:
		return true
	}
}

func applyCallGuardInvalidation(result *body.Result, point cfg.Point, env guardEnv) guardEnv {
	if result == nil || env.unreachable {
		return env
	}
	site, hasSite := result.CallSite(point)
	if !hasSite {
		return guardEnv{}
	}
	outcome, hasOutcome := result.CallOutcomeAt(point)
	if !hasOutcome {
		if callSiteHasExactEmptyGuardInvalidationSummary(result, site) {
			return env
		}
		return env.receiverRootFactsStableAcrossOpenMethodCall(site)
	}
	if !callOutcomeHasExactGuardInvalidationSummary(result, site, outcome, false) {
		return env.receiverRootFactsStableAcrossOpenMethodCall(site)
	}
	if callOutcomeHasGlobalGuardInvalidation(outcome) {
		return guardEnv{}
	}
	invalidated, ok := callOutcomeGuardInvalidationPaths(result, site, outcome)
	if !ok {
		return guardEnv{}
	}
	if len(invalidated) == 0 {
		return env
	}
	for _, invalidation := range invalidated {
		if invalidation.rootRebinding {
			env = env.withoutFactsForPath(invalidation.path)
			continue
		}
		env = env.withoutFactsForCallInvalidation(invalidation.path)
	}
	return env
}

func callSiteHasExactEmptyGuardInvalidationSummary(result *body.Result, site factflow.CallSite) bool {
	if result == nil {
		return false
	}
	sig, ok := result.CallSignature(site)
	return ok && sig.OperationalEffects == nil && sig.Effect.Pure()
}

func callMayInvalidateTrackedPath(result *body.Result, point cfg.Point, target path.Path) bool {
	if result == nil || target.IsEmpty() {
		return false
	}
	site, hasSite := result.CallSite(point)
	if !hasSite {
		return false
	}
	view, ok := result.CallView(point)
	if !ok {
		return false
	}
	fact, _ := view.Borrowed()
	if !callFactReferencesTrackedPath(result, site, fact, target) {
		return false
	}
	outcome, hasOutcome := result.CallOutcomeAt(point)
	if !hasOutcome {
		if callSiteHasExactEmptyGuardInvalidationSummary(result, site) {
			return false
		}
		return true
	}
	if !callOutcomeHasExactGuardInvalidationSummary(result, site, outcome, true) {
		return true
	}
	if callOutcomeHasGlobalGuardInvalidation(outcome) {
		return true
	}
	invalidated, ok := callOutcomeGuardInvalidationPaths(result, site, outcome)
	if !ok {
		return true
	}
	for _, invalidation := range invalidated {
		if target.Overlaps(invalidation.path) {
			return true
		}
	}
	return false
}

func callFactReferencesTrackedPath(result *body.Result, site factflow.CallSite, fact semantics.CallFact, target path.Path) bool {
	if receiver, ok := site.ReceiverPath(); ok && target.Overlaps(receiver) {
		return true
	}
	for _, arg := range fact.Args {
		argPath, ok := result.ExpressionPath(arg)
		if ok && target.Overlaps(argPath) {
			return true
		}
	}
	return false
}

func callOutcomeHasExactGuardInvalidationSummary(result *body.Result, site factflow.CallSite, outcome callpayload.CallOutcome, trustResolvedSummaryAuthority bool) bool {
	sig, hasSignature := result.CallSignature(site)
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
	if trustResolvedSummaryAuthority && !hasSignature && callSiteHasLocalFunctionDefinition(result, site) {
		return true
	}
	return callOutcomeHasExplicitGuardInvalidation(outcome) ||
		hasOperationalEffects ||
		hasPureSignature ||
		(hasSignature && sig.Effect.IsClosed())
}

func callSiteHasLocalFunctionDefinition(result *body.Result, site factflow.CallSite) bool {
	return result != nil && site.CalleeSymbol() != 0 && result.IsFunctionDefinitionTarget(site.CalleeSymbol())
}

func callOutcomeHasExplicitGuardInvalidation(outcome callpayload.CallOutcome) bool {
	return len(outcome.ParamPathInvalidations) != 0 ||
		len(outcome.NormalReturnFacts.PathInvalidations) != 0 ||
		callOutcomeHasGlobalGuardInvalidation(outcome)
}

func callOutcomeHasGlobalGuardInvalidation(outcome callpayload.CallOutcome) bool {
	for _, delta := range outcome.NormalReturnFacts.EffectDeltas {
		if delta.Kind == effectdelta.Mutation && !callboundary.IsPathInvalidationEffectSite(delta.Site) {
			return true
		}
	}
	return false
}

type callGuardInvalidation struct {
	path          path.Path
	rootRebinding bool
}

func callOutcomeGuardInvalidationPaths(result *body.Result, site factflow.CallSite, outcome callpayload.CallOutcome) ([]callGuardInvalidation, bool) {
	paramBindings := callGuardArgumentBindings(result, site)
	callBindings := callGuardCallBindings(result, site)
	var out []callGuardInvalidation
	appendSubstituted := func(bindings []path.Path, target path.Path, rootRebinding bool) {
		substituted, ok := target.Substitute(bindings)
		if !ok || substituted.IsEmpty() {
			return
		}
		out = append(out, callGuardInvalidation{path: substituted, rootRebinding: rootRebinding})
	}
	for _, invalidation := range outcome.ParamPathInvalidations {
		appendSubstituted(paramBindings, invalidation.Path, false)
	}
	for _, invalidation := range outcome.NormalReturnFacts.PathInvalidations {
		appendSubstituted(callBindings, invalidation.Path, concreteRootInvalidation(invalidation.Path))
	}
	return out, true
}

func concreteRootInvalidation(target path.Path) bool {
	return !target.IsPlaceholder() && target.Symbol != 0 && len(target.Segments) == 0
}

func callGuardArgumentBindings(result *body.Result, site factflow.CallSite) []path.Path {
	if result == nil {
		return nil
	}
	var bindings []path.Path
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			return true
		}
		sourcePath, ok := result.ExpressionPathRef(source.ExprRef)
		if !ok || sourcePath.IsEmpty() {
			return true
		}
		for len(bindings) <= i {
			bindings = append(bindings, path.Path{})
		}
		bindings[i] = sourcePath
		return true
	})
	return bindings
}

func callGuardCallBindings(result *body.Result, site factflow.CallSite) []path.Path {
	if result == nil {
		return nil
	}
	var bindings []path.Path
	offset := 0
	if receiverPath, ok := site.ReceiverPath(); ok {
		bindings = appendPathBinding(bindings, 0, receiverPath)
		offset = 1
	}
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			return true
		}
		sourcePath, ok := result.ExpressionPathRef(source.ExprRef)
		if !ok || sourcePath.IsEmpty() {
			return true
		}
		bindings = appendPathBinding(bindings, i+offset, sourcePath)
		return true
	})
	return bindings
}

func appendPathBinding(bindings []path.Path, index int, value path.Path) []path.Path {
	if index < 0 || value.IsEmpty() {
		return bindings
	}
	for len(bindings) <= index {
		bindings = append(bindings, path.Path{})
	}
	bindings[index] = value
	return bindings
}

func applyBranchGuard(env guardEnv, check branchcond.Check, cond bool, span diagnostic.Span) guardEnv {
	if env.unreachable {
		return env
	}
	if proof, ok := redundantConditionProofFor(env, check); ok {
		if proof.always != cond {
			return guardEnv{unreachable: true}
		}
		return env
	}
	if check.Kind == branchcond.CheckTruthy && cond {
		return env.withTruthyAt(check.Path, span)
	}
	if check.Kind == branchcond.CheckFalsy && !cond {
		return env.withTruthyAt(check.Path, span)
	}
	if check.Kind == branchcond.CheckTruthy && !cond {
		return env.withFalsyAt(check.Path, span)
	}
	if check.Kind == branchcond.CheckFalsy && cond {
		return env.withFalsyAt(check.Path, span)
	}
	if check.Kind == branchcond.CheckNil && cond {
		return env.withNilAt(check.Path, span)
	}
	if check.Kind == branchcond.CheckNil && !cond {
		return env.withPresentAt(check.Path, span)
	}
	if check.Kind == branchcond.CheckNotNil && cond {
		return env.withPresentAt(check.Path, span)
	}
	if check.Kind == branchcond.CheckNotNil && !cond {
		return env.withNilAt(check.Path, span)
	}
	if check.Kind == branchcond.CheckLiteralEqual && cond {
		return env.withLiteralCheck(check, false, span).withTruthyAt(check.Path, span)
	}
	if check.Kind == branchcond.CheckLiteralEqual && !cond {
		return env.withLiteralCheck(check, true, span)
	}
	if check.Kind == branchcond.CheckLiteralNot && cond {
		return env.withLiteralCheck(check, true, span)
	}
	if check.Kind == branchcond.CheckLiteralNot && !cond {
		return env.withLiteralCheck(check, false, span).withTruthyAt(check.Path, span)
	}
	if (check.Kind == branchcond.CheckTypeEqual || check.Kind == branchcond.CheckTypeNot) && check.TypeName == "" {
		// type(path) == otherPath with no statically-known type name: the value
		// fixpoint applies any narrowing; the diagnostic guard adds no constraint.
		return env
	}
	if check.Kind == branchcond.CheckTypeEqual && cond {
		return env.withType(runtimeTypeConstraint{target: check.Path, name: check.TypeName, span: span}).withRuntimeTypePresenceAt(check.Path, check.TypeName, span)
	}
	if check.Kind == branchcond.CheckTypeEqual && !cond && check.TypeName == "nil" {
		return env.withPresentAt(check.Path, span)
	}
	if check.Kind == branchcond.CheckTypeNot && cond && check.TypeName == "nil" {
		return env.withPresentAt(check.Path, span)
	}
	if check.Kind == branchcond.CheckTypeNot && !cond {
		return env.withType(runtimeTypeConstraint{target: check.Path, name: check.TypeName, span: span}).withRuntimeTypePresenceAt(check.Path, check.TypeName, span)
	}
	return env
}

func (e guardEnv) withLiteralCheck(check branchcond.Check, negated bool, span diagnostic.Span) guardEnv {
	lit, ok := check.LiteralValue()
	if !ok {
		return e
	}
	return e.with(literalConstraint{target: check.Path, value: lit, negated: negated, span: span})
}

func (e guardEnv) with(c literalConstraint) guardEnv {
	if c.target.IsEmpty() || c.value == nil || e.unreachable {
		return e
	}
	out := e.cloneGuard()
	out.constraints = upsertByTarget(out.constraints, c, func(x literalConstraint) path.Path { return x.target })
	sortGuardEnv(out)
	return out
}

func (e guardEnv) withType(c runtimeTypeConstraint) guardEnv {
	if c.target.IsEmpty() || c.name == "" || e.unreachable {
		return e
	}
	out := e.cloneGuard()
	out.typeChecks = upsertByTarget(out.typeChecks, c, func(x runtimeTypeConstraint) path.Path { return x.target })
	sortGuardEnv(out)
	return out
}

// cloneGuard returns a deep copy of e with every slice field duplicated and
// nothing else changed.
func (e guardEnv) cloneGuard() guardEnv {
	return guardEnv{
		unreachable:    e.unreachable,
		constraints:    append([]literalConstraint(nil), e.constraints...),
		typeChecks:     append([]runtimeTypeConstraint(nil), e.typeChecks...),
		present:        copyPaths(e.present),
		presentOrigins: copyGuardFactOrigins(e.presentOrigins),
		truthy:         copyPaths(e.truthy),
		truthyOrigins:  copyGuardFactOrigins(e.truthyOrigins),
		falsy:          copyPaths(e.falsy),
		falsyOrigins:   copyGuardFactOrigins(e.falsyOrigins),
		nilPaths:       copyPaths(e.nilPaths),
		nilOrigins:     copyGuardFactOrigins(e.nilOrigins),
	}
}

// upsertByTarget replaces the entry whose target equals c's target, or appends c
// when none matches.
func upsertByTarget[T any](in []T, c T, targetOf func(T) path.Path) []T {
	want := targetOf(c)
	for i, existing := range in {
		if targetOf(existing).Equal(want) {
			in[i] = c
			return in
		}
	}
	return append(in, c)
}

func (e guardEnv) withPresentAt(target path.Path, span diagnostic.Span) guardEnv {
	if target.IsEmpty() || e.unreachable {
		return e
	}
	out := e.cloneGuard()
	out.present, out.presentOrigins = appendPathFact(out.present, out.presentOrigins, target, span)
	out.nilPaths, out.nilOrigins = removePathFact(out.nilPaths, out.nilOrigins, target)
	sortGuardEnv(out)
	return out
}

func (e guardEnv) withTruthyAt(target path.Path, span diagnostic.Span) guardEnv {
	if target.IsEmpty() || e.unreachable {
		return e
	}
	out := e.cloneGuard()
	out.present, out.presentOrigins = appendPathFact(out.present, out.presentOrigins, target, span)
	out.truthy, out.truthyOrigins = appendPathFact(out.truthy, out.truthyOrigins, target, span)
	out.falsy, out.falsyOrigins = removePathFact(out.falsy, out.falsyOrigins, target)
	out.nilPaths, out.nilOrigins = removePathFact(out.nilPaths, out.nilOrigins, target)
	sortGuardEnv(out)
	return out
}

func (e guardEnv) withFalsyAt(target path.Path, span diagnostic.Span) guardEnv {
	if target.IsEmpty() || e.unreachable {
		return e
	}
	out := e.cloneGuard()
	out.truthy, out.truthyOrigins = removePathFact(out.truthy, out.truthyOrigins, target)
	out.falsy, out.falsyOrigins = appendPathFact(out.falsy, out.falsyOrigins, target, span)
	sortGuardEnv(out)
	return out
}

func (e guardEnv) withNilAt(target path.Path, span diagnostic.Span) guardEnv {
	if target.IsEmpty() || e.unreachable {
		return e
	}
	out := e.cloneGuard()
	out.present, out.presentOrigins = removePathFact(out.present, out.presentOrigins, target)
	out.truthy, out.truthyOrigins = removePathFact(out.truthy, out.truthyOrigins, target)
	out.falsy, out.falsyOrigins = appendPathFact(out.falsy, out.falsyOrigins, target, span)
	out.nilPaths, out.nilOrigins = appendPathFact(out.nilPaths, out.nilOrigins, target, span)
	sortGuardEnv(out)
	return out
}

func (e guardEnv) withRuntimeTypePresenceAt(target path.Path, typeName string, span diagnostic.Span) guardEnv {
	if typeName == "nil" {
		return e.withNilAt(target, span)
	}
	if typeName != "boolean" {
		return e.withTruthyAt(target, span)
	}
	return e.withPresentAt(target, span)
}

func (e guardEnv) hasPresent(target path.Path) bool {
	return hasPathFact(e.present, target)
}

func (e guardEnv) hasTruthy(target path.Path) bool {
	return hasPathFact(e.truthy, target)
}

func (e guardEnv) hasFalsy(target path.Path) bool {
	return hasPathFact(e.falsy, target)
}

func (e guardEnv) hasNil(target path.Path) bool {
	return hasPathFact(e.nilPaths, target)
}

func (e guardEnv) presentOrigin(target path.Path) diagnostic.Span {
	return guardFactOriginSpan(e.presentOrigins, target)
}

func (e guardEnv) truthyOrigin(target path.Path) diagnostic.Span {
	return guardFactOriginSpan(e.truthyOrigins, target)
}

func (e guardEnv) falsyOrigin(target path.Path) diagnostic.Span {
	return guardFactOriginSpan(e.falsyOrigins, target)
}

func (e guardEnv) nilOrigin(target path.Path) diagnostic.Span {
	return guardFactOriginSpan(e.nilOrigins, target)
}

func (e guardEnv) presentOrTruthyOrigin(target path.Path) diagnostic.Span {
	if span := e.presentOrigin(target); span.Valid() {
		return span
	}
	return e.truthyOrigin(target)
}

func (e guardEnv) withoutDescendantFacts() guardEnv {
	return e.filterFacts(rootOnlyPath)
}

func (e guardEnv) withoutDescendantFactsOf(target path.Path) guardEnv {
	if target.IsEmpty() {
		return e.withoutDescendantFacts()
	}
	if e.unreachable {
		return e
	}
	return e.filterFacts(func(candidate path.Path) bool {
		return !candidate.HasStrictPrefix(target)
	})
}

func (e guardEnv) withoutFactsForCallInvalidation(target path.Path) guardEnv {
	if len(target.Segments) == 0 {
		return e.withoutDescendantFactsOf(target)
	}
	return e.withoutFactsForPath(target)
}

func (e guardEnv) receiverRootFactsStableAcrossOpenMethodCall(site factflow.CallSite) guardEnv {
	if e.unreachable {
		return e
	}
	receiver, ok := site.ReceiverPath()
	if site.MethodName() == "" || !ok || receiver.IsEmpty() || len(receiver.Segments) != 0 {
		return guardEnv{}
	}
	// An unresolved method can mutate through the receiver object, so member
	// facts are not stable. The caller's local root binding itself is stable
	// unless an exact call summary says otherwise; exact summaries are handled
	// before this fallback.
	return e.onlyFactsForExactPath(receiver)
}

func (e guardEnv) withoutFactsForPathAssignment(target path.Path) guardEnv {
	if target.IsEmpty() {
		return e.withoutDescendantFacts()
	}
	if len(target.Segments) == 0 {
		return e.withoutFactsForPath(target)
	}
	if e.unreachable {
		return e
	}
	// Same-root siblings are stable; descendant facts under other roots may alias the mutated table.
	return e.filterFacts(func(candidate path.Path) bool {
		if len(candidate.Segments) == 0 {
			return true
		}
		if !candidate.SameRoot(target) {
			return false
		}
		return !candidate.HasPrefix(target)
	})
}

func (e guardEnv) withoutFactsForPath(target path.Path) guardEnv {
	if target.IsEmpty() {
		return e
	}
	if e.unreachable {
		return e
	}
	return e.filterFacts(func(candidate path.Path) bool {
		return !candidate.HasPrefix(target)
	})
}

func (e guardEnv) onlyFactsForExactPath(target path.Path) guardEnv {
	if target.IsEmpty() {
		return guardEnv{}
	}
	return e.filterFacts(func(candidate path.Path) bool {
		return candidate.Equal(target)
	})
}

func (e guardEnv) filterFacts(keep func(path.Path) bool) guardEnv {
	if e.unreachable {
		return e
	}
	var out guardEnv
	for _, c := range e.constraints {
		if keep(c.target) {
			out.constraints = append(out.constraints, c)
		}
	}
	for _, c := range e.typeChecks {
		if keep(c.target) {
			out.typeChecks = append(out.typeChecks, c)
		}
	}
	for _, p := range e.present {
		if keep(p) {
			out.present = append(out.present, p)
		}
	}
	out.presentOrigins = filterGuardFactOrigins(e.presentOrigins, keep)
	for _, p := range e.truthy {
		if keep(p) {
			out.truthy = append(out.truthy, p)
		}
	}
	out.truthyOrigins = filterGuardFactOrigins(e.truthyOrigins, keep)
	for _, p := range e.falsy {
		if keep(p) {
			out.falsy = append(out.falsy, p)
		}
	}
	out.falsyOrigins = filterGuardFactOrigins(e.falsyOrigins, keep)
	for _, p := range e.nilPaths {
		if keep(p) {
			out.nilPaths = append(out.nilPaths, p)
		}
	}
	out.nilOrigins = filterGuardFactOrigins(e.nilOrigins, keep)
	sortGuardEnv(out)
	return out
}

func rootOnlyPath(p path.Path) bool {
	return !p.IsEmpty() && len(p.Segments) == 0
}

func joinGuardEnvs(a, b guardEnv) guardEnv {
	if a.unreachable {
		return b
	}
	if b.unreachable {
		return a
	}
	var out guardEnv
	for _, left := range a.constraints {
		for _, right := range b.constraints {
			if typ.TypeEquals(left.value, right.value) && left.negated == right.negated && left.target.Equal(right.target) {
				joined := left
				if !spanEqual(left.span, right.span) {
					joined.span = diagnostic.Span{}
				}
				out.constraints = append(out.constraints, joined)
				break
			}
		}
	}
	for _, left := range a.typeChecks {
		for _, right := range b.typeChecks {
			if left.name == right.name && left.target.Equal(right.target) {
				joined := left
				if !spanEqual(left.span, right.span) {
					joined.span = diagnostic.Span{}
				}
				out.typeChecks = append(out.typeChecks, joined)
				break
			}
		}
	}
	out.present, out.presentOrigins = joinPathFacts(a.present, b.present, a.presentOrigins, b.presentOrigins)
	out.truthy, out.truthyOrigins = joinPathFacts(a.truthy, b.truthy, a.truthyOrigins, b.truthyOrigins)
	out.falsy, out.falsyOrigins = joinPathFacts(a.falsy, b.falsy, a.falsyOrigins, b.falsyOrigins)
	out.nilPaths, out.nilOrigins = joinPathFacts(a.nilPaths, b.nilPaths, a.nilOrigins, b.nilOrigins)
	sortGuardEnv(out)
	return out
}

func guardEnvEqual(a, b guardEnv) bool {
	sortGuardEnv(a)
	sortGuardEnv(b)
	return canonicalGuardEnvEqual(a, b)
}

// canonicalGuardEnvEqual compares environments already produced through the
// guardEnv constructors. Those constructors sort every lane at mutation/join
// time, so the fixpoint can compare without repeatedly rebuilding sort keys.
func canonicalGuardEnvEqual(a, b guardEnv) bool {
	if a.unreachable != b.unreachable {
		return false
	}
	if a.unreachable {
		return true
	}
	if len(a.constraints) != len(b.constraints) || len(a.typeChecks) != len(b.typeChecks) ||
		len(a.present) != len(b.present) || len(a.truthy) != len(b.truthy) ||
		len(a.falsy) != len(b.falsy) || len(a.nilPaths) != len(b.nilPaths) ||
		len(a.presentOrigins) != len(b.presentOrigins) || len(a.truthyOrigins) != len(b.truthyOrigins) ||
		len(a.falsyOrigins) != len(b.falsyOrigins) || len(a.nilOrigins) != len(b.nilOrigins) {
		return false
	}
	for i := range a.constraints {
		if !typ.TypeEquals(a.constraints[i].value, b.constraints[i].value) || a.constraints[i].negated != b.constraints[i].negated ||
			!a.constraints[i].target.Equal(b.constraints[i].target) || !spanEqual(a.constraints[i].span, b.constraints[i].span) {
			return false
		}
	}
	for i := range a.typeChecks {
		if a.typeChecks[i].name != b.typeChecks[i].name || !a.typeChecks[i].target.Equal(b.typeChecks[i].target) ||
			!spanEqual(a.typeChecks[i].span, b.typeChecks[i].span) {
			return false
		}
	}
	for i := range a.present {
		if !a.present[i].Equal(b.present[i]) {
			return false
		}
	}
	for i := range a.truthy {
		if !a.truthy[i].Equal(b.truthy[i]) {
			return false
		}
	}
	for i := range a.falsy {
		if !a.falsy[i].Equal(b.falsy[i]) {
			return false
		}
	}
	for i := range a.nilPaths {
		if !a.nilPaths[i].Equal(b.nilPaths[i]) {
			return false
		}
	}
	return guardFactOriginsEqual(a.presentOrigins, b.presentOrigins) &&
		guardFactOriginsEqual(a.truthyOrigins, b.truthyOrigins) &&
		guardFactOriginsEqual(a.falsyOrigins, b.falsyOrigins) &&
		guardFactOriginsEqual(a.nilOrigins, b.nilOrigins)
}

func sortGuardEnv(e guardEnv) {
	sort.Slice(e.constraints, func(i, j int) bool {
		left := e.constraints[i]
		right := e.constraints[j]
		if left.target.Root != right.target.Root {
			return left.target.Root < right.target.Root
		}
		if left.target.Symbol != right.target.Symbol {
			return left.target.Symbol < right.target.Symbol
		}
		leftSuffix := segment.FormatSegments(left.target.Segments)
		rightSuffix := segment.FormatSegments(right.target.Segments)
		if leftSuffix != rightSuffix {
			return leftSuffix < rightSuffix
		}
		if left.value.Hash() != right.value.Hash() {
			return left.value.Hash() < right.value.Hash()
		}
		if left.value.String() != right.value.String() {
			return left.value.String() < right.value.String()
		}
		return !left.negated && right.negated
	})
	sort.Slice(e.typeChecks, func(i, j int) bool {
		left := e.typeChecks[i]
		right := e.typeChecks[j]
		if left.target.Root != right.target.Root {
			return left.target.Root < right.target.Root
		}
		if left.target.Symbol != right.target.Symbol {
			return left.target.Symbol < right.target.Symbol
		}
		leftSuffix := segment.FormatSegments(left.target.Segments)
		rightSuffix := segment.FormatSegments(right.target.Segments)
		if leftSuffix != rightSuffix {
			return leftSuffix < rightSuffix
		}
		return left.name < right.name
	})
	sort.Slice(e.present, func(i, j int) bool {
		return pathLess(e.present[i], e.present[j])
	})
	sortGuardFactOrigins(e.presentOrigins)
	sort.Slice(e.truthy, func(i, j int) bool {
		return pathLess(e.truthy[i], e.truthy[j])
	})
	sortGuardFactOrigins(e.truthyOrigins)
	sort.Slice(e.falsy, func(i, j int) bool {
		return pathLess(e.falsy[i], e.falsy[j])
	})
	sortGuardFactOrigins(e.falsyOrigins)
	sort.Slice(e.nilPaths, func(i, j int) bool {
		return pathLess(e.nilPaths[i], e.nilPaths[j])
	})
	sortGuardFactOrigins(e.nilOrigins)
}

func pathLess(left, right path.Path) bool {
	return left.Less(right)
}

// Paths stored in a guardEnv are immutable values (derivations always allocate
// fresh segment arrays), so these helpers share the path values and only
// duplicate the slice, which sortGuardEnv then reorders in place.
func appendPathFact(in []path.Path, origins []guardFactOrigin, target path.Path, span diagnostic.Span) ([]path.Path, []guardFactOrigin) {
	out := copyPaths(in)
	for _, existing := range out {
		if existing.Equal(target) {
			return out, upsertGuardFactOrigin(origins, target, span)
		}
	}
	return append(out, target), upsertGuardFactOrigin(origins, target, span)
}

func removePathFact(in []path.Path, origins []guardFactOrigin, target path.Path) ([]path.Path, []guardFactOrigin) {
	var out []path.Path
	for _, existing := range in {
		if !existing.Equal(target) {
			out = append(out, existing)
		}
	}
	return out, removeGuardFactOrigin(origins, target)
}

func hasPathFact(in []path.Path, target path.Path) bool {
	if target.IsEmpty() {
		return false
	}
	for _, existing := range in {
		if existing.Equal(target) {
			return true
		}
	}
	return false
}

func joinPathFacts(a, b []path.Path, aOrigins, bOrigins []guardFactOrigin) ([]path.Path, []guardFactOrigin) {
	var out []path.Path
	for _, left := range a {
		for _, right := range b {
			if left.Equal(right) {
				out = append(out, left)
				break
			}
		}
	}
	return out, joinGuardFactOrigins(aOrigins, bOrigins)
}

func copyGuardFactOrigins(in []guardFactOrigin) []guardFactOrigin {
	if len(in) == 0 {
		return nil
	}
	out := make([]guardFactOrigin, len(in))
	copy(out, in)
	return out
}

func upsertGuardFactOrigin(in []guardFactOrigin, target path.Path, span diagnostic.Span) []guardFactOrigin {
	out := copyGuardFactOrigins(in)
	if !span.Valid() {
		return removeGuardFactOrigin(out, target)
	}
	for i, origin := range out {
		if origin.target.Equal(target) {
			out[i] = guardFactOrigin{target: target.Clone(), span: span}
			return out
		}
	}
	return append(out, guardFactOrigin{target: target.Clone(), span: span})
}

func removeGuardFactOrigin(in []guardFactOrigin, target path.Path) []guardFactOrigin {
	var out []guardFactOrigin
	for _, origin := range in {
		if !origin.target.Equal(target) {
			out = append(out, origin)
		}
	}
	return out
}

func filterGuardFactOrigins(in []guardFactOrigin, keep func(path.Path) bool) []guardFactOrigin {
	var out []guardFactOrigin
	for _, origin := range in {
		if keep(origin.target) {
			out = append(out, origin)
		}
	}
	return out
}

func joinGuardFactOrigins(a, b []guardFactOrigin) []guardFactOrigin {
	var out []guardFactOrigin
	for _, left := range a {
		if !left.span.Valid() {
			continue
		}
		for _, right := range b {
			if left.target.Equal(right.target) && spanEqual(left.span, right.span) {
				out = append(out, left)
				break
			}
		}
	}
	return out
}

func guardFactOriginSpan(in []guardFactOrigin, target path.Path) diagnostic.Span {
	for _, origin := range in {
		if origin.target.Equal(target) {
			return origin.span
		}
	}
	return diagnostic.Span{}
}

func guardFactOriginsEqual(a, b []guardFactOrigin) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].target.Equal(b[i].target) || !spanEqual(a[i].span, b[i].span) {
			return false
		}
	}
	return true
}

func sortGuardFactOrigins(in []guardFactOrigin) {
	sort.Slice(in, func(i, j int) bool {
		if !in[i].target.Equal(in[j].target) {
			return pathLess(in[i].target, in[j].target)
		}
		if in[i].span.StartLine != in[j].span.StartLine {
			return in[i].span.StartLine < in[j].span.StartLine
		}
		if in[i].span.StartCol != in[j].span.StartCol {
			return in[i].span.StartCol < in[j].span.StartCol
		}
		if in[i].span.EndLine != in[j].span.EndLine {
			return in[i].span.EndLine < in[j].span.EndLine
		}
		return in[i].span.EndCol < in[j].span.EndCol
	})
}

func spanEqual(left, right diagnostic.Span) bool {
	return left.StartLine == right.StartLine &&
		left.StartCol == right.StartCol &&
		left.EndLine == right.EndLine &&
		left.EndCol == right.EndCol
}

func copyPaths(in []path.Path) []path.Path {
	if len(in) == 0 {
		return nil
	}
	out := make([]path.Path, len(in))
	copy(out, in)
	return out
}

func (e guardEnv) provesRuntimeType(result *body.Result, point cfg.Point, expr ast.Expr, want typ.Type) bool {
	if result == nil || expr == nil || want == nil {
		return false
	}
	p, ok := result.ExpressionPath(expr)
	if !ok {
		return false
	}
	for _, c := range e.typeChecks {
		if !c.target.Equal(p) {
			continue
		}
		return runtimeTypeGuardProves(c.name, want)
	}
	return dominantRuntimeTypeGuard(result, point, p, want)
}
