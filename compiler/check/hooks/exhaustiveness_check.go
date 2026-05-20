package hooks

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	flowpath "github.com/wippyai/go-lua/compiler/check/abstract/transfer/path"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/numparse"
	"github.com/wippyai/go-lua/types/typ"
)

// CheckExhaustiveness warns when a match-like if/elseif chain misses variants
// from a provably closed discriminated union.
func CheckExhaustiveness(fn *ast.FunctionExpr, graph *cfg.Graph, evidence api.FlowEvidence, synth api.BaseSynth, sourceName string) []diag.Diagnostic {
	if fn == nil || graph == nil || synth == nil {
		return nil
	}
	checker := exhaustivenessChecker{
		branchPoint: branchPointsByCondition(evidence.Branches),
		graph:       graph,
		bindings:    graph.Bindings(),
		selectCases: selectCasesByResult(graph, evidence.Assignments),
		synth:       synth,
		sourceName:  sourceName,
	}
	checker.checkStmts(fn.Stmts)
	return checker.diags
}

type exhaustivenessChecker struct {
	branchPoint map[ast.Expr]cfg.Point
	graph       *cfg.Graph
	bindings    *bind.BindingTable
	selectCases map[string]selectCaseDomain
	synth       api.BaseSynth
	sourceName  string
	diags       []diag.Diagnostic
}

type discriminantCheck struct {
	object     ast.Expr
	objectPath constraint.Path
	field      string
	path       string
	literal    *typ.Literal
	value      ast.Expr
	valuePath  constraint.Path
	valueName  string
	condition  ast.Expr
	point      cfg.Point
}

type selectCaseDomain struct {
	cases []selectCase
}

type selectCase struct {
	path constraint.Path
	name string
}

func branchPointsByCondition(branches []api.BranchEvidence) map[ast.Expr]cfg.Point {
	points := make(map[ast.Expr]cfg.Point)
	for _, branch := range branches {
		p := branch.Point
		info := branch.Info
		if info != nil && info.Condition != nil {
			points[info.Condition] = p
		}
	}
	return points
}

func (c *exhaustivenessChecker) checkStmts(stmts []ast.Stmt) {
	for _, stmt := range stmts {
		c.checkStmt(stmt)
	}
}

func (c *exhaustivenessChecker) checkStmt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.IfStmt:
		c.checkIf(s)
	case *ast.WhileStmt:
		c.checkStmts(s.Stmts)
	case *ast.RepeatStmt:
		c.checkStmts(s.Stmts)
	case *ast.NumberForStmt:
		c.checkStmts(s.Stmts)
	case *ast.GenericForStmt:
		c.checkStmts(s.Stmts)
	case *ast.DoBlockStmt:
		c.checkStmts(s.Stmts)
	}
}

func (c *exhaustivenessChecker) checkIf(stmt *ast.IfStmt) {
	c.checkIfChain(stmt)

	for current := stmt; current != nil; {
		c.checkStmts(current.Then)
		next, ok := singleElseIf(current)
		if !ok {
			c.checkStmts(current.Else)
			break
		}
		current = next
	}
}

func (c *exhaustivenessChecker) checkIfChain(stmt *ast.IfStmt) {
	checks, hasElse, ok := c.collectDiscriminantChain(stmt)
	if !ok || hasElse || len(checks) < 2 {
		return
	}

	first := checks[0]
	for _, check := range checks[1:] {
		if check.path != first.path || check.field != first.field {
			return
		}
	}

	if first.literal == nil {
		c.checkSelectCaseChain(stmt, checks)
		return
	}

	objectType := c.synth.TypeOf(first.object, first.point)
	domain, ok := narrow.ClosedDiscriminantDomain(objectType, first.field)
	if !ok {
		return
	}

	handled := make([]*typ.Literal, 0, len(checks))
	handledInDomain := false
	for _, check := range checks {
		handled = append(handled, check.literal)
		if domain.Contains(check.literal) {
			handledInDomain = true
		}
	}
	if !handledInDomain {
		return
	}

	missing := domain.Missing(handled)
	if len(missing) == 0 {
		return
	}
	c.addNonExhaustiveWarning(stmt.Condition, first.path, missing)
}

func (c *exhaustivenessChecker) checkSelectCaseChain(stmt *ast.IfStmt, checks []discriminantCheck) {
	first := checks[0]
	if first.field != "channel" || first.objectPath.IsEmpty() {
		return
	}
	domain, ok := c.selectCases[pathKey(first.objectPath)]
	if !ok || len(domain.cases) < 2 {
		return
	}

	handled := make(map[string]struct{}, len(checks))
	for _, check := range checks {
		if check.valuePath.IsEmpty() {
			return
		}
		key := pathKey(check.valuePath)
		if !domain.contains(key) {
			return
		}
		handled[key] = struct{}{}
	}

	var missing []string
	for _, candidate := range domain.cases {
		if _, ok := handled[pathKey(candidate.path)]; !ok {
			missing = append(missing, candidate.name)
		}
	}
	if len(missing) == 0 {
		return
	}
	c.addNonExhaustiveNamesWarning(stmt.Condition, first.path, missing)
}

func (c *exhaustivenessChecker) collectDiscriminantChain(stmt *ast.IfStmt) ([]discriminantCheck, bool, bool) {
	var checks []discriminantCheck
	current := stmt
	for current != nil {
		check, ok := c.discriminantCheck(current.Condition)
		if !ok {
			return nil, false, false
		}
		checks = append(checks, check)

		next, ok := singleElseIf(current)
		if !ok {
			return checks, len(current.Else) > 0, true
		}
		current = next
	}
	return checks, false, true
}

func (c *exhaustivenessChecker) discriminantCheck(condition ast.Expr) (discriminantCheck, bool) {
	point, ok := c.branchPoint[condition]
	if !ok {
		return discriminantCheck{}, false
	}

	check, ok := equalityDiscriminantCheck(condition)
	if !ok {
		return discriminantCheck{}, false
	}
	check.condition = condition
	check.point = point
	if check.object != nil && c.bindings != nil {
		check.objectPath = flowpath.FromExprWithBindingsAt(check.object, nil, c.bindings, c.graph, point)
	}
	if check.value != nil && c.bindings != nil {
		check.valuePath = flowpath.FromExprWithBindingsAt(check.value, nil, c.bindings, c.graph, point)
	}
	return check, true
}

func (c *exhaustivenessChecker) addNonExhaustiveWarning(node ast.Expr, path string, missing []*typ.Literal) {
	pos := diag.Position{File: c.sourceName}
	span := diag.Span{}
	if node != nil {
		pos.Line = node.Line()
		pos.Column = node.Column()
		span = ast.SpanOf(node)
	}
	message := fmt.Sprintf("non-exhaustive match on %s; missing %s", path, formatMissingCases(missing))
	c.diags = append(c.diags, diag.Diagnostic{
		Severity:    diag.SeverityWarning,
		Code:        diag.ErrNonExhaustive,
		Position:    pos,
		Span:        span,
		Message:     message,
		Explanation: diag.ErrNonExhaustive.Info().Explanation,
		Help:        "Handle the missing case or add an else branch.",
	})
}

func (c *exhaustivenessChecker) addNonExhaustiveNamesWarning(node ast.Expr, path string, missing []string) {
	pos := diag.Position{File: c.sourceName}
	span := diag.Span{}
	if node != nil {
		pos.Line = node.Line()
		pos.Column = node.Column()
		span = ast.SpanOf(node)
	}
	message := fmt.Sprintf("non-exhaustive match on %s; missing %s", path, formatMissingNames(missing))
	c.diags = append(c.diags, diag.Diagnostic{
		Severity:    diag.SeverityWarning,
		Code:        diag.ErrNonExhaustive,
		Position:    pos,
		Span:        span,
		Message:     message,
		Explanation: diag.ErrNonExhaustive.Info().Explanation,
		Help:        "Handle the missing case or add an else branch.",
	})
}

func singleElseIf(stmt *ast.IfStmt) (*ast.IfStmt, bool) {
	if stmt == nil || len(stmt.Else) != 1 {
		return nil, false
	}
	next, ok := stmt.Else[0].(*ast.IfStmt)
	return next, ok
}

func equalityDiscriminantCheck(expr ast.Expr) (discriminantCheck, bool) {
	rel, ok := expr.(*ast.RelationalOpExpr)
	if !ok || rel.Operator != "==" {
		return discriminantCheck{}, false
	}
	if check, ok := attrEqualsLiteral(rel.Lhs, rel.Rhs); ok {
		return check, true
	}
	if check, ok := attrEqualsLiteral(rel.Rhs, rel.Lhs); ok {
		return check, true
	}
	if check, ok := attrEqualsPath(rel.Lhs, rel.Rhs); ok {
		return check, true
	}
	return attrEqualsPath(rel.Rhs, rel.Lhs)
}

func attrEqualsLiteral(attrExpr, literalExpr ast.Expr) (discriminantCheck, bool) {
	lit, ok := literalFromExpr(literalExpr)
	if !ok {
		return discriminantCheck{}, false
	}
	attr, ok := attrExpr.(*ast.AttrGetExpr)
	if !ok {
		return discriminantCheck{}, false
	}
	field := ast.KeyName(attr.Key)
	if field == "" {
		return discriminantCheck{}, false
	}
	objectPath, ok := exprPath(attr.Object)
	if !ok {
		return discriminantCheck{}, false
	}
	return discriminantCheck{
		object:  attr.Object,
		field:   field,
		path:    objectPath + "." + field,
		literal: lit,
	}, true
}

func attrEqualsPath(attrExpr, valueExpr ast.Expr) (discriminantCheck, bool) {
	attr, ok := attrExpr.(*ast.AttrGetExpr)
	if !ok {
		return discriminantCheck{}, false
	}
	field := ast.KeyName(attr.Key)
	if field == "" {
		return discriminantCheck{}, false
	}
	objectPath, ok := exprPath(attr.Object)
	if !ok {
		return discriminantCheck{}, false
	}
	valuePath, ok := exprPath(valueExpr)
	if !ok {
		return discriminantCheck{}, false
	}
	return discriminantCheck{
		object:    attr.Object,
		field:     field,
		path:      objectPath + "." + field,
		value:     valueExpr,
		valueName: valuePath,
	}, true
}

func literalFromExpr(expr ast.Expr) (*typ.Literal, bool) {
	switch e := expr.(type) {
	case *ast.StringExpr:
		return typ.LiteralString(e.Value), true
	case *ast.TrueExpr:
		return typ.True, true
	case *ast.FalseExpr:
		return typ.False, true
	case *ast.NumberExpr:
		if i, ok := numparse.ParseIntegerLiteral(e.Value); ok {
			return typ.LiteralInt(i), true
		}
		if f, ok := numparse.ParseFloatLiteral(e.Value); ok {
			return typ.LiteralNumber(f), true
		}
	}
	return nil, false
}

func exprPath(expr ast.Expr) (string, bool) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		if e.Value == "" {
			return "", false
		}
		return e.Value, true
	case *ast.AttrGetExpr:
		base, ok := exprPath(e.Object)
		if !ok {
			return "", false
		}
		key := ast.KeyName(e.Key)
		if key == "" {
			return "", false
		}
		return base + "." + key, true
	default:
		return "", false
	}
}

func formatMissingCases(missing []*typ.Literal) string {
	values := make([]string, 0, len(missing))
	for _, lit := range missing {
		if lit != nil {
			values = append(values, lit.String())
		}
	}
	if len(values) == 1 {
		return "case: " + values[0]
	}
	return "cases: " + strings.Join(values, ", ")
}

func formatMissingNames(missing []string) string {
	if len(missing) == 1 {
		return "case: " + missing[0]
	}
	return "cases: " + strings.Join(missing, ", ")
}

func selectCasesByResult(graph *cfg.Graph, assignments []api.AssignmentEvidence) map[string]selectCaseDomain {
	if graph == nil || graph.Bindings() == nil {
		return nil
	}
	bindings := graph.Bindings()
	domains := make(map[string]selectCaseDomain)
	for _, assign := range assignments {
		p := assign.Point
		info := assign.Info
		if info == nil {
			continue
		}
		target, ok := info.FirstTarget()
		if !ok || target.Kind != cfg.TargetIdent || target.Symbol == 0 {
			continue
		}
		call := info.SingleSourceCall()
		if !isChannelSelectCall(call) || len(call.Args) == 0 {
			continue
		}
		cases, ok := selectCaseChannels(call.Args[0], p, graph, bindings)
		if !ok || len(cases) < 2 {
			continue
		}
		resultPath := constraint.Path{Root: target.Name, Symbol: target.Symbol}
		if len(info.TargetVersions) > 0 && !info.TargetVersions[0].IsZero() {
			resultPath.Version = info.TargetVersions[0].ID
		}
		domains[pathKey(resultPath)] = selectCaseDomain{cases: cases}
	}
	return domains
}

func isChannelSelectCall(call *cfg.CallInfo) bool {
	if call == nil || call.Method != "" {
		return false
	}
	attr, ok := call.Callee.(*ast.AttrGetExpr)
	if !ok {
		return false
	}
	key := ast.KeyName(attr.Key)
	if key != "select" {
		return false
	}
	root, ok := attr.Object.(*ast.IdentExpr)
	return ok && root.Value == "channel"
}

func selectCaseChannels(expr ast.Expr, p cfg.Point, graph *cfg.Graph, bindings *bind.BindingTable) ([]selectCase, bool) {
	table, ok := expr.(*ast.TableExpr)
	if !ok {
		return nil, false
	}
	cases := make([]selectCase, 0, len(table.Fields))
	for _, field := range table.Fields {
		if field == nil {
			return nil, false
		}
		if key := ast.KeyName(field.Key); key == "default" {
			return nil, false
		}
		call, ok := field.Value.(*ast.FuncCallExpr)
		if !ok || call.Method != "case_receive" || call.Receiver == nil {
			return nil, false
		}
		casePath := flowpath.FromExprWithBindingsAt(call.Receiver, nil, bindings, graph, p)
		if casePath.IsEmpty() {
			return nil, false
		}
		name, ok := exprPath(call.Receiver)
		if !ok {
			name = casePath.String()
		}
		cases = append(cases, selectCase{path: casePath, name: name})
	}
	return cases, len(cases) > 0
}

func (d selectCaseDomain) contains(key string) bool {
	for _, c := range d.cases {
		if pathKey(c.path) == key {
			return true
		}
	}
	return false
}

func pathKey(p constraint.Path) string {
	if p.Symbol != 0 {
		return fmt.Sprintf("#%d@%d%s", p.Symbol, p.Version, constraint.FormatSegments(p.Segments))
	}
	return p.String()
}
