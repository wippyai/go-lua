package narrow

import (
	"sort"
	"strconv"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// DiscriminantDomain is the closed set of literal tags for a discriminated union.
//
// The domain is closed only when the source type is a union of record variants
// and every variant has the same required field with an exact literal type.
// Broader fields, optional tags, dynamic members, nil, any, and unknown all
// make the domain open, because missing-case diagnostics would not be provable.
type DiscriminantDomain struct {
	Field    string
	Literals []*typ.Literal
}

// ClosedDiscriminantDomain returns the closed literal tag set for field.
func ClosedDiscriminantDomain(t typ.Type, field string) (DiscriminantDomain, bool) {
	if t == nil || field == "" {
		return DiscriminantDomain{}, false
	}

	u, ok := closedUnion(t)
	if !ok || len(u.Members) < 2 {
		return DiscriminantDomain{}, false
	}

	literals := make([]*typ.Literal, 0, len(u.Members))
	seen := make(map[string]struct{}, len(u.Members))
	for _, member := range u.Members {
		lit, ok := discriminantLiteral(member, field)
		if !ok {
			return DiscriminantDomain{}, false
		}
		key := LiteralKey(lit)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		literals = append(literals, lit)
	}
	if len(literals) < 2 {
		return DiscriminantDomain{}, false
	}

	sort.Slice(literals, func(i, j int) bool {
		left := literals[i]
		right := literals[j]
		if left.Base != right.Base {
			return left.Base < right.Base
		}
		return left.String() < right.String()
	})
	return DiscriminantDomain{Field: field, Literals: literals}, true
}

// Contains reports whether lit is in this closed discriminant domain.
func (d DiscriminantDomain) Contains(lit *typ.Literal) bool {
	if lit == nil {
		return false
	}
	key := LiteralKey(lit)
	for _, candidate := range d.Literals {
		if LiteralKey(candidate) == key {
			return true
		}
	}
	return false
}

// Missing returns domain literals that are not covered by handled.
func (d DiscriminantDomain) Missing(handled []*typ.Literal) []*typ.Literal {
	if len(d.Literals) == 0 {
		return nil
	}
	covered := make(map[string]struct{}, len(handled))
	for _, lit := range handled {
		if lit == nil {
			continue
		}
		covered[LiteralKey(lit)] = struct{}{}
	}

	missing := make([]*typ.Literal, 0, len(d.Literals))
	for _, lit := range d.Literals {
		if _, ok := covered[LiteralKey(lit)]; !ok {
			missing = append(missing, lit)
		}
	}
	return missing
}

// LiteralKey is a stable equality key for literal singleton types.
func LiteralKey(lit *typ.Literal) string {
	if lit == nil {
		return ""
	}
	return strconv.Itoa(int(lit.Base)) + ":" + lit.String()
}

func closedUnion(t typ.Type) (*typ.Union, bool) {
	t = unwrap.Alias(t)
	if t == nil {
		return nil, false
	}
	if expanded := unwrap.Instantiated(t); expanded != t {
		t = unwrap.Alias(expanded)
	}
	u, ok := t.(*typ.Union)
	if !ok {
		return nil, false
	}
	for _, member := range u.Members {
		if member == nil {
			return nil, false
		}
		switch member.Kind() {
		case kind.Any, kind.Unknown, kind.Nil, kind.Optional, kind.TypeParam, kind.TypeVar:
			return nil, false
		}
		if member.Kind().IsPlaceholder() {
			return nil, false
		}
	}
	return u, true
}

func discriminantLiteral(t typ.Type, field string) (*typ.Literal, bool) {
	t = unwrap.Alias(t)
	if t == nil {
		return nil, false
	}
	if expanded := unwrap.Instantiated(t); expanded != t {
		t = unwrap.Alias(expanded)
	}
	rec, ok := t.(*typ.Record)
	if !ok {
		return nil, false
	}
	f := rec.GetField(field)
	if f == nil || f.Optional || f.Type == nil {
		return nil, false
	}
	lit, ok := unwrap.Alias(f.Type).(*typ.Literal)
	if !ok {
		return nil, false
	}
	return lit, true
}
