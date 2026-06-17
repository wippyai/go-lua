package diagnostics

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type literalConstraint struct {
	target  path.Path
	value   string
	negated bool
}

type runtimeTypeConstraint struct {
	target path.Path
	name   string
}

type guardEnv struct {
	unreachable bool
	constraints []literalConstraint
	typeChecks  []runtimeTypeConstraint
	present     []path.Path
	truthy      []path.Path
	falsy       []path.Path
	nilPaths    []path.Path
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
	return applyBranchGuard(env, fact.Check, cond)
}

func applyGuardNode(result *body.Result, point cfg.Point, env guardEnv) guardEnv {
	if result == nil {
		return env
	}
	if env.unreachable {
		return env
	}
	if _, ok := result.Call(point); ok {
		env = guardEnv{}
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
			return env.withoutDescendantFacts()
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

func applyBranchGuard(env guardEnv, check branchcond.Check, cond bool) guardEnv {
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
		return env.withTruthy(check.Path)
	}
	if check.Kind == branchcond.CheckFalsy && !cond {
		return env.withTruthy(check.Path)
	}
	if check.Kind == branchcond.CheckTruthy && !cond {
		return env.withFalsy(check.Path)
	}
	if check.Kind == branchcond.CheckFalsy && cond {
		return env.withFalsy(check.Path)
	}
	if check.Kind == branchcond.CheckNil && cond {
		return env.withNil(check.Path)
	}
	if check.Kind == branchcond.CheckNil && !cond {
		return env.withPresent(check.Path)
	}
	if check.Kind == branchcond.CheckNotNil && cond {
		return env.withPresent(check.Path)
	}
	if check.Kind == branchcond.CheckNotNil && !cond {
		return env.withNil(check.Path)
	}
	if check.Kind == branchcond.CheckLiteralEqual && cond {
		return env.with(literalConstraint{target: check.Path, value: check.LiteralString}).withTruthy(check.Path)
	}
	if check.Kind == branchcond.CheckLiteralEqual && !cond {
		return env.with(literalConstraint{target: check.Path, value: check.LiteralString, negated: true})
	}
	if check.Kind == branchcond.CheckLiteralNot && cond {
		return env.with(literalConstraint{target: check.Path, value: check.LiteralString, negated: true})
	}
	if check.Kind == branchcond.CheckLiteralNot && !cond {
		return env.with(literalConstraint{target: check.Path, value: check.LiteralString}).withTruthy(check.Path)
	}
	if check.Kind == branchcond.CheckTypeEqual && cond {
		return env.withType(runtimeTypeConstraint{target: check.Path, name: check.TypeName}).withRuntimeTypePresence(check.Path, check.TypeName)
	}
	if check.Kind == branchcond.CheckTypeEqual && !cond && check.TypeName == "nil" {
		return env.withPresent(check.Path)
	}
	if check.Kind == branchcond.CheckTypeNot && cond && check.TypeName == "nil" {
		return env.withPresent(check.Path)
	}
	if check.Kind == branchcond.CheckTypeNot && !cond {
		return env.withType(runtimeTypeConstraint{target: check.Path, name: check.TypeName}).withRuntimeTypePresence(check.Path, check.TypeName)
	}
	return env
}

func (e guardEnv) with(c literalConstraint) guardEnv {
	if c.target.IsEmpty() || c.value == "" {
		return e
	}
	if e.unreachable {
		return e
	}
	out := guardEnv{
		constraints: append([]literalConstraint(nil), e.constraints...),
		typeChecks:  append([]runtimeTypeConstraint(nil), e.typeChecks...),
		present:     copyPaths(e.present),
		truthy:      copyPaths(e.truthy),
		falsy:       copyPaths(e.falsy),
		nilPaths:    copyPaths(e.nilPaths),
	}
	for i, existing := range out.constraints {
		if existing.target.Equal(c.target) {
			out.constraints[i] = c
			sortGuardEnv(out)
			return out
		}
	}
	out.constraints = append(out.constraints, c)
	sortGuardEnv(out)
	return out
}

func (e guardEnv) withType(c runtimeTypeConstraint) guardEnv {
	if c.target.IsEmpty() || c.name == "" {
		return e
	}
	if e.unreachable {
		return e
	}
	out := guardEnv{
		constraints: append([]literalConstraint(nil), e.constraints...),
		typeChecks:  append([]runtimeTypeConstraint(nil), e.typeChecks...),
		present:     copyPaths(e.present),
		truthy:      copyPaths(e.truthy),
		falsy:       copyPaths(e.falsy),
		nilPaths:    copyPaths(e.nilPaths),
	}
	for i, existing := range out.typeChecks {
		if existing.target.Equal(c.target) {
			out.typeChecks[i] = c
			sortGuardEnv(out)
			return out
		}
	}
	out.typeChecks = append(out.typeChecks, c)
	sortGuardEnv(out)
	return out
}

func (e guardEnv) withPresent(target path.Path) guardEnv {
	if target.IsEmpty() {
		return e
	}
	if e.unreachable {
		return e
	}
	out := guardEnv{
		constraints: append([]literalConstraint(nil), e.constraints...),
		typeChecks:  append([]runtimeTypeConstraint(nil), e.typeChecks...),
		present:     appendPathFact(e.present, target),
		truthy:      copyPaths(e.truthy),
		falsy:       copyPaths(e.falsy),
		nilPaths:    removePathFact(e.nilPaths, target),
	}
	sortGuardEnv(out)
	return out
}

func (e guardEnv) withTruthy(target path.Path) guardEnv {
	if target.IsEmpty() {
		return e
	}
	if e.unreachable {
		return e
	}
	out := guardEnv{
		constraints: append([]literalConstraint(nil), e.constraints...),
		typeChecks:  append([]runtimeTypeConstraint(nil), e.typeChecks...),
		present:     appendPathFact(e.present, target),
		truthy:      appendPathFact(e.truthy, target),
		falsy:       removePathFact(e.falsy, target),
		nilPaths:    removePathFact(e.nilPaths, target),
	}
	sortGuardEnv(out)
	return out
}

func (e guardEnv) withFalsy(target path.Path) guardEnv {
	if target.IsEmpty() {
		return e
	}
	if e.unreachable {
		return e
	}
	out := guardEnv{
		constraints: append([]literalConstraint(nil), e.constraints...),
		typeChecks:  append([]runtimeTypeConstraint(nil), e.typeChecks...),
		present:     copyPaths(e.present),
		truthy:      removePathFact(e.truthy, target),
		falsy:       appendPathFact(e.falsy, target),
		nilPaths:    copyPaths(e.nilPaths),
	}
	sortGuardEnv(out)
	return out
}

func (e guardEnv) withNil(target path.Path) guardEnv {
	if target.IsEmpty() {
		return e
	}
	if e.unreachable {
		return e
	}
	out := guardEnv{
		constraints: append([]literalConstraint(nil), e.constraints...),
		typeChecks:  append([]runtimeTypeConstraint(nil), e.typeChecks...),
		present:     removePathFact(e.present, target),
		truthy:      removePathFact(e.truthy, target),
		falsy:       appendPathFact(e.falsy, target),
		nilPaths:    appendPathFact(e.nilPaths, target),
	}
	sortGuardEnv(out)
	return out
}

func (e guardEnv) withRuntimeTypePresence(target path.Path, typeName string) guardEnv {
	if typeName == "nil" {
		return e.withNil(target)
	}
	if typeName != "boolean" {
		return e.withTruthy(target)
	}
	return e.withPresent(target)
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

func (e guardEnv) withoutDescendantFacts() guardEnv {
	if e.unreachable {
		return e
	}
	var out guardEnv
	for _, c := range e.constraints {
		if rootOnlyPath(c.target) {
			out.constraints = append(out.constraints, c)
		}
	}
	for _, c := range e.typeChecks {
		if rootOnlyPath(c.target) {
			out.typeChecks = append(out.typeChecks, c)
		}
	}
	for _, p := range e.present {
		if rootOnlyPath(p) {
			out.present = append(out.present, p.Clone())
		}
	}
	for _, p := range e.truthy {
		if rootOnlyPath(p) {
			out.truthy = append(out.truthy, p.Clone())
		}
	}
	for _, p := range e.falsy {
		if rootOnlyPath(p) {
			out.falsy = append(out.falsy, p.Clone())
		}
	}
	for _, p := range e.nilPaths {
		if rootOnlyPath(p) {
			out.nilPaths = append(out.nilPaths, p.Clone())
		}
	}
	sortGuardEnv(out)
	return out
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
	for _, p := range e.truthy {
		if keep(p) {
			out.truthy = append(out.truthy, p.Clone())
		}
	}
	for _, p := range e.falsy {
		if keep(p) {
			out.falsy = append(out.falsy, p.Clone())
		}
	}
	for _, p := range e.nilPaths {
		if keep(p) {
			out.nilPaths = append(out.nilPaths, p.Clone())
		}
	}
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
			if left.value == right.value && left.negated == right.negated && left.target.Equal(right.target) {
				out.constraints = append(out.constraints, left)
				break
			}
		}
	}
	for _, left := range a.typeChecks {
		for _, right := range b.typeChecks {
			if left.name == right.name && left.target.Equal(right.target) {
				out.typeChecks = append(out.typeChecks, left)
				break
			}
		}
	}
	for _, left := range a.present {
		for _, right := range b.present {
			if left.Equal(right) {
				out.present = append(out.present, left)
				break
			}
		}
	}
	out.truthy = joinPathFacts(a.truthy, b.truthy)
	out.falsy = joinPathFacts(a.falsy, b.falsy)
	out.nilPaths = joinPathFacts(a.nilPaths, b.nilPaths)
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
		len(a.falsy) != len(b.falsy) || len(a.nilPaths) != len(b.nilPaths) {
		return false
	}
	sortGuardEnv(a)
	sortGuardEnv(b)
	for i := range a.constraints {
		if a.constraints[i].value != b.constraints[i].value || a.constraints[i].negated != b.constraints[i].negated || !a.constraints[i].target.Equal(b.constraints[i].target) {
			return false
		}
	}
	for i := range a.typeChecks {
		if a.typeChecks[i].name != b.typeChecks[i].name || !a.typeChecks[i].target.Equal(b.typeChecks[i].target) {
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
	return true
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
		if left.value != right.value {
			return left.value < right.value
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
	sort.Slice(e.truthy, func(i, j int) bool {
		return pathLess(e.truthy[i], e.truthy[j])
	})
	sort.Slice(e.falsy, func(i, j int) bool {
		return pathLess(e.falsy[i], e.falsy[j])
	})
	sort.Slice(e.nilPaths, func(i, j int) bool {
		return pathLess(e.nilPaths[i], e.nilPaths[j])
	})
}

func pathLess(left, right path.Path) bool {
	return left.Less(right)
}

func appendPathFact(in []path.Path, target path.Path) []path.Path {
	out := copyPaths(in)
	for _, existing := range out {
		if existing.Equal(target) {
			return out
		}
	}
	return append(out, target.Clone())
}

func removePathFact(in []path.Path, target path.Path) []path.Path {
	var out []path.Path
	for _, existing := range in {
		if !existing.Equal(target) {
			out = append(out, existing.Clone())
		}
	}
	return out
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

func joinPathFacts(a, b []path.Path) []path.Path {
	var out []path.Path
	for _, left := range a {
		for _, right := range b {
			if left.Equal(right) {
				out = append(out, left.Clone())
				break
			}
		}
	}
	return out
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
	for i, p := range in {
		out[i] = p.Clone()
	}
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
