package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

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
