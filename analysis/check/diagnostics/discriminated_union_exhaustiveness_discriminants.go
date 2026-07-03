package diagnostics

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

type discriminantCase struct {
	index   int
	name    string
	key     string
	literal typ.Type
}

type discriminantAnchor struct {
	anchor     pathdom.Path
	anchorType typ.Type
	suffix     []segment.Segment
}

func (p discriminatedUnionExhaustiveness) discriminantAnchors(result *body.Result, point cfg.Point, target pathdom.Path) []discriminantAnchor {
	if target.Symbol == 0 || len(target.Segments) == 0 {
		return nil
	}
	root := target.RootOnly()
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

func discriminantCaseName(target pathdom.Path, suffix []segment.Segment, caseType typ.Type) string {
	if field, ok := variant.FieldAtPath(caseType, suffix); ok {
		return target.String() + " == " + formatType(field)
	}
	return formatType(caseType)
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
	domains, ok := variant.LiteralDiscriminantDomainsForCases(cases)
	if !ok {
		return nil
	}
	for _, domain := range domains {
		suffix := domain.Suffix
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

func segmentStringKey(seg segment.Segment) (string, bool) {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return seg.Name, seg.Name != ""
	default:
		return "", false
	}
}
