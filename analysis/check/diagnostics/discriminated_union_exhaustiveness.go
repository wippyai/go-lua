package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/compiler/ast"
)

type discriminatedUnionExhaustiveness producerContext
type discriminantBranch struct {
	point cfg.Point
	fact  semantics.BranchConditionFact
}

func (p discriminatedUnionExhaustiveness) Produce(result *body.Result) []diagnostic.Diagnostic {
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	branches := p.discriminantBranchConditions(result, graph)
	ifs := make([]*ast.IfStmt, 0, len(branches))
	byIf := make(map[*ast.IfStmt]discriminantBranch, len(branches))
	for _, branch := range branches {
		if branch.fact.If == nil {
			continue
		}
		ifs = append(ifs, branch.fact.If)
		byIf[branch.fact.If] = branch
	}
	nested := nestedElseIfStatements(ifs)
	var out []diagnostic.Diagnostic
	for _, branch := range branches {
		if branch.fact.If == nil || nested[branch.fact.If] {
			continue
		}
		if diag, ok := p.optionalChainDiagnostic(result, branch.fact.If, byIf); ok {
			out = append(out, diag)
		}
	}
	out = append(out, p.tableDispatchDiagnostics(result, graph)...)
	out = append(out, p.registrationDiagnostics(result, graph)...)
	return out
}

func (p discriminatedUnionExhaustiveness) discriminantBranchConditions(result *body.Result, graph cfg.Graph) []discriminantBranch {
	var out []discriminantBranch
	for _, point := range graph.RPO() {
		if !result.PointNormallyReachable(point) {
			continue
		}
		branch, ok := result.BranchCondition(point)
		if !ok || branch.If == nil {
			continue
		}
		out = append(out, discriminantBranch{point: point, fact: branch})
	}
	return out
}
