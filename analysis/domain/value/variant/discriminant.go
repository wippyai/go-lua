package variant

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// LiteralDiscriminantCase describes one variant case's literal value at a
// shared discriminant suffix.
type LiteralDiscriminantCase struct {
	Index   int
	Literal *typ.Literal
}

// LiteralDiscriminantDomain describes a path suffix where every case in a
// variant-origin family carries a literal value. Consumers can interpret a
// protocol key against this domain without guessing which field is the tag.
type LiteralDiscriminantDomain struct {
	Suffix []segment.Segment
	Cases  []LiteralDiscriminantCase
}

// LiteralDiscriminantDomains returns every shared literal-valued discriminant
// suffix for t's finite variant-origin family.
func LiteralDiscriminantDomains(t typ.Type) ([]LiteralDiscriminantDomain, bool) {
	_, cases, ok := OriginCasesOfType(t)
	if !ok || len(cases) < 2 {
		return nil, false
	}
	return LiteralDiscriminantDomainsForCases(cases)
}

// LiteralDiscriminantDomainsForCases returns shared literal-valued
// discriminant suffixes for an already materialized origin-case set.
func LiteralDiscriminantDomainsForCases(cases []OriginCase) ([]LiteralDiscriminantDomain, bool) {
	if len(cases) < 2 {
		return nil, false
	}
	var out []LiteralDiscriminantDomain
	for _, suffix := range literalDiscriminantSuffixes(cases[0].Type, nil, 0) {
		domain := LiteralDiscriminantDomain{Suffix: cloneSegments(suffix)}
		for _, c := range cases {
			lit, ok := literalAtPath(c.Type, suffix)
			if !ok || !literalCanDiscriminate(lit) {
				domain.Cases = nil
				break
			}
			domain.Cases = append(domain.Cases, LiteralDiscriminantCase{Index: c.Index, Literal: lit})
		}
		if len(domain.Cases) == len(cases) {
			out = append(out, domain)
		}
	}
	return out, len(out) != 0
}

func literalDiscriminantSuffixes(t typ.Type, prefix []segment.Segment, depth int) [][]segment.Segment {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return nil
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Literal:
		if literalCanDiscriminate(v) && len(prefix) != 0 {
			return [][]segment.Segment{cloneSegments(prefix)}
		}
	case *typ.Alias:
		return literalDiscriminantSuffixes(v.UnaliasedTarget(), prefix, depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return nil
		}
		return literalDiscriminantSuffixes(v.Body, prefix, depth+1)
	case *typ.Instantiated:
		expanded, ok := subst.ExpandInstantiatedChanged(v)
		if !ok {
			return nil
		}
		return literalDiscriminantSuffixes(expanded, prefix, depth+1)
	case *typ.Record:
		var out [][]segment.Segment
		for _, field := range v.Fields {
			if field.Optional {
				continue
			}
			next := append(cloneSegments(prefix), segment.Segment{Kind: segment.SegmentField, Name: field.Name})
			out = append(out, literalDiscriminantSuffixes(field.Type, next, depth+1)...)
		}
		for _, member := range v.StaticMembers {
			if member.Optional || member.Kind != typ.StaticMemberStringIndex {
				continue
			}
			next := append(cloneSegments(prefix), segment.Segment{Kind: segment.SegmentIndexString, Name: member.Name})
			out = append(out, literalDiscriminantSuffixes(member.Type, next, depth+1)...)
		}
		return out
	}
	return nil
}

func literalAtPath(t typ.Type, suffix []segment.Segment) (*typ.Literal, bool) {
	field, ok := fieldAtPath(t, suffix, 0)
	if !ok {
		return nil, false
	}
	lit, ok := unwrap.Annotated(field).(*typ.Literal)
	return lit, ok
}

func literalCanDiscriminate(lit *typ.Literal) bool {
	return lit != nil && (lit.Base == kind.String || lit.Base == kind.Boolean)
}

func cloneSegments(in []segment.Segment) []segment.Segment {
	if len(in) == 0 {
		return nil
	}
	out := make([]segment.Segment, len(in))
	copy(out, in)
	return out
}
