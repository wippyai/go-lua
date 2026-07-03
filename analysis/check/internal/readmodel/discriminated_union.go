package readmodel

import (
	"sort"

	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	typeformat "github.com/wippyai/go-lua/analysis/type/format"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type DiscriminatedUnionExhaustiveness = readapi.DiscriminatedUnionExhaustiveness

type discriminatedUnionBranch struct {
	point cfg.Point
	fact  semantics.BranchConditionFact
}

type discriminatedUnionCandidate struct {
	target  path.Path
	anchor  path.Path
	family  uint64
	cases   []discriminatedUnionCase
	handled []int
}

type discriminatedUnionCase struct {
	index int
	name  string
}

type discriminatedUnionAnchor struct {
	anchor     path.Path
	anchorType typ.Type
	suffix     []segment.Segment
}

// ForEachDiscriminatedUnionExhaustiveness visits normally reachable if/elseif
// branch chains that cover only part of a discriminated union.
func (r Reader) ForEachDiscriminatedUnionExhaustiveness(visit func(DiscriminatedUnionExhaustiveness) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	branches := r.discriminatedUnionBranches()
	if len(branches) == 0 {
		return false
	}
	ifs := make([]*ast.IfStmt, 0, len(branches))
	byIf := make(map[*ast.IfStmt]discriminatedUnionBranch, len(branches))
	for _, branch := range branches {
		if branch.fact.If == nil {
			continue
		}
		ifs = append(ifs, branch.fact.If)
		byIf[branch.fact.If] = branch
	}
	nested := nestedElseIfStatements(ifs)
	visited := false
	for _, branch := range branches {
		if branch.fact.If == nil || nested[branch.fact.If] {
			continue
		}
		item, ok := r.discriminatedUnionChain(branch.fact.If, byIf)
		if !ok {
			continue
		}
		visited = true
		if !visit(item) {
			return true
		}
	}
	return visited
}

func (r Reader) discriminatedUnionBranches() []discriminatedUnionBranch {
	graph := r.result.Graph()
	var out []discriminatedUnionBranch
	for _, point := range cfg.RPOReadOnly(graph) {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		branch, ok := r.result.BranchCondition(point)
		if !ok || branch.If == nil {
			continue
		}
		out = append(out, discriminatedUnionBranch{point: point, fact: branch})
	}
	return out
}

func (r Reader) discriminatedUnionChain(head *ast.IfStmt, byIf map[*ast.IfStmt]discriminatedUnionBranch) (DiscriminatedUnionExhaustiveness, bool) {
	if hasDefaultElse(head) {
		return DiscriminatedUnionExhaustiveness{}, false
	}
	chain := ifElseIfChain(head)
	var selected discriminatedUnionCandidate
	selectedSet := false
	handled := make(map[int]bool)
	for _, stmt := range chain {
		branch, ok := byIf[stmt]
		if !ok {
			return DiscriminatedUnionExhaustiveness{}, false
		}
		candidate, ok := r.discriminatedUnionCandidateForCheck(branch.point, branch.fact.Check)
		if !ok {
			return DiscriminatedUnionExhaustiveness{}, false
		}
		if !selectedSet {
			selected = candidate
			selectedSet = true
		} else if !sameDiscriminatedUnionCandidate(selected, candidate) {
			return DiscriminatedUnionExhaustiveness{}, false
		}
		for _, index := range candidate.handled {
			handled[index] = true
		}
	}
	if !selectedSet || len(handled) == 0 || len(handled) >= len(selected.cases) {
		return DiscriminatedUnionExhaustiveness{}, false
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
		return DiscriminatedUnionExhaustiveness{}, false
	}
	return DiscriminatedUnionExhaustiveness{
		Point:    selectedPoint(head, byIf),
		Span:     sourceSpanFromAST(ast.SpanOf(head.Condition)),
		Target:   selected.target.String(),
		Possible: possible,
		Handled:  handledNames,
		Missing:  missing,
	}, true
}

func selectedPoint(head *ast.IfStmt, byIf map[*ast.IfStmt]discriminatedUnionBranch) cfg.Point {
	if branch, ok := byIf[head]; ok {
		return branch.point
	}
	return 0
}

func (r Reader) discriminatedUnionCandidateForCheck(point cfg.Point, check branchcond.Check) (discriminatedUnionCandidate, bool) {
	lit, negate, ok := discriminatedUnionCheckLiteral(check)
	if !ok {
		return discriminatedUnionCandidate{}, false
	}
	for _, anchor := range r.discriminatedUnionAnchors(point, check.Path) {
		family, handled, ok := discriminatedUnionOriginByCheck(anchor.anchorType, anchor.suffix, lit, negate)
		if !ok {
			continue
		}
		caseFamily, cases, ok := variant.OriginCasesOfType(anchor.anchorType)
		if !ok || caseFamily != family || len(cases) < 2 {
			continue
		}
		return discriminatedUnionCandidate{
			target:  check.Path,
			anchor:  anchor.anchor,
			family:  family,
			cases:   discriminatedUnionCasesFor(check.Path, anchor.suffix, cases),
			handled: handled,
		}, true
	}
	return discriminatedUnionCandidate{}, false
}

func (r Reader) discriminatedUnionAnchors(point cfg.Point, target path.Path) []discriminatedUnionAnchor {
	if target.Symbol == 0 || len(target.Segments) == 0 {
		return nil
	}
	root := target.RootOnly()
	rootType, ok := r.discriminatedUnionRootType(point, root)
	if !ok {
		return nil
	}
	segments := target.Segments
	out := make([]discriminatedUnionAnchor, 0, len(segments))
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
		out = append(out, discriminatedUnionAnchor{
			anchor:     anchorPath,
			anchorType: anchorType,
			suffix:     append([]segment.Segment(nil), suffix...),
		})
	}
	return out
}

func (r Reader) discriminatedUnionRootType(point cfg.Point, root path.Path) (typ.Type, bool) {
	if root.Symbol == 0 {
		return nil, false
	}
	if annotated, ok := r.symbolDeclaredType(root.Symbol); ok {
		return r.resultShapeTransparentComparableType(annotated), true
	}
	value, ok := r.result.SymbolValueAtBoundary(point, root.Symbol)
	if !ok {
		return nil, false
	}
	return r.FullVariantOriginType(value)
}

func discriminatedUnionCheckLiteral(check branchcond.Check) (typ.Type, bool, bool) {
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

func discriminatedUnionOriginByCheck(anchorType typ.Type, rest []segment.Segment, lit typ.Type, negate bool) (uint64, []int, bool) {
	if negate {
		return variant.OriginByPathLiteralNot(anchorType, rest, lit)
	}
	return variant.OriginByPathLiteral(anchorType, rest, lit)
}

func discriminatedUnionCasesFor(target path.Path, suffix []segment.Segment, cases []variant.OriginCase) []discriminatedUnionCase {
	out := make([]discriminatedUnionCase, 0, len(cases))
	for _, c := range cases {
		out = append(out, discriminatedUnionCase{
			index: c.Index,
			name:  discriminatedUnionCaseName(target, suffix, c.Type),
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

func discriminatedUnionCaseName(target path.Path, suffix []segment.Segment, caseType typ.Type) string {
	if field, ok := variant.FieldAtPath(caseType, suffix); ok {
		return target.String() + " == " + typeformat.Short(field)
	}
	return typeformat.Short(caseType)
}

func sameDiscriminatedUnionCandidate(a, b discriminatedUnionCandidate) bool {
	return a.family == b.family && a.target.Equal(b.target) && a.anchor.Equal(b.anchor)
}

func nestedElseIfStatements(stmts []*ast.IfStmt) map[*ast.IfStmt]bool {
	out := make(map[*ast.IfStmt]bool)
	for _, stmt := range stmts {
		if stmt == nil || len(stmt.Else) == 0 {
			continue
		}
		if nested, ok := stmt.Else[0].(*ast.IfStmt); ok && nested != nil {
			out[nested] = true
		}
	}
	return out
}

func hasDefaultElse(stmt *ast.IfStmt) bool {
	for stmt != nil {
		if len(stmt.Else) == 0 {
			return false
		}
		next, ok := stmt.Else[0].(*ast.IfStmt)
		if !ok {
			return true
		}
		stmt = next
	}
	return false
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
