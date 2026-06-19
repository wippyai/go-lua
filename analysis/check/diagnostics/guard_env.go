package diagnostics

import (
	"sort"
	"sync"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
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

// guardEnvironments is a fixpoint over result, so the ~10 diagnostic producers
// that need it share one computation per result instead of each re-deriving it.
// The entry is held only for the duration of one produceWithResolver call, which
// releases it on return.
var (
	guardEnvCacheMu sync.Mutex
	guardEnvCache   = map[*body.Result]map[cfg.Point]guardEnv{}
)

func cachedGuardEnvironments(result *body.Result) map[cfg.Point]guardEnv {
	guardEnvCacheMu.Lock()
	envs, ok := guardEnvCache[result]
	guardEnvCacheMu.Unlock()
	if ok {
		return envs
	}
	envs = guardEnvironments(result)
	guardEnvCacheMu.Lock()
	guardEnvCache[result] = envs
	guardEnvCacheMu.Unlock()
	return envs
}

func releaseGuardEnvironments(result *body.Result) {
	guardEnvCacheMu.Lock()
	delete(guardEnvCache, result)
	guardEnvCacheMu.Unlock()
}

func guardEnvReachableAt(envs map[cfg.Point]guardEnv, point cfg.Point) bool {
	env, ok := envs[point]
	return !ok || !env.unreachable
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
			if !guardEnvEqual(in[point], nextIn) {
				in[point] = nextIn
				changed = true
			}
			if !guardEnvEqual(out[point], nextOut) {
				out[point] = nextOut
				changed = true
			}
		}
	}
	return in
}

func joinPredecessorGuardEnvs(result *body.Result, graph cfg.Graph, point cfg.Point, out map[cfg.Point]guardEnv) guardEnv {
	preds := graph.Predecessors(point)
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
	return applyBranchGuard(env, fact.Check, cond, span)
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
	if fact.HasPath && !fact.Path.IsEmpty() {
		if len(fact.Path.Segments) > 0 {
			return env.withoutFactsForPathAssignment(fact.Path)
		}
		return env.withoutFactsForPath(fact.Path)
	}
	if directDynamicIndexAssignment(fact) {
		return env.withoutDescendantFacts()
	}
	return env
}

func directDynamicIndexAssignment(fact semantics.OrdinaryAssignmentFact) bool {
	if fact.HasPath || !fact.HasContainerPath {
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
		return guardEnv{}
	}
	sig, hasSignature := result.CallSignature(site)
	hasOperationalEffects := hasSignature &&
		sig.OperationalEffects != nil &&
		!sig.OperationalEffects.IsEmpty()
	// A known return value can make PostReturnAuthority true; it is not proof that side effects are complete.
	hasExactSignatureEffects := callOutcomeHasExplicitGuardInvalidation(outcome) || hasOperationalEffects || (hasSignature && sig.Effect.IsClosed())
	if !hasExactSignatureEffects {
		return guardEnv{}
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
	for _, target := range invalidated {
		env = env.withoutFactsForCallInvalidation(target)
	}
	return env
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

func callOutcomeGuardInvalidationPaths(result *body.Result, site factflow.CallSite, outcome callpayload.CallOutcome) ([]path.Path, bool) {
	paramBindings := callGuardArgumentBindings(result, site)
	callBindings := callGuardCallBindings(result, site)
	var out []path.Path
	appendSubstituted := func(bindings []path.Path, target path.Path) {
		substituted, ok := target.Substitute(bindings)
		if !ok || substituted.IsEmpty() {
			return
		}
		out = append(out, substituted)
	}
	for _, invalidation := range outcome.ParamPathInvalidations {
		appendSubstituted(paramBindings, invalidation.Path)
	}
	for _, invalidation := range outcome.NormalReturnFacts.PathInvalidations {
		appendSubstituted(callBindings, invalidation.Path)
	}
	return out, true
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

func (e guardEnv) withPresent(target path.Path) guardEnv {
	return e.withPresentAt(target, diagnostic.Span{})
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

func (e guardEnv) withTruthy(target path.Path) guardEnv {
	return e.withTruthyAt(target, diagnostic.Span{})
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

func (e guardEnv) withFalsy(target path.Path) guardEnv {
	return e.withFalsyAt(target, diagnostic.Span{})
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

func (e guardEnv) withNil(target path.Path) guardEnv {
	return e.withNilAt(target, diagnostic.Span{})
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

func (e guardEnv) withRuntimeTypePresence(target path.Path, typeName string) guardEnv {
	return e.withRuntimeTypePresenceAt(target, typeName, diagnostic.Span{})
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
		return !pathHasStrictPrefix(candidate, target)
	})
}

func (e guardEnv) withoutFactsForCallInvalidation(target path.Path) guardEnv {
	if len(target.Segments) == 0 {
		return e.withoutDescendantFactsOf(target)
	}
	return e.withoutFactsForPath(target)
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
		if !samePathRoot(candidate, target) {
			return false
		}
		return !pathHasPrefix(candidate, target)
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
		return !pathHasPrefix(candidate, target)
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
			out.present = append(out.present, p.Clone())
		}
	}
	out.presentOrigins = filterGuardFactOrigins(e.presentOrigins, keep)
	for _, p := range e.truthy {
		if keep(p) {
			out.truthy = append(out.truthy, p.Clone())
		}
	}
	out.truthyOrigins = filterGuardFactOrigins(e.truthyOrigins, keep)
	for _, p := range e.falsy {
		if keep(p) {
			out.falsy = append(out.falsy, p.Clone())
		}
	}
	out.falsyOrigins = filterGuardFactOrigins(e.falsyOrigins, keep)
	for _, p := range e.nilPaths {
		if keep(p) {
			out.nilPaths = append(out.nilPaths, p.Clone())
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
	sortGuardEnv(a)
	sortGuardEnv(b)
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
				out = append(out, left.Clone())
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

func pathHasStrictPrefix(candidate, prefix path.Path) bool {
	return len(prefix.Segments) < len(candidate.Segments) && pathHasPrefix(candidate, prefix)
}

func samePathRoot(a, b path.Path) bool {
	if a.Symbol != 0 || b.Symbol != 0 {
		return a.Symbol == b.Symbol && a.Version == b.Version
	}
	return a.Root == b.Root && a.Version == b.Version
}

func pathHasPrefix(candidate, prefix path.Path) bool {
	if candidate.Symbol != 0 || prefix.Symbol != 0 {
		if candidate.Symbol != prefix.Symbol || candidate.Version != prefix.Version {
			return false
		}
	} else if candidate.Root != prefix.Root {
		return false
	}
	if len(prefix.Segments) > len(candidate.Segments) {
		return false
	}
	for i := range prefix.Segments {
		if candidate.Segments[i] != prefix.Segments[i] {
			return false
		}
	}
	return true
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
		t, ok := runtimeTypeName(c.name)
		return ok && subtype.IsSubtype(t, want)
	}
	return dominantRuntimeTypeGuard(result, point, p, want)
}
