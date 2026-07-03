package body

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

// IfBranch is one reachable branch condition in an if/elseif chain. It keeps
// the lowered semantic fact and exposes syntax-derived shape/spans so consumers
// above body do not inspect IfStmt topology directly.
type IfBranch struct {
	Point         cfg.Point
	Fact          BranchConditionFact
	ConditionSpan SourceSpan
}

// IfBranchChain is a reachable if/elseif chain headed by Head. Branches are in
// source order from the head through each elseif. HasDefaultElse reports a final
// non-elseif else block.
type IfBranchChain struct {
	Head           IfBranch
	Branches       []IfBranch
	HasDefaultElse bool
}

// HasElseIf reports whether the chain contains at least one elseif branch.
func (c IfBranchChain) HasElseIf() bool {
	return len(c.Branches) > 1
}

// IfBranchChains returns reachable top-level if/elseif chains. Nested elseif
// statements are represented only inside their head chain.
func (r *Result) IfBranchChains() []IfBranchChain {
	if r == nil {
		return nil
	}
	graph := r.Graph()
	if graph == nil {
		return nil
	}
	branches := r.ifBranches(graph)
	if len(branches) == 0 {
		return nil
	}
	nested := make(map[*ast.IfStmt]struct{})
	byIf := make(map[*ast.IfStmt]IfBranch, len(branches))
	for _, branch := range branches {
		if branch.Fact.If == nil {
			continue
		}
		byIf[branch.Fact.If] = branch
		if child := firstElseIf(branch.Fact.If); child != nil {
			nested[child] = struct{}{}
		}
	}
	out := make([]IfBranchChain, 0, len(branches))
	for _, branch := range branches {
		if branch.Fact.If == nil {
			continue
		}
		if _, ok := nested[branch.Fact.If]; ok {
			continue
		}
		chain := ifBranchChainFromHead(branch.Fact.If, byIf)
		if len(chain.Branches) == 0 {
			continue
		}
		out = append(out, chain)
	}
	return out
}

func (r *Result) ifBranches(graph cfg.Graph) []IfBranch {
	var out []IfBranch
	for _, point := range cfg.RPOReadOnly(graph) {
		if !r.PointNormallyReachable(point) {
			continue
		}
		fact, ok := r.BranchCondition(point)
		if !ok || fact.If == nil {
			continue
		}
		out = append(out, IfBranch{
			Point:         point,
			Fact:          fact,
			ConditionSpan: sourceSpanFromAST(ast.SpanOf(fact.Condition)),
		})
	}
	return out
}

func ifBranchChainFromHead(head *ast.IfStmt, byIf map[*ast.IfStmt]IfBranch) IfBranchChain {
	var chain IfBranchChain
	for stmt := head; stmt != nil; {
		branch, ok := byIf[stmt]
		if !ok {
			return IfBranchChain{}
		}
		if len(chain.Branches) == 0 {
			chain.Head = branch
		}
		chain.Branches = append(chain.Branches, branch)
		next := firstElseIf(stmt)
		if next == nil {
			chain.HasDefaultElse = len(stmt.Else) > 0
			break
		}
		stmt = next
	}
	return chain
}

func firstElseIf(stmt *ast.IfStmt) *ast.IfStmt {
	if stmt == nil || len(stmt.Else) == 0 {
		return nil
	}
	child, _ := stmt.Else[0].(*ast.IfStmt)
	return child
}

func sourceSpanFromAST(span ast.Span) SourceSpan {
	return SourceSpan{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    span.EndCol,
	}
}
