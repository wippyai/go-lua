package readmodel

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	typeformat "github.com/wippyai/go-lua/analysis/type/format"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func (r Reader) registrationStringDiscriminantCasesForArgument(point cfg.Point, argPath path.Path) (path.Path, []registrationDiscriminantCase, bool) {
	if len(argPath.Segments) > 0 {
		cases, ok := r.registrationStringDiscriminantCases(point, argPath)
		return argPath, cases, ok
	}
	for _, domain := range r.registrationStringDiscriminantDomainsForRoot(point, argPath) {
		return domain.target, domain.cases, true
	}
	return path.Path{}, nil, false
}

type registrationStringDiscriminantDomain struct {
	target path.Path
	cases  []registrationDiscriminantCase
}

func (r Reader) registrationStringDiscriminantDomainsForRoot(point cfg.Point, root path.Path) []registrationStringDiscriminantDomain {
	rootType, ok := r.discriminatedUnionRootType(point, root)
	if !ok {
		return nil
	}
	out := r.registrationStringDiscriminantDomainsForType(root, nil, rootType, 0)
	sort.Slice(out, func(i, j int) bool {
		return out[i].target.String() < out[j].target.String()
	})
	return out
}

func (r Reader) registrationStringDiscriminantDomainsForType(root path.Path, prefix []segment.Segment, t typ.Type, depth int) []registrationStringDiscriminantDomain {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return nil
	}
	if _, cases, ok := variant.OriginCasesOfType(t); ok && len(cases) >= 2 {
		return registrationStringDiscriminantDomainsForCases(root, prefix, cases)
	}
	var out []registrationStringDiscriminantDomain
	for _, child := range body.StaticTypeChildren(t) {
		nextPrefix := appendSegment(prefix, child.Segment)
		out = append(out, r.registrationStringDiscriminantDomainsForType(root, nextPrefix, child.Type, depth+1)...)
	}
	return out
}

func registrationStringDiscriminantDomainsForCases(root path.Path, prefix []segment.Segment, cases []variant.OriginCase) []registrationStringDiscriminantDomain {
	var out []registrationStringDiscriminantDomain
	domains, ok := variant.LiteralDiscriminantDomainsForCases(cases)
	if !ok {
		return nil
	}
	for _, domain := range domains {
		suffix := domain.Suffix
		target := root.AppendSegments(prefix).AppendSegments(suffix)
		domainCases, ok := registrationStringDiscriminantCasesFor(target, suffix, cases)
		if !ok {
			continue
		}
		out = append(out, registrationStringDiscriminantDomain{target: target, cases: domainCases})
	}
	return out
}

func (r Reader) registrationStringDiscriminantCases(point cfg.Point, target path.Path) ([]registrationDiscriminantCase, bool) {
	for _, anchor := range r.discriminatedUnionAnchors(point, target) {
		_, cases, ok := variant.OriginCasesOfType(anchor.anchorType)
		if !ok || len(cases) < 2 {
			continue
		}
		domainCases, ok := registrationStringDiscriminantCasesFor(target, anchor.suffix, cases)
		if !ok {
			continue
		}
		return domainCases, true
	}
	return nil, false
}

func registrationStringDiscriminantCasesFor(target path.Path, suffix []segment.Segment, cases []variant.OriginCase) ([]registrationDiscriminantCase, bool) {
	out := make([]registrationDiscriminantCase, 0, len(cases))
	seen := make(map[string]struct{}, len(cases))
	for _, c := range cases {
		key, ok := registrationDiscriminantCaseStringKey(c.Type, suffix)
		if !ok {
			return nil, false
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, false
		}
		seen[key] = struct{}{}
		out = append(out, registrationDiscriminantCase{
			index: c.Index,
			name:  registrationDiscriminantCaseName(target, suffix, c.Type),
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

func registrationDiscriminantCaseName(target path.Path, suffix []segment.Segment, caseType typ.Type) string {
	if field, ok := variant.FieldAtPath(caseType, suffix); ok {
		return target.String() + " == " + typeformat.Short(field)
	}
	return typeformat.Short(caseType)
}

func registrationDiscriminantCaseStringKey(caseType typ.Type, suffix []segment.Segment) (string, bool) {
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

func registrationSegmentStringKey(seg segment.Segment) (string, bool) {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return seg.Name, seg.Name != ""
	default:
		return "", false
	}
}

func registrationCaseName(registry, key string) string {
	if identifierName(key) {
		return registry + "." + key
	}
	return registry + "[" + typeformat.Short(typ.LiteralString(key)) + "]"
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

func appendSegment(prefix []segment.Segment, seg segment.Segment) []segment.Segment {
	next := make([]segment.Segment, 0, len(prefix)+1)
	next = append(next, prefix...)
	next = append(next, seg)
	return next
}
