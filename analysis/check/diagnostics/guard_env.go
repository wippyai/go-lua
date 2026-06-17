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
	constraints []literalConstraint
	typeChecks  []runtimeTypeConstraint
	present     []path.Path
}

func guardEnvironments(result *body.Result) map[cfg.Point]guardEnv {
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	in := make(map[cfg.Point]guardEnv)
	out := make(map[cfg.Point]guardEnv)
	for _, point := range graph.RPO() {
		in[point] = guardEnv{}
		out[point] = guardEnv{}
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
	fact, ok := result.OrdinaryAssignment(point)
	if !ok {
		return env
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
	if check.Kind == branchcond.CheckTruthy && cond {
		return env.withPresent(check.Path)
	}
	if check.Kind == branchcond.CheckFalsy && !cond {
		return env.withPresent(check.Path)
	}
	if check.Kind == branchcond.CheckNil && !cond {
		return env.withPresent(check.Path)
	}
	if check.Kind == branchcond.CheckNotNil && cond {
		return env.withPresent(check.Path)
	}
	if check.Kind == branchcond.CheckLiteralEqual && cond {
		return env.with(literalConstraint{target: check.Path, value: check.LiteralString}).withPresent(check.Path)
	}
	if check.Kind == branchcond.CheckLiteralEqual && !cond {
		return env.with(literalConstraint{target: check.Path, value: check.LiteralString, negated: true})
	}
	if check.Kind == branchcond.CheckLiteralNot && cond {
		return env.with(literalConstraint{target: check.Path, value: check.LiteralString, negated: true})
	}
	if check.Kind == branchcond.CheckLiteralNot && !cond {
		return env.with(literalConstraint{target: check.Path, value: check.LiteralString}).withPresent(check.Path)
	}
	if check.Kind == branchcond.CheckTypeEqual && cond {
		return env.withType(runtimeTypeConstraint{target: check.Path, name: check.TypeName}).withPresent(check.Path)
	}
	if check.Kind == branchcond.CheckTypeNot && !cond {
		return env.withType(runtimeTypeConstraint{target: check.Path, name: check.TypeName})
	}
	return env
}

func (e guardEnv) with(c literalConstraint) guardEnv {
	if c.target.IsEmpty() || c.value == "" {
		return e
	}
	out := guardEnv{
		constraints: append([]literalConstraint(nil), e.constraints...),
		typeChecks:  append([]runtimeTypeConstraint(nil), e.typeChecks...),
		present:     copyPaths(e.present),
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
	out := guardEnv{
		constraints: append([]literalConstraint(nil), e.constraints...),
		typeChecks:  append([]runtimeTypeConstraint(nil), e.typeChecks...),
		present:     copyPaths(e.present),
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
	out := guardEnv{
		constraints: append([]literalConstraint(nil), e.constraints...),
		typeChecks:  append([]runtimeTypeConstraint(nil), e.typeChecks...),
		present:     copyPaths(e.present),
	}
	for _, existing := range out.present {
		if existing.Equal(target) {
			return out
		}
	}
	out.present = append(out.present, target)
	sortGuardEnv(out)
	return out
}

func (e guardEnv) hasPresent(target path.Path) bool {
	if target.IsEmpty() {
		return false
	}
	for _, existing := range e.present {
		if existing.Equal(target) {
			return true
		}
	}
	return false
}

func (e guardEnv) withoutDescendantFacts() guardEnv {
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
	sortGuardEnv(out)
	return out
}

func rootOnlyPath(p path.Path) bool {
	return !p.IsEmpty() && len(p.Segments) == 0
}

func joinGuardEnvs(a, b guardEnv) guardEnv {
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
	sortGuardEnv(out)
	return out
}

func guardEnvEqual(a, b guardEnv) bool {
	if len(a.constraints) != len(b.constraints) || len(a.typeChecks) != len(b.typeChecks) || len(a.present) != len(b.present) {
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
		left := e.present[i]
		right := e.present[j]
		if left.Root != right.Root {
			return left.Root < right.Root
		}
		if left.Symbol != right.Symbol {
			return left.Symbol < right.Symbol
		}
		return segment.FormatSegments(left.Segments) < segment.FormatSegments(right.Segments)
	})
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
