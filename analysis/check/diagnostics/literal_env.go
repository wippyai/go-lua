package diagnostics

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
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

type literalEnv struct {
	constraints []literalConstraint
	typeChecks  []runtimeTypeConstraint
}

func literalEnvironments(result *body.Result) map[cfg.Point]literalEnv {
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	in := make(map[cfg.Point]literalEnv)
	out := make(map[cfg.Point]literalEnv)
	for _, point := range graph.RPO() {
		in[point] = literalEnv{}
		out[point] = literalEnv{}
	}
	changed := true
	for changed {
		changed = false
		for _, point := range graph.RPO() {
			nextIn := joinPredecessorEnvs(result, graph, point, out)
			nextOut := nextIn
			if !envEqual(in[point], nextIn) {
				in[point] = nextIn
				changed = true
			}
			if !envEqual(out[point], nextOut) {
				out[point] = nextOut
				changed = true
			}
		}
	}
	return in
}

func joinPredecessorEnvs(result *body.Result, graph cfg.Graph, point cfg.Point, out map[cfg.Point]literalEnv) literalEnv {
	preds := graph.Predecessors(point)
	if len(preds) == 0 {
		return literalEnv{}
	}
	var env literalEnv
	for i, pred := range preds {
		edgeEnv := applyLiteralEdge(result, graph, pred, point, out[pred])
		if i == 0 {
			env = edgeEnv
			continue
		}
		env = joinEnvs(env, edgeEnv)
	}
	return env
}

func applyLiteralEdge(result *body.Result, graph cfg.Graph, from, to cfg.Point, env literalEnv) literalEnv {
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
	return applyBranchLiteral(env, fact.Check, cond)
}

func applyBranchLiteral(env literalEnv, check branchcond.Check, cond bool) literalEnv {
	if check.Kind == branchcond.CheckLiteralEqual && cond {
		return env.with(literalConstraint{target: check.Path, value: check.LiteralString})
	}
	if check.Kind == branchcond.CheckLiteralEqual && !cond {
		return env.with(literalConstraint{target: check.Path, value: check.LiteralString, negated: true})
	}
	if check.Kind == branchcond.CheckLiteralNot && cond {
		return env.with(literalConstraint{target: check.Path, value: check.LiteralString, negated: true})
	}
	if check.Kind == branchcond.CheckLiteralNot && !cond {
		return env.with(literalConstraint{target: check.Path, value: check.LiteralString})
	}
	if check.Kind == branchcond.CheckTypeEqual && cond {
		return env.withType(runtimeTypeConstraint{target: check.Path, name: check.TypeName})
	}
	if check.Kind == branchcond.CheckTypeNot && !cond {
		return env.withType(runtimeTypeConstraint{target: check.Path, name: check.TypeName})
	}
	return env
}

func (e literalEnv) with(c literalConstraint) literalEnv {
	if c.target.IsEmpty() || c.value == "" {
		return e
	}
	out := literalEnv{constraints: append([]literalConstraint(nil), e.constraints...)}
	for i, existing := range out.constraints {
		if existing.target.Equal(c.target) {
			out.constraints[i] = c
			sortEnv(out)
			return out
		}
	}
	out.constraints = append(out.constraints, c)
	sortEnv(out)
	return out
}

func (e literalEnv) withType(c runtimeTypeConstraint) literalEnv {
	if c.target.IsEmpty() || c.name == "" {
		return e
	}
	out := literalEnv{
		constraints: append([]literalConstraint(nil), e.constraints...),
		typeChecks:  append([]runtimeTypeConstraint(nil), e.typeChecks...),
	}
	for i, existing := range out.typeChecks {
		if existing.target.Equal(c.target) {
			out.typeChecks[i] = c
			sortEnv(out)
			return out
		}
	}
	out.typeChecks = append(out.typeChecks, c)
	sortEnv(out)
	return out
}

func joinEnvs(a, b literalEnv) literalEnv {
	if (len(a.constraints) == 0 || len(b.constraints) == 0) && (len(a.typeChecks) == 0 || len(b.typeChecks) == 0) {
		return literalEnv{}
	}
	var out literalEnv
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
	sortEnv(out)
	return out
}

func envEqual(a, b literalEnv) bool {
	if len(a.constraints) != len(b.constraints) || len(a.typeChecks) != len(b.typeChecks) {
		return false
	}
	sortEnv(a)
	sortEnv(b)
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
	return true
}

func sortEnv(e literalEnv) {
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
}

func (e literalEnv) provesRuntimeType(result *body.Result, point cfg.Point, expr ast.Expr, want typ.Type) bool {
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

func runtimeTypeName(name string) (typ.Type, bool) {
	switch name {
	case "nil":
		return typ.Nil, true
	case "boolean":
		return typ.Boolean, true
	case "number":
		return typ.Number, true
	case "string":
		return typ.String, true
	default:
		return nil, false
	}
}

func dominantRuntimeTypeGuard(result *body.Result, point cfg.Point, p path.Path, want typ.Type) bool {
	graph := result.Graph()
	if graph == nil || point == 0 || p.IsEmpty() {
		return false
	}
	dom := dominance.ComputeImmediateDominatorInfo(graph)
	for _, branch := range graph.RPO() {
		if !dom.StrictlyDominates(branch, point) {
			continue
		}
		fact, ok := result.BranchCondition(branch)
		if !ok || !fact.Check.Path.Equal(p) {
			continue
		}
		rejectCond, ok := runtimeTypeGuardRejectCond(fact.Check.Kind)
		if !ok {
			continue
		}
		t, ok := runtimeTypeName(fact.Check.TypeName)
		if !ok || !subtype.IsSubtype(t, want) {
			continue
		}
		for _, succ := range graph.Successors(branch) {
			cond, ok := graph.EdgeCond(branch, succ)
			if !ok || cond != rejectCond {
				continue
			}
			if !reachable(graph, succ, point) {
				return true
			}
		}
	}
	return false
}

func runtimeTypeGuardRejectCond(kind branchcond.CheckKind) (bool, bool) {
	switch kind {
	case branchcond.CheckTypeEqual:
		return false, true
	case branchcond.CheckTypeNot:
		return true, true
	default:
		return false, false
	}
}

func reachable(graph cfg.Graph, from, to cfg.Point) bool {
	if graph == nil {
		return false
	}
	if from == to {
		return true
	}
	seen := map[cfg.Point]struct{}{from: {}}
	stack := []cfg.Point{from}
	for len(stack) != 0 {
		last := len(stack) - 1
		point := stack[last]
		stack = stack[:last]
		for _, succ := range graph.Successors(point) {
			if succ == to {
				return true
			}
			if _, ok := seen[succ]; ok {
				continue
			}
			seen[succ] = struct{}{}
			stack = append(stack, succ)
		}
	}
	return false
}
