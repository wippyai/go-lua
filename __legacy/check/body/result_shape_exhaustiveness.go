package body

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	typeformat "github.com/wippyai/go-lua/analysis/type/format"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// ResultShapeExhaustivenessProof records a case-specific member read on a
// discriminated union where solved state has not proved the required case.
type ResultShapeExhaustivenessProof struct {
	Point         cfg.Point
	ReceiverLabel string
	ReadLabel     string
	Discriminant  string
	RequiredCase  string
	Span          SourceSpan
}

type resultShapeCase struct {
	index   int
	name    string
	literal typ.Type
}

type resultShapeLiteralDomain struct {
	target path.Path
	suffix []segment.Segment
	cases  []resultShapeCase
}

// ForEachResultShapeExhaustivenessProof visits case-specific field reads on
// discriminated unions where solved state has not proved the required case.
func (r *Result) ForEachResultShapeExhaustivenessProof(visit func(ResultShapeExhaustivenessProof) bool) bool {
	if r == nil || visit == nil {
		return false
	}
	visited := false
	return r.ForEachResultShapeReadOccurrence(func(occ StaticMemberReadOccurrence) bool {
		item, ok := r.resultShapeRead(occ)
		if !ok {
			return true
		}
		visited = true
		return visit(item)
	}) || visited
}

func (r *Result) resultShapeRead(occ StaticMemberReadOccurrence) (ResultShapeExhaustivenessProof, bool) {
	member := occ.MemberName
	if member == "" || member == "ok" {
		return ResultShapeExhaustivenessProof{}, false
	}
	if !occ.HasReceiverPath || occ.ReceiverPath.Symbol == 0 {
		return ResultShapeExhaustivenessProof{}, false
	}
	if !occ.HasReadPath || occ.ReadPath.Symbol == 0 {
		return ResultShapeExhaustivenessProof{}, false
	}
	receiverType, ok := r.resultShapeBroadReceiverType(occ.Point, occ.ReceiverPath)
	if !ok {
		return ResultShapeExhaustivenessProof{}, false
	}
	discriminant, required, ok := resultShapeRequiredCaseForMember(occ.ReceiverPath, receiverType, member)
	if !ok {
		return ResultShapeExhaustivenessProof{}, false
	}
	if r.resultShapeRequiredCaseProven(occ.Point, discriminant, required) {
		return ResultShapeExhaustivenessProof{}, false
	}
	if r.resultShapeOtherCaseDominates(occ.Point, discriminant, required) {
		return ResultShapeExhaustivenessProof{}, false
	}
	return ResultShapeExhaustivenessProof{
		Point:         occ.Point,
		ReceiverLabel: occ.ReceiverPath.String(),
		ReadLabel:     occ.ReadPath.String(),
		Discriminant:  discriminant.String(),
		RequiredCase:  required.name,
		Span:          occ.Span,
	}, true
}

func (r *Result) resultShapeBroadReceiverType(point cfg.Point, receiver path.Path) (typ.Type, bool) {
	if r == nil || receiver.IsEmpty() || receiver.Symbol == 0 {
		return nil, false
	}
	return r.DeclaredOrVariantOriginPathTypeAt(point, receiver)
}

func resultShapeRequiredCaseForMember(receiver path.Path, receiverType typ.Type, member string) (path.Path, resultShapeCase, bool) {
	_, cases, ok := variant.OriginCasesOfType(receiverType)
	if !ok || len(cases) < 2 {
		return path.Path{}, resultShapeCase{}, false
	}
	requiredIndex, ok := variant.SingleCaseWithField(cases, member)
	if !ok {
		return path.Path{}, resultShapeCase{}, false
	}
	for _, domain := range resultShapeLiteralDomainsForCases(receiver, cases) {
		for _, c := range domain.cases {
			if c.index == requiredIndex {
				return domain.target, c, true
			}
		}
	}
	return path.Path{}, resultShapeCase{}, false
}

func resultShapeLiteralDomainsForCases(receiver path.Path, cases []variant.OriginCase) []resultShapeLiteralDomain {
	if len(cases) == 0 {
		return nil
	}
	domains, ok := variant.LiteralDiscriminantDomainsForCases(cases)
	if !ok {
		return nil
	}
	out := make([]resultShapeLiteralDomain, 0, len(domains))
	for _, domain := range domains {
		suffix := domain.Suffix
		target := receiver.AppendSegments(suffix)
		domainCases, ok := resultShapeLiteralCasesFor(target, suffix, cases)
		if !ok {
			continue
		}
		out = append(out, resultShapeLiteralDomain{
			target: target,
			suffix: append([]segment.Segment(nil), suffix...),
			cases:  domainCases,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].target.String() < out[j].target.String()
	})
	return out
}

func resultShapeLiteralCasesFor(target path.Path, suffix []segment.Segment, cases []variant.OriginCase) ([]resultShapeCase, bool) {
	out := make([]resultShapeCase, 0, len(cases))
	var seen []typ.Type
	for _, c := range cases {
		lit, ok := resultShapeDiscriminantCaseLiteral(c.Type, suffix)
		if !ok || !resultShapeLiteralSupported(lit) {
			return nil, false
		}
		for _, previous := range seen {
			if typ.TypeEquals(previous, lit) {
				return nil, false
			}
		}
		seen = append(seen, lit)
		out = append(out, resultShapeCase{
			index:   c.Index,
			name:    resultShapeDiscriminantCaseName(target, suffix, c.Type),
			literal: lit,
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

func resultShapeDiscriminantCaseName(target path.Path, suffix []segment.Segment, caseType typ.Type) string {
	if field, ok := variant.FieldAtPath(caseType, suffix); ok {
		return target.String() + " == " + typeformat.Short(field)
	}
	return typeformat.Short(caseType)
}

func resultShapeDiscriminantCaseLiteral(caseType typ.Type, suffix []segment.Segment) (*typ.Literal, bool) {
	field, ok := variant.FieldAtPath(caseType, suffix)
	if !ok {
		return nil, false
	}
	lit, ok := field.(*typ.Literal)
	return lit, ok
}

func resultShapeLiteralSupported(lit *typ.Literal) bool {
	if lit == nil {
		return false
	}
	switch lit.Value.(type) {
	case bool, string:
		return true
	default:
		return false
	}
}

func (r *Result) resultShapeRequiredCaseProven(point cfg.Point, discriminant path.Path, required resultShapeCase) bool {
	if required.literal == nil {
		return false
	}
	if r.DominatingBranchProvesLiteral(point, discriminant, required.literal) {
		return true
	}
	lit, ok := r.PathLiteralTypeAtBoundary(point, discriminant)
	return ok && typ.TypeEquals(lit, required.literal)
}

func (r *Result) resultShapeOtherCaseDominates(point cfg.Point, discriminant path.Path, required resultShapeCase) bool {
	if required.literal == nil {
		return false
	}
	proven, _, ok := r.DominatingLiteralBranchForPath(point, discriminant)
	return ok && !typ.TypeEquals(proven, required.literal)
}
