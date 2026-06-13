package diagnostics

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/channelselect"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/compiler/ast"
)

type ChannelSelectExhaustiveness Config

type channelSelectDiagnosticInfo struct {
	result     pathdom.Path
	cases      []channelSelectDiagnosticCase
	hasDefault bool
}

type channelSelectDiagnosticCase struct {
	path pathdom.Path
	name string
}

func (p ChannelSelectExhaustiveness) Produce(result *check.Result) []diagnostic.Diagnostic {
	_ = p
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	selects := channelSelectDiagnosticInfos(result)
	if len(selects) == 0 {
		return nil
	}
	branches := channelSelectBranchConditions(result, graph)
	if len(branches) == 0 {
		return nil
	}
	nested := nestedElseIfStatements(branches)
	byIf := make(map[*ast.IfStmt]semantics.BranchConditionFact, len(branches))
	for _, branch := range branches {
		if branch.If != nil {
			byIf[branch.If] = branch
		}
	}
	var out []diagnostic.Diagnostic
	for _, branch := range branches {
		if branch.If == nil || nested[branch.If] || !hasElseIf(branch.If) {
			continue
		}
		if diag, ok := channelSelectChainDiagnostic(result, branch.If, byIf, selects); ok {
			out = append(out, diag)
		}
	}
	return out
}

func channelSelectDiagnosticInfos(result *check.Result) []channelSelectDiagnosticInfo {
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	var out []channelSelectDiagnosticInfo
	for _, point := range graph.RPO() {
		call, ok := result.Call(point)
		if !ok || !call.HasChannelSelect || !call.ChannelSelect.ResultTarget.HasPath {
			continue
		}
		selectFact := call.ChannelSelect
		if selectFact.ResultTarget.Path.IsEmpty() || len(selectFact.Cases) == 0 {
			continue
		}
		info := channelSelectDiagnosticInfo{result: selectFact.ResultTarget.Path, hasDefault: selectFact.HasDefault}
		for _, c := range selectFact.Cases {
			if !c.HasChannelPath || c.ChannelPath.IsEmpty() {
				continue
			}
			name := c.ChannelPath.DisplayRoot(result.SymbolName)
			if name == "" {
				name = c.ChannelPath.String()
			}
			info.cases = append(info.cases, channelSelectDiagnosticCase{
				path: c.ChannelPath,
				name: name,
			})
		}
		if len(info.cases) > 0 {
			out = append(out, info)
		}
	}
	return out
}

func channelSelectBranchConditions(result *check.Result, graph cfg.Graph) []semantics.BranchConditionFact {
	var out []semantics.BranchConditionFact
	for _, point := range graph.RPO() {
		branch, ok := result.BranchCondition(point)
		if !ok || branch.If == nil {
			continue
		}
		out = append(out, branch)
	}
	return out
}

func nestedElseIfStatements(branches []semantics.BranchConditionFact) map[*ast.IfStmt]bool {
	out := make(map[*ast.IfStmt]bool)
	for _, branch := range branches {
		if branch.If == nil || len(branch.If.Else) == 0 {
			continue
		}
		if nested, ok := branch.If.Else[0].(*ast.IfStmt); ok && nested != nil {
			out[nested] = true
		}
	}
	return out
}

func hasElseIf(stmt *ast.IfStmt) bool {
	if stmt == nil || len(stmt.Else) == 0 {
		return false
	}
	_, ok := stmt.Else[0].(*ast.IfStmt)
	return ok
}

func channelSelectChainDiagnostic(
	result *check.Result,
	head *ast.IfStmt,
	byIf map[*ast.IfStmt]semantics.BranchConditionFact,
	selects []channelSelectDiagnosticInfo,
) (diagnostic.Diagnostic, bool) {
	chain := ifElseIfChain(head)
	selected := -1
	handled := make(map[int]bool)
	for _, stmt := range chain {
		branch, ok := byIf[stmt]
		if !ok || branch.Check.Kind != branchcond.CheckPathEqual {
			continue
		}
		selectIndex, caseIndex, ok := channelSelectCaseForCheck(branch.Check, selects)
		if !ok {
			continue
		}
		if selected == -1 {
			selected = selectIndex
		}
		if selected == selectIndex {
			handled[caseIndex] = true
		}
	}
	if selected == -1 || len(handled) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	info := selects[selected]
	if info.hasDefault {
		return diagnostic.Diagnostic{}, false
	}
	if len(handled) >= len(info.cases) {
		return diagnostic.Diagnostic{}, false
	}
	var missing []string
	for i, c := range info.cases {
		if !handled[i] {
			missing = append(missing, c.name)
		}
	}
	if len(missing) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	return channelSelectExhaustivenessDiagnostic(head, missing), true
}

func ifElseIfChain(head *ast.IfStmt) []*ast.IfStmt {
	var chain []*ast.IfStmt
	for stmt := head; stmt != nil; {
		chain = append(chain, stmt)
		if len(stmt.Else) == 0 {
			break
		}
		next, ok := stmt.Else[0].(*ast.IfStmt)
		if !ok {
			break
		}
		stmt = next
	}
	return chain
}

func channelSelectCaseForCheck(
	check branchcond.Check,
	selects []channelSelectDiagnosticInfo,
) (int, int, bool) {
	for selectIndex, info := range selects {
		resultChannel := info.result.Field(channelselect.ResultChannelField)
		for caseIndex, c := range info.cases {
			if pathsMatchPair(check.Path, check.OtherPath, resultChannel, c.path) {
				return selectIndex, caseIndex, true
			}
		}
	}
	return -1, -1, false
}

func pathsMatchPair(left, right, wantLeft, wantRight pathdom.Path) bool {
	return (left.Equal(wantLeft) && right.Equal(wantRight)) ||
		(left.Equal(wantRight) && right.Equal(wantLeft))
}

func channelSelectExhaustivenessDiagnostic(head *ast.IfStmt, missing []string) diagnostic.Diagnostic {
	span := ast.SpanOf(head.Condition)
	caseWord := "case"
	if len(missing) > 1 {
		caseWord = "cases"
	}
	message := fmt.Sprintf("channel select is not exhaustive; missing %s: %s", caseWord, strings.Join(missing, ", "))
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      span.StartLine,
			Column:    span.StartCol,
			EndLine:   span.EndLine,
			EndColumn: span.EndCol,
		},
		Span:     span,
		Code:     CodeChannelSelectExhaustive,
		Severity: diagnostic.SeverityWarning,
		Message:  message,
		Explanation: diagnostic.NewExplanation(diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustProven,
			Span:    span,
			Message: message,
		}),
		Labels: []diagnostic.Label{{Span: span, Message: "channel select case chain"}},
		Help:   "Handle each channel select case explicitly in the if/elseif chain.",
	}
}
