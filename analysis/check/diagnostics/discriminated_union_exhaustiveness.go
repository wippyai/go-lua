package diagnostics

import (
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

type discriminatedUnionExhaustiveness producerContext

type discriminantBranch struct {
	point cfg.Point
	fact  semantics.BranchConditionFact
}

type discriminantCase struct {
	index   int
	name    string
	key     string
	literal typ.Type
}

type discriminantCandidate struct {
	target  pathdom.Path
	anchor  pathdom.Path
	family  uint64
	cases   []discriminantCase
	handled []int
}

type discriminantAnchor struct {
	anchor     pathdom.Path
	anchorType typ.Type
	suffix     []segment.Segment
}

type discriminatedUnionEvidence struct {
	target   string
	possible []string
	handled  []string
	missing  []string
}

type optionalEvidence struct {
	target  string
	missing []string
	span    diagnostic.Span
}

type dispatchTableEvidence struct {
	table      string
	target     string
	possible   []string
	keys       []string
	missing    []string
	missingFor []string
	tableSpan  diagnostic.Span
	lookupSpan diagnostic.Span
}

type registrationEvidence struct {
	registry         string
	target           string
	possible         []string
	registered       []string
	missing          []string
	missingFor       []string
	registrationSpan diagnostic.Span
	dispatchSpan     diagnostic.Span
}

type resultShapeEvidence struct {
	receiver     string
	readPath     string
	discriminant string
	requiredCase string
	readSpan     diagnostic.Span
}

type dispatchTableSummary struct {
	table string
	path  pathdom.Path
	keys  map[string]bool
	span  diagnostic.Span
}

func collectDispatchTableSummaries(result *body.Result, flow *diagnosticFlowCache, inherited map[pathdom.PathKey]dispatchTableSummary) map[pathdom.PathKey]dispatchTableSummary {
	out := cloneDispatchTableSummaries(inherited)
	graph := result.Graph()
	if graph == nil {
		return out
	}
	for _, point := range graph.RPO() {
		if fact, ok := result.LocalAssignment(point); ok && fact.HasSymbol && fact.Expr != nil {
			literal, literalOK := result.ObjectLiteral(fact.Expr)
			if literalOK {
				base := pathdom.NewPath(fact.Symbol, result.SymbolName(fact.Symbol))
				collectObjectLiteralDispatchTableSummaries(result, &out, base, literal)
			}
		}
		if fact, ok := result.OrdinaryAssignment(point); ok {
			updateDispatchTableSummariesForAssignment(out, fact)
		}
		if _, ok := result.Call(point); ok {
			updateDispatchTableSummariesForCall(result, out, point)
		}
	}
	return out
}

func collectObjectLiteralDispatchTableSummaries(result *body.Result, out *map[pathdom.PathKey]dispatchTableSummary, table pathdom.Path, fact semantics.ObjectLiteralFact) {
	if result == nil || out == nil || table.IsEmpty() {
		return
	}
	if keys, ok := objectLiteralDispatchKeys(fact); ok {
		if *out == nil {
			*out = make(map[pathdom.PathKey]dispatchTableSummary, 1)
		}
		(*out)[table.Key()] = dispatchTableSummary{
			table: table.String(),
			path:  table.Clone(),
			keys:  keys,
			span:  ast.SpanOf(fact.Table),
		}
	}
	for _, entry := range fact.Entries {
		if len(entry.Suffix.Segments) == 0 {
			continue
		}
		nested, ok := result.ObjectLiteral(entry.Value)
		if !ok {
			continue
		}
		collectObjectLiteralDispatchTableSummaries(result, out, table.AppendSegments(entry.Suffix.Segments), nested)
	}
}

func cloneDispatchTableSummaries(in map[pathdom.PathKey]dispatchTableSummary) map[pathdom.PathKey]dispatchTableSummary {
	if len(in) == 0 {
		return nil
	}
	out := make(map[pathdom.PathKey]dispatchTableSummary, len(in))
	for key, summary := range in {
		summary.path = summary.path.Clone()
		summary.keys = cloneDispatchKeySet(summary.keys)
		out[key] = summary
	}
	return out
}

func cloneDispatchKeySet(in map[string]bool) map[string]bool {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]bool, len(in))
	for key, present := range in {
		out[key] = present
	}
	return out
}

func updateDispatchTableSummariesForAssignment(summaries map[pathdom.PathKey]dispatchTableSummary, fact semantics.OrdinaryAssignmentFact) {
	if len(summaries) == 0 {
		return
	}
	for summaryKey, summary := range summaries {
		if fact.HasPath {
			key, staticKey, touches := dispatchTableAssignmentKeyForPath(fact, summary.path)
			if !touches {
				continue
			}
			if !staticKey {
				delete(summaries, summaryKey)
				continue
			}
			if summary.keys == nil {
				summary.keys = make(map[string]bool, 1)
			}
			summary.keys[key] = true
			summaries[summaryKey] = summary
			continue
		}
		if fact.HasSymbol && fact.Symbol == summary.path.Symbol {
			delete(summaries, summaryKey)
			continue
		}
		if fact.HasContainerPath && pathsOverlapForInvalidation(summary.path, fact.ContainerPath) {
			delete(summaries, summaryKey)
		}
	}
}

func updateDispatchTableSummariesForCall(result *body.Result, summaries map[pathdom.PathKey]dispatchTableSummary, point cfg.Point) {
	if len(summaries) == 0 {
		return
	}
	for summaryKey, summary := range summaries {
		if callMayInvalidateTrackedPath(result, point, summary.path) {
			delete(summaries, summaryKey)
		}
	}
}

func (p discriminatedUnionExhaustiveness) Produce(result *body.Result) []diagnostic.Diagnostic {
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	branches := discriminantBranchConditions(result, graph)
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
		if diag, ok := p.chainDiagnostic(result, branch.fact.If, byIf); ok {
			out = append(out, diag)
		}
		if diag, ok := p.optionalChainDiagnostic(result, branch.fact.If, byIf); ok {
			out = append(out, diag)
		}
	}
	out = append(out, p.tableDispatchDiagnostics(result, graph)...)
	out = append(out, p.registrationDiagnostics(result, graph)...)
	out = append(out, p.resultShapeConsumptionDiagnostics(result, graph)...)
	return out
}

func discriminantBranchConditions(result *body.Result, graph cfg.Graph) []discriminantBranch {
	envs := cachedGuardEnvironments(result)
	var out []discriminantBranch
	for _, point := range graph.RPO() {
		if !guardEnvReachableAt(envs, point) {
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

func (p discriminatedUnionExhaustiveness) chainDiagnostic(result *body.Result, head *ast.IfStmt, byIf map[*ast.IfStmt]discriminantBranch) (diagnostic.Diagnostic, bool) {
	if hasDefaultElse(head) {
		return diagnostic.Diagnostic{}, false
	}
	chain := ifElseIfChain(head)
	var selected discriminantCandidate
	selectedSet := false
	handled := map[int]bool{}
	for _, stmt := range chain {
		branch, ok := byIf[stmt]
		if !ok {
			return diagnostic.Diagnostic{}, false
		}
		candidate, ok := p.candidateForCheck(result, branch.point, branch.fact.Check)
		if !ok {
			return diagnostic.Diagnostic{}, false
		}
		if !selectedSet {
			selected = candidate
			selectedSet = true
		} else if !sameDiscriminantCandidate(selected, candidate) {
			return diagnostic.Diagnostic{}, false
		}
		for _, index := range candidate.handled {
			handled[index] = true
		}
	}
	if !selectedSet || len(handled) == 0 || len(handled) >= len(selected.cases) {
		return diagnostic.Diagnostic{}, false
	}
	var possible []string
	var handledNames []string
	var missing []string
	for _, c := range selected.cases {
		possible = append(possible, c.name)
		if handled[c.index] {
			handledNames = append(handledNames, c.name)
		} else {
			missing = append(missing, c.name)
		}
	}
	if len(missing) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	span := ast.SpanOf(head.Condition)
	return newDiscriminatedUnionExhaustivenessDiagnostic(span, discriminatedUnionEvidence{
		target:   selected.target.String(),
		possible: possible,
		handled:  handledNames,
		missing:  missing,
	}), true
}

type optionalBranchCandidate struct {
	target       pathdom.Path
	handlesNil   bool
	handlesValue bool
}

func (p discriminatedUnionExhaustiveness) optionalChainDiagnostic(result *body.Result, head *ast.IfStmt, byIf map[*ast.IfStmt]discriminantBranch) (diagnostic.Diagnostic, bool) {
	if hasDefaultElse(head) {
		return diagnostic.Diagnostic{}, false
	}
	chain := ifElseIfChain(head)
	var selected pathdom.Path
	selectedSet := false
	handlesNil := false
	handlesValue := false
	consumesValue := false
	for _, stmt := range chain {
		branch, ok := byIf[stmt]
		if !ok {
			return diagnostic.Diagnostic{}, false
		}
		candidate, ok := p.optionalCandidateForCheck(result, branch.point, branch.fact.Check)
		if !ok {
			return diagnostic.Diagnostic{}, false
		}
		if !selectedSet {
			selected = candidate.target
			selectedSet = true
		} else if !selected.Equal(candidate.target) {
			return diagnostic.Diagnostic{}, false
		}
		handlesNil = handlesNil || candidate.handlesNil
		handlesValue = handlesValue || candidate.handlesValue
		if candidate.handlesValue &&
			optionalBranchConsumesPath(result, stmt.Then, candidate.target) &&
			!optionalStatementsTerminate(result, stmt.Then) {
			consumesValue = true
		}
	}
	if !selectedSet || !handlesValue || !consumesValue || handlesNil {
		return diagnostic.Diagnostic{}, false
	}
	span := ast.SpanOf(head.Condition)
	missing := []string{selected.String() + " == nil"}
	return newOptionalExhaustivenessDiagnostic(optionalEvidence{
		target:  selected.String(),
		missing: missing,
		span:    span,
	}), true
}

func (p discriminatedUnionExhaustiveness) optionalCandidateForCheck(result *body.Result, point cfg.Point, check branchcond.Check) (optionalBranchCandidate, bool) {
	if check.Path.IsEmpty() {
		return optionalBranchCandidate{}, false
	}
	t, ok := optionalPathType(result, p.resolver, point, check.Path)
	if !ok || !optionalTypeHasValue(t) {
		return optionalBranchCandidate{}, false
	}
	switch check.Kind {
	case branchcond.CheckNil:
		return optionalBranchCandidate{target: check.Path, handlesNil: true}, true
	case branchcond.CheckNotNil:
		return optionalBranchCandidate{target: check.Path, handlesValue: true}, true
	case branchcond.CheckTruthy:
		if optionalTruthyPartitionsNilValue(t) {
			return optionalBranchCandidate{target: check.Path, handlesValue: true}, true
		}
	case branchcond.CheckFalsy:
		if optionalTruthyPartitionsNilValue(t) {
			return optionalBranchCandidate{target: check.Path, handlesNil: true}, true
		}
	}
	return optionalBranchCandidate{}, false
}

func optionalPathType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, target pathdom.Path) (typ.Type, bool) {
	if target.Symbol == 0 {
		return nil, false
	}
	root := target
	root.Segments = nil
	t, ok := discriminantRootType(result, resolver, point, root)
	if !ok || t == nil {
		return nil, false
	}
	for _, seg := range target.Segments {
		next, ok := expressionSegmentType(t, seg)
		if !ok {
			return nil, false
		}
		t = next
	}
	return t, true
}

func optionalTypeHasValue(t typ.Type) bool {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) || !projectionHasNil(t) {
		return false
	}
	value := projectionWithoutNil(t)
	return value != nil && !typ.IsNever(value)
}

func optionalTruthyPartitionsNilValue(t typ.Type) bool {
	value := projectionWithoutNil(t)
	return value != nil && !typ.IsNever(value) && !typeAdmitsFalse(value)
}

func typeAdmitsFalse(t typ.Type) bool {
	switch v := t.(type) {
	case nil:
		return false
	case *typ.Alias:
		return typeAdmitsFalse(v.UnaliasedTarget())
	case *typ.Union:
		for _, member := range v.Members {
			if typeAdmitsFalse(member) {
				return true
			}
		}
		return false
	default:
		return typ.TypeEquals(t, typ.Boolean) || typ.TypeEquals(t, typ.False)
	}
}

func optionalBranchConsumesPath(result *body.Result, stmts []ast.Stmt, target pathdom.Path) bool {
	for _, stmt := range stmts {
		if optionalStmtConsumesPath(result, stmt, target) {
			return true
		}
	}
	return false
}

func optionalStatementsTerminate(result *body.Result, stmts []ast.Stmt) bool {
	for _, stmt := range stmts {
		if optionalStmtTerminates(result, stmt) {
			return true
		}
	}
	return false
}

func optionalStmtTerminates(result *body.Result, stmt ast.Stmt) bool {
	switch s := stmt.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.FuncCallStmt:
		return optionalNoReturnCall(result, s.Expr)
	case *ast.DoBlockStmt:
		return optionalStatementsTerminate(result, s.Stmts)
	case *ast.IfStmt:
		return len(s.Else) != 0 &&
			optionalStatementsTerminate(result, s.Then) &&
			optionalStatementsTerminate(result, s.Else)
	default:
		return false
	}
}

func optionalNoReturnCall(result *body.Result, expr ast.Expr) bool {
	call, ok := expr.(*ast.FuncCallExpr)
	if !ok || call.Receiver != nil || call.Method != "" {
		return false
	}
	fn, ok := call.Func.(*ast.IdentExpr)
	return ok && result.IdentResolvesToGlobal(fn, "error")
}

func optionalStmtConsumesPath(result *body.Result, stmt ast.Stmt, target pathdom.Path) bool {
	switch s := stmt.(type) {
	case *ast.LocalAssignStmt:
		return optionalExprsConsumePath(result, s.Exprs, target)
	case *ast.AssignStmt:
		if optionalExprsConsumePath(result, s.Rhs, target) {
			return true
		}
		for _, lhs := range s.Lhs {
			if optionalLValueConsumesPath(result, lhs, target) {
				return true
			}
		}
	case *ast.FuncCallStmt:
		return optionalExprConsumesPath(result, s.Expr, target)
	case *ast.ReturnStmt:
		return optionalExprsConsumePath(result, s.Exprs, target)
	case *ast.DoBlockStmt:
		return optionalBranchConsumesPath(result, s.Stmts, target)
	case *ast.IfStmt:
		return optionalBranchConsumesPath(result, s.Then, target) || optionalBranchConsumesPath(result, s.Else, target)
	case *ast.WhileStmt:
		return optionalBranchConsumesPath(result, s.Stmts, target)
	case *ast.RepeatStmt:
		return optionalBranchConsumesPath(result, s.Stmts, target)
	case *ast.NumberForStmt:
		return optionalExprConsumesPath(result, s.Init, target) ||
			optionalExprConsumesPath(result, s.Limit, target) ||
			optionalExprConsumesPath(result, s.Step, target) ||
			optionalBranchConsumesPath(result, s.Stmts, target)
	case *ast.GenericForStmt:
		return optionalExprsConsumePath(result, s.Exprs, target) ||
			optionalBranchConsumesPath(result, s.Stmts, target)
	case *ast.FuncDefStmt:
		if s.Name == nil {
			return false
		}
		return optionalLValueConsumesPath(result, s.Name.Func, target) ||
			optionalExprConsumesPath(result, s.Name.Receiver, target)
	}
	return false
}

func optionalExprsConsumePath(result *body.Result, exprs []ast.Expr, target pathdom.Path) bool {
	for _, expr := range exprs {
		if optionalExprConsumesPath(result, expr, target) {
			return true
		}
	}
	return false
}

func optionalLValueConsumesPath(result *body.Result, expr ast.Expr, target pathdom.Path) bool {
	switch e := expr.(type) {
	case nil:
		return false
	case *ast.IdentExpr:
		return false
	case *ast.AttrGetExpr:
		return optionalExprConsumesPath(result, e.Object, target) ||
			(e.KeySyntax == ast.AttrKeyIndex && optionalExprConsumesPath(result, e.Key, target))
	case *ast.CastExpr:
		return optionalLValueConsumesPath(result, e.Expr, target)
	case *ast.NonNilAssertExpr:
		return optionalLValueConsumesPath(result, e.Expr, target)
	default:
		return optionalExprConsumesPath(result, expr, target)
	}
}

func optionalExprConsumesPath(result *body.Result, expr ast.Expr, target pathdom.Path) bool {
	if expr == nil || target.IsEmpty() {
		return false
	}
	if p, ok := result.ExpressionPath(expr); ok && pathHasPrefix(p, target) {
		return true
	}
	consumes := false
	walkExprChildren(expr, func(child ast.Expr) {
		if consumes {
			return
		}
		if optionalExprConsumesPath(result, child, target) {
			consumes = true
		}
	})
	return consumes
}

func (p discriminatedUnionExhaustiveness) candidateForCheck(result *body.Result, point cfg.Point, check branchcond.Check) (discriminantCandidate, bool) {
	lit, negate, ok := discriminantCheckLiteral(check)
	if !ok {
		return discriminantCandidate{}, false
	}
	for _, anchor := range p.discriminantAnchors(result, point, check.Path) {
		family, handled, ok := discriminantOriginByCheck(anchor.anchorType, anchor.suffix, lit, negate)
		if !ok {
			continue
		}
		caseFamily, cases, ok := variant.OriginCasesOfType(anchor.anchorType)
		if !ok || caseFamily != family || len(cases) < 2 {
			continue
		}
		return discriminantCandidate{
			target:  check.Path,
			anchor:  anchor.anchor,
			family:  family,
			cases:   discriminantCasesFor(check.Path, anchor.suffix, cases),
			handled: handled,
		}, true
	}
	return discriminantCandidate{}, false
}

func (p discriminatedUnionExhaustiveness) discriminantAnchors(result *body.Result, point cfg.Point, target pathdom.Path) []discriminantAnchor {
	if target.Symbol == 0 || len(target.Segments) == 0 {
		return nil
	}
	root := target
	root.Segments = nil
	rootType, ok := discriminantRootType(result, p.resolver, point, root)
	if !ok {
		return nil
	}
	segments := target.Segments
	out := make([]discriminantAnchor, 0, len(segments))
	for prefixLen := 0; prefixLen < len(segments); prefixLen++ {
		prefix := segments[:prefixLen]
		suffix := segments[prefixLen:]
		anchorType := rootType
		if len(prefix) > 0 {
			var fieldOK bool
			anchorType, fieldOK = variant.FieldAtPath(rootType, prefix)
			if !fieldOK {
				continue
			}
		}
		anchorPath := root
		anchorPath.Segments = append([]segment.Segment(nil), prefix...)
		out = append(out, discriminantAnchor{
			anchor:     anchorPath,
			anchorType: anchorType,
			suffix:     append([]segment.Segment(nil), suffix...),
		})
	}
	return out
}

func discriminantCheckLiteral(check branchcond.Check) (typ.Type, bool, bool) {
	switch check.Kind {
	case branchcond.CheckLiteralEqual:
		lit, ok := check.LiteralValue()
		return lit, false, ok
	case branchcond.CheckLiteralNot:
		lit, ok := check.LiteralValue()
		return lit, true, ok
	case branchcond.CheckTruthy:
		return typ.True, false, true
	case branchcond.CheckFalsy:
		return typ.True, true, true
	default:
		return nil, false, false
	}
}

func discriminantOriginByCheck(anchorType typ.Type, rest []segment.Segment, lit typ.Type, negate bool) (uint64, []int, bool) {
	if negate {
		return variant.OriginByPathLiteralNot(anchorType, rest, lit)
	}
	return variant.OriginByPathLiteral(anchorType, rest, lit)
}

func discriminantRootType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, root pathdom.Path) (typ.Type, bool) {
	if result == nil || root.Symbol == 0 {
		return nil, false
	}
	if annotation, ok := result.SymbolTypeAnnotation(root.Symbol); ok {
		lowered, loweredOK := lowerType(annotation, resolver)
		if !loweredOK {
			return nil, false
		}
		return transparentComparableType(result, lowered), true
	}
	value, ok := result.SymbolValueAtBoundary(point, root.Symbol)
	if !ok {
		return nil, false
	}
	return readmodel.New(result).FullVariantOriginType(value)
}

func discriminantCasesFor(target pathdom.Path, suffix []segment.Segment, cases []variant.OriginCase) []discriminantCase {
	out := make([]discriminantCase, 0, len(cases))
	for _, c := range cases {
		out = append(out, discriminantCase{
			index:   c.Index,
			name:    discriminantCaseName(target, suffix, c.Type),
			literal: discriminantCaseLiteralType(c.Type, suffix),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].name != out[j].name {
			return out[i].name < out[j].name
		}
		return out[i].index < out[j].index
	})
	return out
}

func discriminantCaseName(target pathdom.Path, suffix []segment.Segment, caseType typ.Type) string {
	if field, ok := variant.FieldAtPath(caseType, suffix); ok {
		return target.String() + " == " + formatType(field)
	}
	return formatType(caseType)
}

func sameDiscriminantCandidate(a, b discriminantCandidate) bool {
	return a.family == b.family && a.target.Equal(b.target) && a.anchor.Equal(b.anchor)
}

type resultShapeRead struct {
	point        cfg.Point
	expr         *ast.AttrGetExpr
	receiverExpr ast.Expr
	receiver     pathdom.Path
	readPath     pathdom.Path
	discriminant pathdom.Path
	required     discriminantCase
}

func (p discriminatedUnionExhaustiveness) resultShapeConsumptionDiagnostics(result *body.Result, graph cfg.Graph) []diagnostic.Diagnostic {
	envs := cachedGuardEnvironments(result)
	var out []diagnostic.Diagnostic
	seen := make(map[*ast.AttrGetExpr]struct{})
	for _, point := range graph.RPO() {
		if !guardEnvReachableAt(envs, point) {
			continue
		}
		emit := func(expr ast.Expr) {
			for _, read := range p.resultShapeReadsInExpr(result, point, expr, seen) {
				if resultShapeRequiredCaseProven(envs[point], read.discriminant, read.required) {
					continue
				}
				if p.resultShapeCurrentTypeProvesRequired(result, point, envs[point], read.receiverExpr, read.required) {
					continue
				}
				if resultShapeOtherCaseProven(envs[point], read.discriminant, read.required) {
					continue
				}
				out = append(out, newResultShapeExhaustivenessDiagnostic(resultShapeEvidence{
					receiver:     read.receiver.String(),
					readPath:     read.readPath.String(),
					discriminant: read.discriminant.String(),
					requiredCase: read.required.name,
					readSpan:     ast.SpanOf(read.expr),
				}))
			}
		}
		if fact, ok := result.LocalAssignment(point); ok {
			emit(fact.Expr)
		}
		if fact, ok := result.OrdinaryAssignment(point); ok {
			emitAssignmentTargetReads(fact.Target, emit)
			emit(fact.Value)
		}
		if fact, ok := result.Call(point); ok {
			emit(fact.Call)
		}
		if fact, ok := result.ReturnFact(point); ok {
			for _, expr := range fact.Exprs {
				emit(expr)
			}
		}
		if fact, ok := result.BranchCondition(point); ok {
			emit(fact.Condition)
		}
	}
	return out
}

func (p discriminatedUnionExhaustiveness) resultShapeReadsInExpr(result *body.Result, point cfg.Point, expr ast.Expr, seen map[*ast.AttrGetExpr]struct{}) []resultShapeRead {
	var out []resultShapeRead
	p.walkResultShapeReads(result, point, expr, seen, &out, 0)
	return out
}

func (p discriminatedUnionExhaustiveness) walkResultShapeReads(result *body.Result, point cfg.Point, expr ast.Expr, seen map[*ast.AttrGetExpr]struct{}, out *[]resultShapeRead, depth int) {
	if expr == nil || depth > typ.DefaultRecursionDepth {
		return
	}
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		p.walkResultShapeReads(result, point, e.Object, seen, out, depth+1)
		if e.KeySyntax == ast.AttrKeyIndex {
			p.walkResultShapeReads(result, point, e.Key, seen, out, depth+1)
		}
		if _, done := seen[e]; done {
			return
		}
		seen[e] = struct{}{}
		if read, ok := p.resultShapeRead(result, point, e); ok {
			*out = append(*out, read)
		}
	case *ast.FuncCallExpr:
		p.walkResultShapeReads(result, point, e.Func, seen, out, depth+1)
		p.walkResultShapeReads(result, point, e.Receiver, seen, out, depth+1)
		for _, arg := range e.Args {
			p.walkResultShapeReads(result, point, arg, seen, out, depth+1)
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax == ast.AttrKeyIndex {
				p.walkResultShapeReads(result, point, field.Key, seen, out, depth+1)
			}
			p.walkResultShapeReads(result, point, field.Value, seen, out, depth+1)
		}
	case *ast.LogicalOpExpr:
		p.walkResultShapeReads(result, point, e.Lhs, seen, out, depth+1)
		p.walkResultShapeReads(result, point, e.Rhs, seen, out, depth+1)
	case *ast.RelationalOpExpr:
		p.walkResultShapeReads(result, point, e.Lhs, seen, out, depth+1)
		p.walkResultShapeReads(result, point, e.Rhs, seen, out, depth+1)
	case *ast.StringConcatOpExpr:
		p.walkResultShapeReads(result, point, e.Lhs, seen, out, depth+1)
		p.walkResultShapeReads(result, point, e.Rhs, seen, out, depth+1)
	case *ast.ArithmeticOpExpr:
		p.walkResultShapeReads(result, point, e.Lhs, seen, out, depth+1)
		p.walkResultShapeReads(result, point, e.Rhs, seen, out, depth+1)
	case *ast.UnaryMinusOpExpr:
		p.walkResultShapeReads(result, point, e.Expr, seen, out, depth+1)
	case *ast.UnaryNotOpExpr:
		p.walkResultShapeReads(result, point, e.Expr, seen, out, depth+1)
	case *ast.UnaryLenOpExpr:
		p.walkResultShapeReads(result, point, e.Expr, seen, out, depth+1)
	case *ast.UnaryBNotOpExpr:
		p.walkResultShapeReads(result, point, e.Expr, seen, out, depth+1)
	case *ast.CastExpr:
		p.walkResultShapeReads(result, point, e.Expr, seen, out, depth+1)
	case *ast.NonNilAssertExpr:
		p.walkResultShapeReads(result, point, e.Expr, seen, out, depth+1)
	}
}

func (p discriminatedUnionExhaustiveness) resultShapeRead(result *body.Result, point cfg.Point, expr *ast.AttrGetExpr) (resultShapeRead, bool) {
	member, ok := staticMemberReadName(expr)
	if !ok || member == "ok" {
		return resultShapeRead{}, false
	}
	receiverPath, ok := result.ExpressionPath(expr.Object)
	if !ok || receiverPath.Symbol == 0 {
		return resultShapeRead{}, false
	}
	readPath, ok := result.ExpressionPath(expr)
	if !ok || readPath.Symbol == 0 {
		return resultShapeRead{}, false
	}
	receiverType, ok := newStructuralFlowExpressionTyper(result, p.resolver, point, guardEnv{}).broadType(expr.Object)
	if !ok {
		return resultShapeRead{}, false
	}
	discriminant, required, ok := resultShapeRequiredCaseForMember(receiverPath, receiverType, member)
	if !ok {
		return resultShapeRead{}, false
	}
	return resultShapeRead{
		point:        point,
		expr:         expr,
		receiverExpr: expr.Object,
		receiver:     receiverPath,
		readPath:     readPath,
		discriminant: discriminant,
		required:     required,
	}, true
}

func resultShapeRequiredCaseForMember(receiver pathdom.Path, receiverType typ.Type, member string) (pathdom.Path, discriminantCase, bool) {
	_, cases, ok := variant.OriginCasesOfType(receiverType)
	if !ok || len(cases) != 2 {
		return pathdom.Path{}, discriminantCase{}, false
	}
	okSuffix := []segment.Segment{{Kind: segment.SegmentField, Name: "ok"}}
	discriminant := receiver
	discriminant.Segments = append(append([]segment.Segment(nil), receiver.Segments...), okSuffix...)
	domainCases, ok := booleanDiscriminantCasesFor(discriminant, okSuffix, cases)
	if !ok || len(domainCases) != 2 {
		return pathdom.Path{}, discriminantCase{}, false
	}
	var required []discriminantCase
	for _, c := range domainCases {
		if caseType, ok := originCaseTypeByIndex(cases, c.index); ok {
			if _, ok := access.Field(caseType, member); ok {
				required = append(required, c)
			}
		}
	}
	if len(required) != 1 {
		return pathdom.Path{}, discriminantCase{}, false
	}
	return discriminant, required[0], true
}

func (p discriminatedUnionExhaustiveness) resultShapeCurrentTypeProvesRequired(result *body.Result, point cfg.Point, env guardEnv, expr ast.Expr, required discriminantCase) bool {
	if required.literal == nil {
		return false
	}
	current, ok := newStructuralFlowExpressionTyper(result, p.resolver, point, env).typeOf(expr)
	if !ok || current == nil {
		return false
	}
	field, ok := variant.FieldAtPath(current, []segment.Segment{{Kind: segment.SegmentField, Name: "ok"}})
	return ok && typ.TypeEquals(field, required.literal)
}

func booleanDiscriminantCasesFor(target pathdom.Path, suffix []segment.Segment, cases []variant.OriginCase) ([]discriminantCase, bool) {
	out := make([]discriminantCase, 0, len(cases))
	seen := make(map[bool]struct{}, len(cases))
	for _, c := range cases {
		lit, ok := discriminantCaseLiteral(c.Type, suffix)
		if !ok {
			return nil, false
		}
		value, ok := lit.Value.(bool)
		if !ok {
			return nil, false
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, false
		}
		seen[value] = struct{}{}
		out = append(out, discriminantCase{
			index:   c.Index,
			name:    discriminantCaseName(target, suffix, c.Type),
			literal: lit,
		})
	}
	if _, ok := seen[true]; !ok {
		return nil, false
	}
	if _, ok := seen[false]; !ok {
		return nil, false
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].name != out[j].name {
			return out[i].name < out[j].name
		}
		return out[i].index < out[j].index
	})
	return out, true
}

func originCaseTypeByIndex(cases []variant.OriginCase, index int) (typ.Type, bool) {
	for _, c := range cases {
		if c.Index == index {
			return c.Type, true
		}
	}
	return nil, false
}

func discriminantCaseLiteralType(caseType typ.Type, suffix []segment.Segment) typ.Type {
	lit, _ := discriminantCaseLiteral(caseType, suffix)
	return lit
}

func discriminantCaseLiteral(caseType typ.Type, suffix []segment.Segment) (*typ.Literal, bool) {
	field, ok := variant.FieldAtPath(caseType, suffix)
	if !ok {
		return nil, false
	}
	lit, ok := field.(*typ.Literal)
	return lit, ok
}

func resultShapeRequiredCaseProven(env guardEnv, discriminant pathdom.Path, required discriminantCase) bool {
	if required.literal == nil {
		return false
	}
	return guardEnvProvesLiteral(env, discriminant, required.literal)
}

func resultShapeOtherCaseProven(env guardEnv, discriminant pathdom.Path, required discriminantCase) bool {
	if required.literal == nil {
		return false
	}
	switch required.literal {
	case typ.True:
		return guardEnvProvesLiteral(env, discriminant, typ.False)
	case typ.False:
		return guardEnvProvesLiteral(env, discriminant, typ.True)
	default:
		return false
	}
}

func guardEnvProvesLiteral(env guardEnv, target pathdom.Path, lit typ.Type) bool {
	if typ.TypeEquals(lit, typ.True) && env.hasTruthy(target) {
		return true
	}
	if typ.TypeEquals(lit, typ.False) && env.hasFalsy(target) {
		return true
	}
	for _, c := range env.constraints {
		if !c.target.Equal(target) {
			continue
		}
		if !c.negated && typ.TypeEquals(c.value, lit) {
			return true
		}
		if c.negated && typ.TypeEquals(lit, typ.True) && typ.TypeEquals(c.value, typ.False) {
			return true
		}
		if c.negated && typ.TypeEquals(lit, typ.False) && typ.TypeEquals(c.value, typ.True) {
			return true
		}
	}
	return false
}

type registrationCall struct {
	point    cfg.Point
	call     *ast.FuncCallExpr
	registry pathdom.Path
	key      string
	span     diagnostic.Span
}

type openRegistrationMutation struct {
	point          cfg.Point
	path           pathdom.Path
	registry       pathdom.Path
	key            string
	hasKey         bool
	opensAll       bool
	aliasSensitive bool
}

type dispatchCall struct {
	point        cfg.Point
	call         *ast.FuncCallExpr
	registry     pathdom.Path
	discriminant pathdom.Path
	cases        []discriminantCase
	span         diagnostic.Span
}

func (p discriminatedUnionExhaustiveness) registrationDiagnostics(result *body.Result, graph cfg.Graph) []diagnostic.Diagnostic {
	registrations, openRegistries := p.registrationCalls(result, graph)
	if len(registrations) == 0 {
		return nil
	}
	var out []diagnostic.Diagnostic
	for _, dispatch := range p.dispatchCalls(result, graph) {
		if p.openRegistrationCanReach(result, graph, openRegistries, dispatch) {
			continue
		}
		seen := make(map[string]registrationCall)
		for _, reg := range registrations {
			if !registrationRegistryMatchesAt(result, reg.point, reg.registry, dispatch.registry) ||
				!diagnosticCanReach(p.flow, graph, reg.point, dispatch.point) {
				continue
			}
			if existing, ok := seen[reg.key]; ok && existing.point > reg.point {
				continue
			}
			seen[reg.key] = reg
		}
		if len(seen) == 0 {
			continue
		}
		if diag, ok := registrationExhaustivenessDiagnosticFor(dispatch, seen); ok {
			out = append(out, diag)
		}
	}
	return out
}

func registrationExhaustivenessDiagnosticFor(dispatch dispatchCall, registrations map[string]registrationCall) (diagnostic.Diagnostic, bool) {
	var possible []string
	var registered []string
	var missing []string
	var missingFor []string
	matched := false
	for _, c := range dispatch.cases {
		possible = append(possible, c.name)
		if _, ok := registrations[c.key]; ok {
			matched = true
			registered = append(registered, registrationCaseName(dispatch.registry.String(), c.key))
			continue
		}
		missing = append(missing, registrationCaseName(dispatch.registry.String(), c.key))
		missingFor = append(missingFor, c.name)
	}
	if !matched || len(missing) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	sort.Strings(registered)
	regSpan := firstRegistrationSpan(registrations)
	return newRegistrationExhaustivenessDiagnostic(registrationEvidence{
		registry:         dispatch.registry.String(),
		target:           dispatch.discriminant.String(),
		possible:         possible,
		registered:       registered,
		missing:          missing,
		missingFor:       missingFor,
		registrationSpan: regSpan,
		dispatchSpan:     dispatch.span,
	}), true
}

func firstRegistrationSpan(registrations map[string]registrationCall) diagnostic.Span {
	var span diagnostic.Span
	var point cfg.Point
	first := true
	for _, reg := range registrations {
		if first || reg.point < point {
			span = reg.span
			point = reg.point
			first = false
		}
	}
	return span
}

func (p discriminatedUnionExhaustiveness) registrationCalls(result *body.Result, graph cfg.Graph) ([]registrationCall, []openRegistrationMutation) {
	var registrations []registrationCall
	var open []openRegistrationMutation
	for _, point := range graph.RPO() {
		call, ok := result.Call(point)
		if !ok || call.Call == nil {
			if assignment, ok := result.OrdinaryAssignment(point); ok {
				if mutation, ok := openRegistrationAssignment(assignment); ok {
					mutation.point = point
					open = append(open, mutation)
				}
			}
			continue
		}
		if reg, ok := registrationCallFromFact(result, call, point); ok {
			registrations = append(registrations, reg)
			continue
		}
		if mutation, ok := openRegistrationMutationFromFact(result, point, call); ok {
			mutation.point = point
			open = append(open, mutation)
		}
	}
	return registrations, open
}

func (p discriminatedUnionExhaustiveness) openRegistrationCanReach(result *body.Result, graph cfg.Graph, open []openRegistrationMutation, dispatch dispatchCall) bool {
	for _, mutation := range open {
		if mutation.point == dispatch.point || !diagnosticCanReach(p.flow, graph, mutation.point, dispatch.point) {
			continue
		}
		if mutation.opensAll {
			if pathsOverlapForInvalidation(mutation.path, dispatch.registry) {
				return true
			}
			if mutation.aliasSensitive && registrationRegistryMatchesAt(result, mutation.point, mutation.path, dispatch.registry) {
				return true
			}
			continue
		}
		if pathHasPrefix(dispatch.registry, mutation.path) {
			return true
		}
		if mutation.hasKey &&
			registrationRegistryMatchesAt(result, mutation.point, mutation.registry, dispatch.registry) &&
			registrationMutationKeyMatchesCase(mutation.key, dispatch.cases) {
			return true
		}
	}
	return false
}

func registrationRegistryMatchesAt(result *body.Result, point cfg.Point, left, right pathdom.Path) bool {
	if left.Equal(right) {
		return true
	}
	return result != nil && result.PathsEquivalentAtBoundary(point, left, right)
}

func registrationMutationKeyMatchesCase(key string, cases []discriminantCase) bool {
	for _, c := range cases {
		if c.key == key {
			return true
		}
	}
	return false
}

func openRegistrationAssignment(fact semantics.OrdinaryAssignmentFact) (openRegistrationMutation, bool) {
	if fact.HasPath && fact.Path.Symbol != 0 {
		mutation := openRegistrationMutation{path: fact.Path, aliasSensitive: true}
		if key, ok := fact.Path.DirectFieldName(); ok {
			mutation.registry = pathdom.Path{Root: fact.Path.Root, Symbol: fact.Path.Symbol, Version: fact.Path.Version}
			mutation.key = key
			mutation.hasKey = true
			return mutation, true
		}
		if seg, ok := fact.Path.LastSegment(); ok {
			if key, keyOK := segmentStringKey(seg); keyOK {
				mutation.registry = fact.Path.Parent()
				mutation.key = key
				mutation.hasKey = true
				return mutation, true
			}
		}
		mutation.opensAll = true
		return mutation, true
	}
	if fact.HasContainerPath && fact.ContainerPath.Symbol != 0 {
		return openRegistrationMutation{path: fact.ContainerPath, opensAll: true, aliasSensitive: true}, true
	}
	if fact.HasSymbol && fact.Symbol != 0 {
		return openRegistrationMutation{path: pathdom.Path{Symbol: fact.Symbol}, opensAll: true}, true
	}
	return openRegistrationMutation{}, false
}

func registrationCallFromFact(result *body.Result, fact semantics.CallFact, point cfg.Point) (registrationCall, bool) {
	registry, keyIndex, ok := registrationRegistryAndKeyIndex(result, fact)
	if !ok || keyIndex < 0 || keyIndex >= len(fact.Args)-1 {
		return registrationCall{}, false
	}
	key, ok := stringLiteralExprValue(fact.Args[keyIndex])
	if !ok || !registrationCallbackExpr(result, point, fact.Args[keyIndex+1]) {
		return registrationCall{}, false
	}
	return registrationCall{
		point:    point,
		call:     fact.Call,
		registry: registry,
		key:      key,
		span:     ast.SpanOf(fact.Call),
	}, true
}

func registrationRegistryAndKeyIndex(result *body.Result, fact semantics.CallFact) (pathdom.Path, int, bool) {
	if fact.Call == nil {
		return pathdom.Path{}, 0, false
	}
	if fact.HasReceiverPath && fact.Method != "" && len(fact.Args) >= 2 {
		return fact.ReceiverPath, 0, true
	}
	if len(fact.Args) >= 3 {
		if registry, ok := result.ExpressionPath(fact.Args[0]); ok && registry.Symbol != 0 {
			return registry, 1, true
		}
	}
	return pathdom.Path{}, 0, false
}

func openRegistrationMutationFromFact(result *body.Result, point cfg.Point, fact semantics.CallFact) (openRegistrationMutation, bool) {
	registry, keyIndex, ok := registrationRegistryAndKeyIndex(result, fact)
	if ok && keyIndex >= 0 && keyIndex < len(fact.Args)-1 {
		if _, ok := stringLiteralExprValue(fact.Args[keyIndex]); ok && registrationCallbackExpr(result, point, fact.Args[keyIndex+1]) {
			return openRegistrationMutation{}, false
		}
		if registrationCallbackExpr(result, point, fact.Args[keyIndex+1]) {
			return openRegistrationMutation{path: registry, opensAll: true, aliasSensitive: true}, true
		}
	}
	if fact.HasReceiverPath && callMayInvalidateTrackedPath(result, point, fact.ReceiverPath) {
		return openRegistrationMutation{path: fact.ReceiverPath, opensAll: true, aliasSensitive: true}, true
	}
	for _, arg := range fact.Args {
		argPath, ok := result.ExpressionPath(arg)
		if ok && callMayInvalidateTrackedPath(result, point, argPath) {
			return openRegistrationMutation{path: argPath, opensAll: true, aliasSensitive: true}, true
		}
	}
	return openRegistrationMutation{}, false
}

func registrationCallbackExpr(result *body.Result, point cfg.Point, expr ast.Expr) bool {
	if _, ok := directFunctionExprFromExpr(expr); ok {
		return true
	}
	if _, ok := result.FunctionValueTypeAtBoundary(point, expr); ok {
		return true
	}
	path, ok := result.ExpressionPath(expr)
	if !ok || path.IsEmpty() {
		return false
	}
	return registrationCallbackPathExpr(result, point, path, nil)
}

func registrationCallbackPathExpr(result *body.Result, point cfg.Point, target pathdom.Path, seen map[pathdom.PathKey]struct{}) bool {
	graph := result.Graph()
	if graph == nil || target.IsEmpty() {
		return false
	}
	key := target.Key()
	if _, ok := seen[key]; ok {
		return false
	}
	if seen == nil {
		seen = make(map[pathdom.PathKey]struct{}, 1)
	}
	seen[key] = struct{}{}
	if dominatingFunctionDefinitionForPath(result, point, target) != nil {
		return true
	}
	idom := dominance.ComputeImmediateDominatorInfo(graph).Map()
	visited := make(map[cfg.Point]struct{}, graph.Size())
	for cursor := point; ; {
		if _, ok := visited[cursor]; ok {
			return false
		}
		visited[cursor] = struct{}{}
		if fact, ok := result.LocalAssignment(cursor); ok &&
			len(target.Segments) == 0 &&
			fact.HasSymbol &&
			fact.Symbol == target.Symbol {
			return registrationCallbackSourceExpr(result, cursor, fact.Expr, seen)
		}
		if fact, ok := result.OrdinaryAssignment(cursor); ok {
			if len(target.Segments) == 0 &&
				fact.HasSymbol &&
				fact.Symbol == target.Symbol {
				return registrationCallbackSourceExpr(result, cursor, fact.Value, seen)
			}
			if fact.HasPath && fact.Path.Equal(target) {
				return registrationCallbackSourceExpr(result, cursor, fact.Value, seen)
			}
			if fact.HasPath && pathHasPrefix(target, fact.Path) {
				return false
			}
		}
		parent, ok := idom[cursor]
		if !ok || parent == cursor {
			return false
		}
		cursor = parent
	}
}

func registrationCallbackSourceExpr(result *body.Result, point cfg.Point, expr ast.Expr, seen map[pathdom.PathKey]struct{}) bool {
	if _, ok := directFunctionExprFromExpr(expr); ok {
		return true
	}
	if _, ok := result.FunctionValueTypeAtBoundary(point, expr); ok {
		return true
	}
	path, ok := result.ExpressionPath(expr)
	if !ok || path.IsEmpty() {
		return false
	}
	return registrationCallbackPathExpr(result, point, path, seen)
}

func (p discriminatedUnionExhaustiveness) dispatchCalls(result *body.Result, graph cfg.Graph) []dispatchCall {
	var out []dispatchCall
	for _, point := range graph.RPO() {
		fact, ok := result.Call(point)
		if !ok || fact.Call == nil || isRegistrationLikeCall(result, fact) {
			continue
		}
		registry, args, ok := dispatchRegistryAndArgs(result, fact)
		if !ok {
			continue
		}
		for _, arg := range args {
			argPath, ok := result.ExpressionPath(arg)
			if !ok || argPath.Symbol == 0 {
				continue
			}
			discriminant, cases, ok := p.stringDiscriminantCasesForArgument(result, point, argPath)
			if !ok {
				continue
			}
			out = append(out, dispatchCall{
				point:        point,
				call:         fact.Call,
				registry:     registry,
				discriminant: discriminant,
				cases:        cases,
				span:         ast.SpanOf(fact.Call),
			})
			break
		}
	}
	return out
}

func isRegistrationLikeCall(result *body.Result, fact semantics.CallFact) bool {
	if _, ok := registrationCallFromFact(result, fact, 0); ok {
		return true
	}
	if _, ok := openRegistrationMutationFromFact(result, 0, fact); ok {
		return true
	}
	return false
}

func dispatchRegistryAndArgs(result *body.Result, fact semantics.CallFact) (pathdom.Path, []ast.Expr, bool) {
	if fact.HasReceiverPath && fact.Method != "" && len(fact.Args) > 0 {
		return fact.ReceiverPath, fact.Args, true
	}
	if len(fact.Args) >= 2 {
		registry, ok := result.ExpressionPath(fact.Args[0])
		if ok && registry.Symbol != 0 {
			return registry, fact.Args[1:], true
		}
	}
	return pathdom.Path{}, nil, false
}

func (p discriminatedUnionExhaustiveness) stringDiscriminantCasesForArgument(result *body.Result, point cfg.Point, argPath pathdom.Path) (pathdom.Path, []discriminantCase, bool) {
	if len(argPath.Segments) > 0 {
		cases, ok := p.stringDiscriminantCases(result, point, argPath)
		return argPath, cases, ok
	}
	for _, domain := range p.stringDiscriminantDomainsForRoot(result, point, argPath) {
		return domain.target, domain.cases, true
	}
	return pathdom.Path{}, nil, false
}

type stringDiscriminantDomain struct {
	target pathdom.Path
	cases  []discriminantCase
}

func (p discriminatedUnionExhaustiveness) stringDiscriminantDomainsForRoot(result *body.Result, point cfg.Point, root pathdom.Path) []stringDiscriminantDomain {
	rootType, ok := discriminantRootType(result, p.resolver, point, root)
	if !ok {
		return nil
	}
	out := stringDiscriminantDomainsForType(root, nil, rootType, 0)
	sort.Slice(out, func(i, j int) bool {
		return out[i].target.String() < out[j].target.String()
	})
	return out
}

func stringDiscriminantDomainsForType(root pathdom.Path, prefix []segment.Segment, t typ.Type, depth int) []stringDiscriminantDomain {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return nil
	}
	if _, cases, ok := variant.OriginCasesOfType(t); ok && len(cases) >= 2 {
		return stringDiscriminantDomainsForCases(root, prefix, cases)
	}
	var out []stringDiscriminantDomain
	for _, child := range staticDiscriminantChildren(t, depth) {
		nextPrefix := appendSegment(prefix, child.segment)
		out = append(out, stringDiscriminantDomainsForType(root, nextPrefix, child.typ, depth+1)...)
	}
	return out
}

func stringDiscriminantDomainsForCases(root pathdom.Path, prefix []segment.Segment, cases []variant.OriginCase) []stringDiscriminantDomain {
	var out []stringDiscriminantDomain
	for _, suffix := range stringLiteralSuffixes(cases[0].Type, nil, 0) {
		target := root.AppendSegments(prefix).AppendSegments(suffix)
		domainCases, ok := stringDiscriminantCasesFor(target, suffix, cases)
		if !ok {
			continue
		}
		out = append(out, stringDiscriminantDomain{target: target, cases: domainCases})
	}
	return out
}

type staticDiscriminantChild struct {
	segment segment.Segment
	typ     typ.Type
}

func staticDiscriminantChildren(t typ.Type, depth int) []staticDiscriminantChild {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return nil
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Alias:
		return staticDiscriminantChildren(v.UnaliasedTarget(), depth+1)
	case *typ.Optional:
		return staticDiscriminantChildren(v.Inner, depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return nil
		}
		return staticDiscriminantChildren(v.Body, depth+1)
	case *typ.Instantiated:
		expanded, ok := subst.ExpandInstantiatedChanged(v)
		if !ok {
			return nil
		}
		return staticDiscriminantChildren(expanded, depth+1)
	case *typ.Record:
		out := make([]staticDiscriminantChild, 0, len(v.Fields)+len(v.StaticMembers))
		for _, field := range v.Fields {
			out = append(out, staticDiscriminantChild{
				segment: segment.Segment{Kind: segment.SegmentField, Name: field.Name},
				typ:     field.Type,
			})
		}
		for _, member := range v.StaticMembers {
			if member.Kind != typ.StaticMemberStringIndex {
				continue
			}
			out = append(out, staticDiscriminantChild{
				segment: segment.Segment{Kind: segment.SegmentIndexString, Name: member.Name},
				typ:     member.Type,
			})
		}
		return out
	default:
		return nil
	}
}

func appendSegment(prefix []segment.Segment, seg segment.Segment) []segment.Segment {
	next := make([]segment.Segment, 0, len(prefix)+1)
	next = append(next, prefix...)
	next = append(next, seg)
	return next
}

func stringLiteralSuffixes(t typ.Type, prefix []segment.Segment, depth int) [][]segment.Segment {
	if t == nil || depth > 2 {
		return nil
	}
	switch v := t.(type) {
	case *typ.Literal:
		if _, ok := v.Value.(string); ok && len(prefix) > 0 {
			return [][]segment.Segment{append([]segment.Segment(nil), prefix...)}
		}
		return nil
	case *typ.Alias:
		return stringLiteralSuffixes(v.UnaliasedTarget(), prefix, depth+1)
	case *typ.Optional:
		return stringLiteralSuffixes(v.Inner, prefix, depth+1)
	case *typ.Record:
		var out [][]segment.Segment
		for _, field := range v.Fields {
			next := append(append([]segment.Segment(nil), prefix...), segment.Segment{Kind: segment.SegmentField, Name: field.Name})
			out = append(out, stringLiteralSuffixes(field.Type, next, depth+1)...)
		}
		for _, member := range v.StaticMembers {
			if member.Kind != typ.StaticMemberStringIndex {
				continue
			}
			next := append(append([]segment.Segment(nil), prefix...), segment.Segment{Kind: segment.SegmentIndexString, Name: member.Name})
			out = append(out, stringLiteralSuffixes(member.Type, next, depth+1)...)
		}
		return out
	default:
		return nil
	}
}

func stringLiteralExprValue(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.StringExpr)
	if !ok {
		return "", false
	}
	return lit.Value, true
}

func pathSetContains(paths []pathdom.Path, target pathdom.Path) bool {
	for _, p := range paths {
		if p.Equal(target) {
			return true
		}
	}
	return false
}

type dispatchLookup struct {
	point        cfg.Point
	expr         *ast.AttrGetExpr
	table        pathdom.Path
	discriminant pathdom.Path
}

func (p discriminatedUnionExhaustiveness) tableDispatchDiagnostics(result *body.Result, graph cfg.Graph) []diagnostic.Diagnostic {
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		for _, lookup := range p.dispatchLookupsAt(result, point) {
			if diag, ok := p.tableDispatchDiagnostic(result, lookup); ok {
				out = append(out, diag)
			}
		}
	}
	return out
}

func (p discriminatedUnionExhaustiveness) dispatchLookupsAt(result *body.Result, point cfg.Point) []dispatchLookup {
	var out []dispatchLookup
	if fact, ok := result.LocalAssignment(point); ok && fact.Expr != nil {
		out = append(out, p.dispatchLookupsInExpr(result, point, fact.Expr, false)...)
	}
	if fact, ok := result.OrdinaryAssignment(point); ok && fact.Value != nil {
		out = append(out, p.dispatchLookupsInExpr(result, point, fact.Value, false)...)
	}
	if call, ok := result.Call(point); ok && call.Call != nil {
		out = append(out, p.dispatchLookupsInExpr(result, point, call.Func, true)...)
		if call.Receiver != nil {
			out = append(out, p.dispatchLookupsInExpr(result, point, call.Receiver, true)...)
		}
	}
	return out
}

func (p discriminatedUnionExhaustiveness) dispatchLookupsInExpr(result *body.Result, point cfg.Point, expr ast.Expr, scanCallFunc bool) []dispatchLookup {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		var out []dispatchLookup
		if lookup, ok := p.dispatchLookupFromAttr(result, point, e); ok {
			out = append(out, lookup)
		}
		out = append(out, p.dispatchLookupsInExpr(result, point, e.Object, scanCallFunc)...)
		out = append(out, p.dispatchLookupsInExpr(result, point, e.Key, scanCallFunc)...)
		return out
	case *ast.FuncCallExpr:
		if !scanCallFunc {
			return nil
		}
		var out []dispatchLookup
		out = append(out, p.dispatchLookupsInExpr(result, point, e.Func, scanCallFunc)...)
		out = append(out, p.dispatchLookupsInExpr(result, point, e.Receiver, scanCallFunc)...)
		return out
	case *ast.CastExpr:
		return p.dispatchLookupsInExpr(result, point, e.Expr, scanCallFunc)
	case *ast.NonNilAssertExpr:
		return p.dispatchLookupsInExpr(result, point, e.Expr, scanCallFunc)
	case *ast.LogicalOpExpr:
		return nil
	default:
		return nil
	}
}

func (p discriminatedUnionExhaustiveness) dispatchLookupFromAttr(result *body.Result, point cfg.Point, attr *ast.AttrGetExpr) (dispatchLookup, bool) {
	if attr == nil || attr.KeySyntax != ast.AttrKeyIndex {
		return dispatchLookup{}, false
	}
	tablePath, ok := result.ExpressionPath(attr.Object)
	if !ok || tablePath.Symbol == 0 {
		return dispatchLookup{}, false
	}
	discriminantPath, ok := result.ExpressionPath(attr.Key)
	if !ok || discriminantPath.Symbol == 0 || len(discriminantPath.Segments) == 0 {
		return dispatchLookup{}, false
	}
	return dispatchLookup{
		point:        point,
		expr:         attr,
		table:        tablePath,
		discriminant: discriminantPath,
	}, true
}

func (p discriminatedUnionExhaustiveness) tableDispatchDiagnostic(result *body.Result, lookup dispatchLookup) (diagnostic.Diagnostic, bool) {
	cases, ok := p.stringDiscriminantCases(result, lookup.point, lookup.discriminant)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	keys, tableSpan, ok := p.dispatchTableKeysAt(result, lookup.point, lookup.table)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	var possible []string
	var presentKeys []string
	var missingCases []string
	var missingKeys []string
	for _, c := range cases {
		possible = append(possible, c.name)
		if keys[c.key] {
			presentKeys = append(presentKeys, dispatchKeyName(lookup.table.String(), c.key))
			continue
		}
		missingCases = append(missingCases, c.name)
		missingKeys = append(missingKeys, dispatchKeyName(lookup.table.String(), c.key))
	}
	if len(missingKeys) == 0 {
		return diagnostic.Diagnostic{}, false
	}
	sort.Strings(presentKeys)
	lookupSpan := ast.SpanOf(lookup.expr)
	return newDispatchTableExhaustivenessDiagnostic(dispatchTableEvidence{
		table:      lookup.table.String(),
		target:     lookup.discriminant.String(),
		possible:   possible,
		keys:       presentKeys,
		missing:    missingKeys,
		missingFor: missingCases,
		tableSpan:  tableSpan,
		lookupSpan: lookupSpan,
	}), true
}

func (p discriminatedUnionExhaustiveness) stringDiscriminantCases(result *body.Result, point cfg.Point, target pathdom.Path) ([]discriminantCase, bool) {
	for _, anchor := range p.discriminantAnchors(result, point, target) {
		_, cases, ok := variant.OriginCasesOfType(anchor.anchorType)
		if !ok || len(cases) < 2 {
			continue
		}
		domainCases, ok := stringDiscriminantCasesFor(target, anchor.suffix, cases)
		if !ok {
			continue
		}
		return domainCases, true
	}
	return nil, false
}

func stringDiscriminantCasesFor(target pathdom.Path, suffix []segment.Segment, cases []variant.OriginCase) ([]discriminantCase, bool) {
	out := make([]discriminantCase, 0, len(cases))
	seen := make(map[string]struct{}, len(cases))
	for _, c := range cases {
		key, ok := discriminantCaseStringKey(c.Type, suffix)
		if !ok {
			return nil, false
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, false
		}
		seen[key] = struct{}{}
		out = append(out, discriminantCase{
			index:   c.Index,
			name:    discriminantCaseName(target, suffix, c.Type),
			key:     key,
			literal: discriminantCaseLiteralType(c.Type, suffix),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].name != out[j].name {
			return out[i].name < out[j].name
		}
		return out[i].index < out[j].index
	})
	return out, true
}

func discriminantCaseStringKey(caseType typ.Type, suffix []segment.Segment) (string, bool) {
	field, ok := variant.FieldAtPath(caseType, suffix)
	if !ok {
		return "", false
	}
	lit, ok := field.(*typ.Literal)
	if !ok {
		return "", false
	}
	value, ok := lit.Value.(string)
	return value, ok
}

func (p discriminatedUnionExhaustiveness) dispatchTableKeysAt(result *body.Result, point cfg.Point, table pathdom.Path) (map[string]bool, diagnostic.Span, bool) {
	if table.Symbol == 0 {
		return nil, diagnostic.Span{}, false
	}
	if keys, tableSpan, basePoint, ok := p.dispatchTableBaseKeysAt(result, point, table); ok {
		if !p.applyReachableDispatchTableAssignments(result, basePoint, point, table, keys) {
			return nil, diagnostic.Span{}, false
		}
		if p.trackedPathMayBeInvalidatedBetween(result, result.Graph(), basePoint, point, table) {
			return nil, diagnostic.Span{}, false
		}
		return keys, tableSpan, true
	}
	return p.inheritedDispatchTableKeysAt(result, point, table)
}

func (p discriminatedUnionExhaustiveness) dispatchTableBaseKeysAt(result *body.Result, point cfg.Point, table pathdom.Path) (map[string]bool, diagnostic.Span, cfg.Point, bool) {
	graph := result.Graph()
	if graph == nil || table.Symbol == 0 {
		return nil, diagnostic.Span{}, 0, false
	}
	var idom map[cfg.Point]cfg.Point
	if p.flow != nil && p.flow.graph == graph {
		idom = p.flow.immediateDominators()
	} else {
		idom = dominance.ComputeImmediateDominatorInfo(graph).Map()
	}
	visited := make(map[cfg.Point]struct{}, graph.Size())
	for cursor := point; ; {
		if _, ok := visited[cursor]; ok {
			return nil, diagnostic.Span{}, 0, false
		}
		visited[cursor] = struct{}{}
		if fact, ok := result.OrdinaryAssignment(cursor); ok {
			if keys, span, ok := dispatchTableReplacementKeys(result, fact, table); ok {
				return keys, span, cursor, true
			}
			_, staticKey, touches := dispatchTableAssignmentKeyForPath(fact, table)
			if touches && !staticKey {
				return nil, diagnostic.Span{}, 0, false
			}
		}
		if fact, ok := result.LocalAssignment(cursor); ok && fact.HasSymbol && fact.Symbol == table.Symbol && fact.Expr != nil {
			literal, ok := result.ObjectLiteral(fact.Expr)
			if !ok {
				return nil, diagnostic.Span{}, 0, false
			}
			keys, span, ok := objectLiteralDispatchKeysAtPath(result, literal, table.Segments)
			if !ok {
				return nil, diagnostic.Span{}, 0, false
			}
			return keys, span, cursor, true
		}
		parent, ok := idom[cursor]
		if !ok || parent == cursor {
			return nil, diagnostic.Span{}, 0, false
		}
		cursor = parent
	}
}

func (p discriminatedUnionExhaustiveness) inheritedDispatchTableKeysAt(result *body.Result, point cfg.Point, table pathdom.Path) (map[string]bool, diagnostic.Span, bool) {
	summary, ok := p.dispatchTables[table.Key()]
	if !ok || len(summary.keys) == 0 {
		return nil, diagnostic.Span{}, false
	}
	keys := cloneDispatchKeySet(summary.keys)
	if !p.applyReachableDispatchTableAssignments(result, result.Graph().Entry(), point, table, keys) {
		return nil, diagnostic.Span{}, false
	}
	if p.trackedPathMayBeInvalidatedBetween(result, result.Graph(), result.Graph().Entry(), point, table) {
		return nil, diagnostic.Span{}, false
	}
	return keys, summary.span, true
}

func (p discriminatedUnionExhaustiveness) trackedPathMayBeInvalidatedBetween(result *body.Result, graph cfg.Graph, from, to cfg.Point, target pathdom.Path) bool {
	if result == nil || graph == nil || target.IsEmpty() {
		return false
	}
	for _, candidate := range graph.RPO() {
		if candidate == to {
			continue
		}
		if !diagnosticCanReach(p.flow, graph, from, candidate) || !diagnosticCanReach(p.flow, graph, candidate, to) {
			continue
		}
		if callMayInvalidateTrackedPath(result, candidate, target) {
			return true
		}
	}
	return false
}

func objectLiteralDispatchKeys(fact semantics.ObjectLiteralFact) (map[string]bool, bool) {
	if fact.Table == nil {
		return nil, false
	}
	keys := make(map[string]bool, len(fact.Table.Fields))
	arrayIndex := 0
	for _, field := range fact.Table.Fields {
		suffix, ok := pathexpr.ResolveTableFieldSuffix(field, &arrayIndex)
		if !ok {
			return nil, false
		}
		switch suffix.Kind {
		case pathexpr.TableFieldSuffixField, pathexpr.TableFieldSuffixStringIndex:
			if suffix.Segment.Name == "" {
				return nil, false
			}
			keys[suffix.Segment.Name] = true
		case pathexpr.TableFieldSuffixImplicitIndex, pathexpr.TableFieldSuffixIntIndex:
			continue
		default:
			return nil, false
		}
	}
	return keys, true
}

func objectLiteralDispatchKeysAtPath(result *body.Result, fact semantics.ObjectLiteralFact, suffix []segment.Segment) (map[string]bool, diagnostic.Span, bool) {
	if len(suffix) == 0 {
		keys, ok := objectLiteralDispatchKeys(fact)
		return keys, ast.SpanOf(fact.Table), ok
	}
	nested, ok := nestedObjectLiteralFact(result, fact, suffix)
	if !ok {
		return nil, diagnostic.Span{}, false
	}
	keys, ok := objectLiteralDispatchKeys(nested)
	return keys, ast.SpanOf(nested.Table), ok
}

func nestedObjectLiteralFact(result *body.Result, fact semantics.ObjectLiteralFact, suffix []segment.Segment) (semantics.ObjectLiteralFact, bool) {
	if result == nil || len(suffix) == 0 {
		return semantics.ObjectLiteralFact{}, false
	}
	for _, entry := range fact.Entries {
		if !sameSegments(entry.Suffix.Segments, suffix) {
			continue
		}
		nested, ok := result.ObjectLiteral(entry.Value)
		return nested, ok
	}
	return semantics.ObjectLiteralFact{}, false
}

func sameSegments(a, b []segment.Segment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (p discriminatedUnionExhaustiveness) applyReachableDispatchTableAssignments(result *body.Result, from, point cfg.Point, table pathdom.Path, keys map[string]bool) bool {
	graph := result.Graph()
	if graph == nil {
		return false
	}
	var idom map[cfg.Point]cfg.Point
	if p.flow != nil && p.flow.graph == graph {
		idom = p.flow.immediateDominators()
	} else {
		idom = dominance.ComputeImmediateDominatorInfo(graph).Map()
	}
	if len(idom) == 0 {
		return false
	}
	for _, candidate := range graph.RPO() {
		if candidate == from || candidate == point {
			continue
		}
		fact, ok := result.OrdinaryAssignment(candidate)
		if !ok {
			continue
		}
		key, staticKey, touches := dispatchTableAssignmentKeyForPath(fact, table)
		if !touches ||
			!diagnosticCanReach(p.flow, graph, from, candidate) ||
			!diagnosticCanReach(p.flow, graph, candidate, point) {
			continue
		}
		if dominance.Dominates(idom, candidate, point) {
			if replacement, _, ok := dispatchTableReplacementKeys(result, fact, table); ok {
				replaceDispatchKeySet(keys, replacement)
				continue
			}
			if !staticKey {
				return false
			}
			keys[key] = true
			continue
		}
		if replacement, _, ok := dispatchTableReplacementKeys(result, fact, table); ok {
			intersectDispatchKeySet(keys, replacement)
			continue
		}
		if !staticKey {
			return false
		}
	}
	return true
}

func dispatchTableReplacementKeys(result *body.Result, fact semantics.OrdinaryAssignmentFact, table pathdom.Path) (map[string]bool, diagnostic.Span, bool) {
	suffix, ok := dispatchTableReplacementSuffix(fact, table)
	if !ok || fact.Value == nil {
		return nil, diagnostic.Span{}, false
	}
	literal, ok := result.ObjectLiteral(fact.Value)
	if !ok {
		return nil, diagnostic.Span{}, false
	}
	return objectLiteralDispatchKeysAtPath(result, literal, suffix)
}

func dispatchTableReplacementSuffix(fact semantics.OrdinaryAssignmentFact, table pathdom.Path) ([]segment.Segment, bool) {
	if table.Symbol == 0 {
		return nil, false
	}
	if fact.HasSymbol && fact.Symbol == table.Symbol {
		return append([]segment.Segment(nil), table.Segments...), true
	}
	if !fact.HasPath {
		return nil, false
	}
	if fact.Path.Equal(table) {
		return nil, true
	}
	if pathHasPrefix(table, fact.Path) {
		suffix := table.Segments[len(fact.Path.Segments):]
		return append([]segment.Segment(nil), suffix...), true
	}
	return nil, false
}

func replaceDispatchKeySet(target map[string]bool, replacement map[string]bool) {
	for key := range target {
		delete(target, key)
	}
	for key, present := range replacement {
		target[key] = present
	}
}

func intersectDispatchKeySet(target map[string]bool, other map[string]bool) {
	for key := range target {
		if !other[key] {
			delete(target, key)
		}
	}
}

func dispatchTableAssignmentKeyForPath(fact semantics.OrdinaryAssignmentFact, table pathdom.Path) (key string, staticKey bool, touches bool) {
	if table.Symbol == 0 {
		return "", false, false
	}
	if fact.HasPath {
		if pathHasPrefix(fact.Path, table) {
			suffix := fact.Path.Segments[len(table.Segments):]
			if len(suffix) != 1 {
				return "", false, true
			}
			key, ok := segmentStringKey(suffix[0])
			return key, ok, true
		}
		if pathHasPrefix(table, fact.Path) {
			return "", false, true
		}
	}
	if fact.HasSymbol && fact.Symbol == table.Symbol {
		return "", false, true
	}
	if fact.HasContainerPath && pathsOverlapForInvalidation(table, fact.ContainerPath) {
		return "", false, true
	}
	return "", false, false
}

func segmentStringKey(seg segment.Segment) (string, bool) {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return seg.Name, seg.Name != ""
	default:
		return "", false
	}
}

func dispatchKeyName(table, key string) string {
	if identifierName(key) {
		return table + "." + key
	}
	return table + "[" + formatType(typ.LiteralString(key)) + "]"
}

func registrationCaseName(registry, key string) string {
	return dispatchKeyName(registry, key)
}

func identifierName(s string) bool {
	if s == "" {
		return false
	}
	if !((s[0] >= 'A' && s[0] <= 'Z') || (s[0] >= 'a' && s[0] <= 'z') || s[0] == '_') {
		return false
	}
	for i := 1; i < len(s); i++ {
		ch := s[i]
		if !((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_') {
			return false
		}
	}
	return true
}

func newDiscriminatedUnionExhaustivenessDiagnostic(span diagnostic.Span, evidence discriminatedUnionEvidence) diagnostic.Diagnostic {
	caseWord := pluralize(len(evidence.missing), "case", "cases")
	missing := discriminantCaseList(evidence.missing)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        span,
		Code:        CodeDiscriminatedUnionExhaustive,
		Severity:    diagnostic.SeverityWarning,
		Message:     discriminatedUnionExhaustivenessMessage(caseWord, missing),
		Explanation: discriminatedUnionExhaustivenessExplanation(span, evidence),
		Help:        discriminatedUnionExhaustivenessHelp(),
		Labels:      []diagnostic.Label{sourceLabel(span, labelUnionCaseTest)},
	})
}

func newOptionalExhaustivenessDiagnostic(evidence optionalEvidence) diagnostic.Diagnostic {
	caseWord := pluralize(len(evidence.missing), "case", "cases")
	missing := discriminantCaseList(evidence.missing)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        evidence.span,
		Code:        CodeDiscriminatedUnionExhaustive,
		Severity:    diagnostic.SeverityWarning,
		Message:     optionalExhaustivenessMessage(caseWord, missing),
		Explanation: optionalExhaustivenessExplanation(evidence),
		Help:        optionalExhaustivenessHelp(),
		Labels:      []diagnostic.Label{sourceLabel(evidence.span, labelOptionalCaseCheck)},
	})
}

func newDispatchTableExhaustivenessDiagnostic(evidence dispatchTableEvidence) diagnostic.Diagnostic {
	keyWord := pluralize(len(evidence.missing), "key", "keys")
	missing := dispatchKeyList(evidence.missing)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        evidence.lookupSpan,
		Code:        CodeDiscriminatedUnionExhaustive,
		Severity:    diagnostic.SeverityWarning,
		Message:     dispatchTableExhaustivenessMessage(keyWord, missing),
		Explanation: dispatchTableExhaustivenessExplanation(evidence),
		Help:        dispatchTableExhaustivenessHelp(),
		Labels: []diagnostic.Label{
			sourceLabel(evidence.tableSpan, labelDispatchTable),
			sourceLabel(evidence.lookupSpan, labelDispatchLookup),
		},
	})
}

func newRegistrationExhaustivenessDiagnostic(evidence registrationEvidence) diagnostic.Diagnostic {
	registrationWord := pluralize(len(evidence.missing), "registration", "registrations")
	missing := dispatchKeyList(evidence.missing)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        evidence.dispatchSpan,
		Code:        CodeDiscriminatedUnionExhaustive,
		Severity:    diagnostic.SeverityWarning,
		Message:     registrationExhaustivenessMessage(registrationWord, missing),
		Explanation: registrationExhaustivenessExplanation(evidence),
		Help:        registrationExhaustivenessHelp(),
		Labels: []diagnostic.Label{
			sourceLabel(evidence.registrationSpan, labelRegistrationCall),
			sourceLabel(evidence.dispatchSpan, labelDispatchCall),
		},
	})
}

func newResultShapeExhaustivenessDiagnostic(evidence resultShapeEvidence) diagnostic.Diagnostic {
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        evidence.readSpan,
		Code:        CodeDiscriminatedUnionExhaustive,
		Severity:    diagnostic.SeverityWarning,
		Message:     resultShapeExhaustivenessMessage(evidence.readPath, evidence.requiredCase),
		Explanation: resultShapeExhaustivenessExplanation(evidence),
		Help:        resultShapeExhaustivenessHelp(),
		Labels:      []diagnostic.Label{sourceLabel(evidence.readSpan, labelResultFieldRead)},
	})
}

func discriminatedUnionExhaustivenessExplanation(span diagnostic.Span, evidence discriminatedUnionEvidence) diagnostic.Explanation {
	items := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    span,
			Message: selectedDiscriminantPathEvidence(evidence.target),
		},
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    span,
			Message: possibleDiscriminantCasesEvidence(discriminantCaseList(evidence.possible)),
		},
	}
	if len(evidence.handled) > 0 {
		items = append(items, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    span,
			Message: handledDiscriminantCasesEvidence(discriminantCaseList(evidence.handled)),
		})
	}
	items = append(items,
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    span,
			Message: missingDiscriminantCasesEvidence(discriminantCaseList(evidence.missing)),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    span,
			Message: missingDiscriminantDefaultEvidence(),
		},
	)
	return diagnostic.NewExplanation(items...)
}

func optionalExhaustivenessExplanation(evidence optionalEvidence) diagnostic.Explanation {
	return diagnostic.NewExplanation(
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.span,
			Message: selectedOptionalPathEvidence(evidence.target),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.span,
			Message: optionalPossibleCasesEvidence(evidence.target),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.span,
			Message: optionalConsumedCaseEvidence(evidence.target),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    evidence.span,
			Message: optionalMissingCasesEvidence(discriminantCaseList(evidence.missing)),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    evidence.span,
			Message: optionalMissingDefaultEvidence(),
		},
	)
}

func dispatchTableExhaustivenessExplanation(evidence dispatchTableEvidence) diagnostic.Explanation {
	return diagnostic.NewExplanation(
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.lookupSpan,
			Message: dispatchLookupEvidence(evidence.table, evidence.target),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.lookupSpan,
			Message: possibleDiscriminantCasesEvidence(discriminantCaseList(evidence.possible)),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.tableSpan,
			Message: dispatchTableKeysEvidence(dispatchKeyList(evidence.keys)),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    evidence.lookupSpan,
			Message: missingDispatchKeysEvidence(dispatchMissingKeyCases(evidence.missing, evidence.missingFor)),
		},
	)
}

func resultShapeExhaustivenessExplanation(evidence resultShapeEvidence) diagnostic.Explanation {
	return diagnostic.NewExplanation(
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.readSpan,
			Message: resultShapeUnionEvidence(evidence.receiver, evidence.discriminant),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.readSpan,
			Message: resultShapeFieldCaseEvidence(evidence.readPath, evidence.requiredCase),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    evidence.readSpan,
			Message: resultShapeMissingProofEvidence(evidence.requiredCase),
		},
	)
}

func registrationExhaustivenessExplanation(evidence registrationEvidence) diagnostic.Explanation {
	return diagnostic.NewExplanation(
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.dispatchSpan,
			Message: registrationDispatchEvidence(evidence.registry, evidence.target),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.dispatchSpan,
			Message: possibleDiscriminantCasesEvidence(discriminantCaseList(evidence.possible)),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    evidence.registrationSpan,
			Message: registeredCasesEvidence(dispatchKeyList(evidence.registered)),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    evidence.dispatchSpan,
			Message: missingRegistrationsEvidence(dispatchMissingKeyCases(evidence.missing, evidence.missingFor)),
		},
	)
}

func discriminantCaseList(cases []string) string {
	return strings.Join(codeNames(cases), ", ")
}

func dispatchKeyList(keys []string) string {
	if len(keys) == 0 {
		return "none"
	}
	return strings.Join(codeNames(keys), ", ")
}

func dispatchMissingKeyCases(keys []string, cases []string) string {
	if len(keys) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(keys))
	for i, key := range keys {
		if i < len(cases) && cases[i] != "" {
			parts = append(parts, codeName(key)+" for "+codeName(cases[i]))
		} else {
			parts = append(parts, codeName(key))
		}
	}
	return strings.Join(parts, ", ")
}
