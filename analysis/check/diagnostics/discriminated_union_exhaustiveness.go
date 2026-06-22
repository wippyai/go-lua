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
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type discriminatedUnionExhaustiveness producerContext

type discriminantBranch struct {
	point cfg.Point
	fact  semantics.BranchConditionFact
}

type discriminantCase struct {
	index int
	name  string
	key   string
}

type discriminantCandidate struct {
	target  pathdom.Path
	anchor  pathdom.Path
	family  uint64
	cases   []discriminantCase
	handled []int
}

type discriminatedUnionEvidence struct {
	target   string
	possible []string
	handled  []string
	missing  []string
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
	}
	out = append(out, p.tableDispatchDiagnostics(result, graph)...)
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

func (p discriminatedUnionExhaustiveness) candidateForCheck(result *body.Result, point cfg.Point, check branchcond.Check) (discriminantCandidate, bool) {
	lit, negate, ok := discriminantCheckLiteral(check)
	if !ok || check.Path.Symbol == 0 || len(check.Path.Segments) == 0 {
		return discriminantCandidate{}, false
	}
	root := check.Path
	root.Segments = nil
	rootType, ok := discriminantRootType(result, p.resolver, point, root)
	if !ok {
		return discriminantCandidate{}, false
	}
	segments := check.Path.Segments
	for prefixLen := 0; prefixLen < len(segments); prefixLen++ {
		prefix := segments[:prefixLen]
		rest := segments[prefixLen:]
		anchorType := rootType
		if len(prefix) > 0 {
			var fieldOK bool
			anchorType, fieldOK = variant.FieldAtPath(rootType, prefix)
			if !fieldOK {
				continue
			}
		}
		family, handled, ok := discriminantOriginByCheck(anchorType, rest, lit, negate)
		if !ok {
			continue
		}
		caseFamily, cases, ok := variant.OriginCasesOfType(anchorType)
		if !ok || caseFamily != family || len(cases) < 2 {
			continue
		}
		anchor := root
		anchor.Segments = append([]segment.Segment(nil), prefix...)
		return discriminantCandidate{
			target:  check.Path,
			anchor:  anchor,
			family:  family,
			cases:   discriminantCasesFor(check.Path, rest, cases),
			handled: handled,
		}, true
	}
	return discriminantCandidate{}, false
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
			index: c.Index,
			name:  discriminantCaseName(target, suffix, c.Type),
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
	if !ok || tablePath.Symbol == 0 || len(tablePath.Segments) != 0 {
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
	if target.Symbol == 0 || len(target.Segments) == 0 {
		return nil, false
	}
	root := target
	root.Segments = nil
	rootType, ok := discriminantRootType(result, p.resolver, point, root)
	if !ok {
		return nil, false
	}
	segments := target.Segments
	for prefixLen := 0; prefixLen < len(segments); prefixLen++ {
		prefix := segments[:prefixLen]
		rest := segments[prefixLen:]
		anchorType := rootType
		if len(prefix) > 0 {
			var fieldOK bool
			anchorType, fieldOK = variant.FieldAtPath(rootType, prefix)
			if !fieldOK {
				continue
			}
		}
		_, cases, ok := variant.OriginCasesOfType(anchorType)
		if !ok || len(cases) < 2 {
			continue
		}
		domainCases, ok := stringDiscriminantCasesFor(target, rest, cases)
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
			index: c.Index,
			name:  discriminantCaseName(target, suffix, c.Type),
			key:   key,
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
	if table.Symbol == 0 || len(table.Segments) != 0 {
		return nil, diagnostic.Span{}, false
	}
	decl, declPoint, ok := dominatingRootLocalAssignment(result, p.flow, point, table.Symbol)
	if !ok || decl.Expr == nil {
		return nil, diagnostic.Span{}, false
	}
	fact, ok := result.ObjectLiteral(decl.Expr)
	if !ok {
		return nil, diagnostic.Span{}, false
	}
	keys, ok := objectLiteralDispatchKeys(fact)
	if !ok {
		return nil, diagnostic.Span{}, false
	}
	if !p.addDominatingStaticDispatchWrites(result, declPoint, point, table.Symbol, keys) {
		return nil, diagnostic.Span{}, false
	}
	return keys, ast.SpanOf(fact.Table), true
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
		default:
			return nil, false
		}
	}
	return keys, true
}

func (p discriminatedUnionExhaustiveness) addDominatingStaticDispatchWrites(result *body.Result, declPoint, point cfg.Point, rootSymbol symbol.ID, keys map[string]bool) bool {
	if result.Graph() == nil || p.flow == nil {
		return true
	}
	idom := p.flow.immediateDominators()
	for cursor := point; cursor != declPoint; {
		if fact, ok := result.OrdinaryAssignment(cursor); ok {
			if fact.HasPath && fact.Path.Symbol == rootSymbol {
				if len(fact.Path.Segments) != 1 {
					return false
				}
				key, ok := segmentStringKey(fact.Path.Segments[0])
				if !ok {
					return false
				}
				keys[key] = true
			} else if fact.HasContainerPath && fact.ContainerPath.Symbol == rootSymbol {
				return false
			}
		}
		parent, ok := idom[cursor]
		if !ok || parent == cursor {
			return false
		}
		cursor = parent
	}
	return true
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
