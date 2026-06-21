package diagnostics

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/type/channelselect"
	"github.com/wippyai/go-lua/compiler/ast"
)

type channelSelectExhaustiveness producerContext

type selectInfo struct {
	point      cfg.Point
	result     pathdom.Path
	cases      []selectCase
	hasDefault bool
}

type selectCase struct {
	path pathdom.Path
	name string
}

type exhaustivenessEvidence struct {
	resultChannel pathdom.Path
	handled       []string
	missing       []string
	hasDefault    bool
}

type channelSelectCaseIndex map[channelSelectCaseKey][]channelSelectCaseMatch

type channelSelectCaseKey struct {
	resultChannel pathdom.PathKey
	channel       pathdom.PathKey
}

type channelSelectCaseMatch struct {
	selectIndex int
	caseIndex   int
}

type channelSelectBranch struct {
	point cfg.Point
	fact  semantics.BranchConditionFact
}

func (p channelSelectExhaustiveness) Produce(result *body.Result) []diagnostic.Diagnostic {
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	selects := collectSelectInfos(result)
	if len(selects) == 0 {
		return nil
	}
	branches := channelSelectBranchConditions(result, graph)
	if len(branches) == 0 {
		return nil
	}
	cases := newChannelSelectCaseIndex(selects)
	nested := nestedElseIfStatements(branches)
	byIf := make(map[*ast.IfStmt]channelSelectBranch, len(branches))
	for _, branch := range branches {
		if branch.fact.If != nil {
			byIf[branch.fact.If] = branch
		}
	}
	var out []diagnostic.Diagnostic
	for _, branch := range branches {
		if branch.fact.If == nil || nested[branch.fact.If] || !hasElseIf(branch.fact.If) {
			continue
		}
		if diag, ok := channelSelectChainDiagnostic(result, graph, p.flow, branch.fact.If, byIf, selects, cases); ok {
			out = append(out, diag)
		}
	}
	return out
}

func collectSelectInfos(result *body.Result) []selectInfo {
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	envs := cachedGuardEnvironments(result)
	var out []selectInfo
	for _, point := range graph.RPO() {
		if !guardEnvReachableAt(envs, point) {
			continue
		}
		call, ok := result.Call(point)
		if !ok || !call.HasChannelSelect || !call.ChannelSelect.ResultTarget.HasPath {
			continue
		}
		selectFact := call.ChannelSelect
		if selectFact.ResultTarget.Path.IsEmpty() || len(selectFact.Cases) == 0 {
			continue
		}
		info := selectInfo{point: point, result: selectFact.ResultTarget.Path, hasDefault: selectFact.HasDefault}
		for _, c := range selectFact.Cases {
			if !c.HasChannelPath || c.ChannelPath.IsEmpty() {
				continue
			}
			name := c.ChannelPath.DisplayRoot(result.SymbolName)
			if name == "" {
				name = c.ChannelPath.String()
			}
			info.cases = append(info.cases, selectCase{
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

func channelSelectBranchConditions(result *body.Result, graph cfg.Graph) []channelSelectBranch {
	envs := cachedGuardEnvironments(result)
	var out []channelSelectBranch
	for _, point := range graph.RPO() {
		if !guardEnvReachableAt(envs, point) {
			continue
		}
		branch, ok := result.BranchCondition(point)
		if !ok || branch.If == nil {
			continue
		}
		out = append(out, channelSelectBranch{point: point, fact: branch})
	}
	return out
}

func nestedElseIfStatements(branches []channelSelectBranch) map[*ast.IfStmt]bool {
	out := make(map[*ast.IfStmt]bool)
	for _, branch := range branches {
		if branch.fact.If == nil || len(branch.fact.If.Else) == 0 {
			continue
		}
		if nested, ok := branch.fact.If.Else[0].(*ast.IfStmt); ok && nested != nil {
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
	result *body.Result,
	graph cfg.Graph,
	flow *diagnosticFlowCache,
	head *ast.IfStmt,
	byIf map[*ast.IfStmt]channelSelectBranch,
	selects []selectInfo,
	cases channelSelectCaseIndex,
) (diagnostic.Diagnostic, bool) {
	chain := ifElseIfChain(head)
	headBranch, ok := byIf[head]
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	handledBySelect := make(map[int]map[int]bool)
	for _, stmt := range chain {
		branch, ok := byIf[stmt]
		if !ok || branch.fact.Check.Kind != branchcond.CheckPathEqual {
			continue
		}
		for _, match := range cases.matchesForCheck(branch.fact.Check) {
			if match.selectIndex < 0 || match.selectIndex >= len(selects) {
				continue
			}
			if !selectCanReachBranch(selects[match.selectIndex], headBranch.point, graph, flow) {
				continue
			}
			handled := handledBySelect[match.selectIndex]
			if handled == nil {
				handled = make(map[int]bool)
				handledBySelect[match.selectIndex] = handled
			}
			handled[match.caseIndex] = true
		}
	}
	selected, handled, ok := bestChannelSelectCandidate(handledBySelect)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	info := selects[selected]
	if info.hasDefault {
		return diagnostic.Diagnostic{}, false
	}
	if len(handled) >= len(info.cases) {
		return diagnostic.Diagnostic{}, false
	}
	var handledNames []string
	var missing []string
	for i, c := range info.cases {
		if handled[i] {
			handledNames = appendUniqueString(handledNames, c.name)
		} else {
			missing = appendUniqueString(missing, c.name)
		}
	}
	if len(missing) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	return newExhaustivenessDiagnostic(head, exhaustivenessEvidence{
		resultChannel: info.result.Field(channelselect.ResultChannelField),
		handled:       handledNames,
		missing:       missing,
		hasDefault:    info.hasDefault,
	}), true
}

func selectCanReachBranch(info selectInfo, branchPoint cfg.Point, graph cfg.Graph, flow *diagnosticFlowCache) bool {
	if graph == nil {
		return true
	}
	if info.point == branchPoint {
		return true
	}
	return diagnosticCanReach(flow, graph, info.point, branchPoint)
}

func bestChannelSelectCandidate(handledBySelect map[int]map[int]bool) (int, map[int]bool, bool) {
	selected := -1
	var selectedHandled map[int]bool
	for selectIndex, handled := range handledBySelect {
		if len(handled) == 0 {
			continue
		}
		if selected == -1 || len(handled) > len(selectedHandled) || len(handled) == len(selectedHandled) && selectIndex > selected {
			selected = selectIndex
			selectedHandled = handled
		}
	}
	return selected, selectedHandled, selected != -1
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

func newChannelSelectCaseIndex(selects []selectInfo) channelSelectCaseIndex {
	out := make(channelSelectCaseIndex)
	for selectIndex, info := range selects {
		resultChannel := info.result.Field(channelselect.ResultChannelField)
		resultKey := resultChannel.Key()
		if resultKey == "" {
			continue
		}
		for caseIndex, c := range info.cases {
			channelKey := c.path.Key()
			if channelKey == "" {
				continue
			}
			key := channelSelectCaseKey{resultChannel: resultKey, channel: channelKey}
			out[key] = append(out[key], channelSelectCaseMatch{
				selectIndex: selectIndex,
				caseIndex:   caseIndex,
			})
		}
	}
	return out
}

func (idx channelSelectCaseIndex) matchesForCheck(check branchcond.Check) []channelSelectCaseMatch {
	matches := idx[channelSelectCaseKey{resultChannel: check.Path.Key(), channel: check.OtherPath.Key()}]
	if len(matches) == 0 {
		matches = idx[channelSelectCaseKey{resultChannel: check.OtherPath.Key(), channel: check.Path.Key()}]
	}
	if len(matches) == 0 {
		return nil
	}
	return matches
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func newExhaustivenessDiagnostic(head *ast.IfStmt, evidence exhaustivenessEvidence) diagnostic.Diagnostic {
	span := ast.SpanOf(head.Condition)
	caseWord := pluralize(len(evidence.missing), "case", "cases")
	message := channelSelectExhaustivenessMessage(caseWord, channelCaseList(evidence.missing))
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        span,
		Code:        CodeChannelSelectExhaustive,
		Severity:    diagnostic.SeverityWarning,
		Message:     message,
		Explanation: exhaustivenessExplanation(span, evidence),
		Help:        channelSelectExhaustivenessHelp(),
		Labels:      []diagnostic.Label{sourceLabel(span, labelChannelCaseTest)},
	})
}

func exhaustivenessExplanation(span diagnostic.Span, evidence exhaustivenessEvidence) diagnostic.Explanation {
	items := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    span,
			Message: selectedChannelPathEvidence(evidence.resultChannel.String()),
		},
	}
	if len(evidence.handled) > 0 {
		items = append(items, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    span,
			Message: handledChannelCasesEvidence(channelCaseList(evidence.handled)),
		})
	}
	items = append(items, diagnostic.Evidence{
		Kind:    diagnostic.EvidenceMissingProof,
		Trust:   diagnostic.TrustUnknown,
		Span:    span,
		Message: missingChannelCasesEvidence(channelCaseList(evidence.missing)),
	})
	if !evidence.hasDefault {
		items = append(items, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    span,
			Message: missingChannelDefaultEvidence(),
		})
	}
	return diagnostic.NewExplanation(items...)
}

func channelCaseList(cases []string) string {
	return strings.Join(codeNames(cases), ", ")
}

func codeNames(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, codeName(name))
	}
	return out
}
