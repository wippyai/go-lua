package diagnostics

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
)

type literalConstraint struct {
	target  path.Path
	value   string
	negated bool
}

type literalEnv struct {
	constraints []literalConstraint
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

func joinEnvs(a, b literalEnv) literalEnv {
	if len(a.constraints) == 0 || len(b.constraints) == 0 {
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
	sortEnv(out)
	return out
}

func envEqual(a, b literalEnv) bool {
	if len(a.constraints) != len(b.constraints) {
		return false
	}
	sortEnv(a)
	sortEnv(b)
	for i := range a.constraints {
		if a.constraints[i].value != b.constraints[i].value || a.constraints[i].negated != b.constraints[i].negated || !a.constraints[i].target.Equal(b.constraints[i].target) {
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
}
