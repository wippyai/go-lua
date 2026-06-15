package projection

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// ElementOf projects the element/value type read from Lua container shapes.
func ElementOf(t typ.Type) (typ.Type, bool) {
	return elementOfDepth(t, 0)
}

// KeyOf projects the key type iterated over Lua container shapes. Arrays and
// tuples key by integer; maps and record map components key by their declared key
// type. A union keys by the union of its members' key types, skipping nil members.
func KeyOf(t typ.Type) (typ.Type, bool) {
	return keyOfDepth(t, 0)
}

func keyOfDepth(t typ.Type, depth int) (typ.Type, bool) {
	if depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	t = unwrap.NormalizeNil(t)
	if t == nil {
		return nil, false
	}
	switch tt := t.(type) {
	case *typ.Annotated:
		return keyOfDepth(tt.Inner, depth+1)
	case *typ.Alias:
		return keyOfDepth(tt.UnaliasedTarget(), depth+1)
	case *typ.Optional:
		return keyOfDepth(tt.Inner, depth+1)
	case *typ.Recursive:
		if tt.Body == nil || tt.Body == t {
			return nil, false
		}
		return keyOfDepth(tt.Body, depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(tt)
		if expanded == nil || expanded == t {
			return nil, false
		}
		return keyOfDepth(expanded, depth+1)
	case *typ.Array:
		return typ.Integer, true
	case *typ.Tuple:
		return typ.Integer, true
	case *typ.Map:
		if unwrap.NormalizeNil(tt.Key) == nil {
			return nil, false
		}
		return tt.Key, true
	case *typ.ReadonlyMap:
		if unwrap.NormalizeNil(tt.Key) == nil {
			return nil, false
		}
		return tt.Key, true
	case *typ.Record:
		if tt.HasMapComponent() && unwrap.NormalizeNil(tt.MapKey) != nil {
			return tt.MapKey, true
		}
		return nil, false
	case *typ.Union:
		members := make([]typ.Type, 0, len(tt.Members))
		for _, member := range tt.Members {
			member = unwrap.NormalizeNil(member)
			if member == nil || member.Kind() == kind.Nil {
				continue
			}
			k, ok := keyOfDepth(member, depth+1)
			if !ok {
				return nil, false
			}
			members = append(members, k)
		}
		if len(members) == 0 {
			return nil, false
		}
		return normalize.UnionForEvidence(members...), true
	default:
		return nil, false
	}
}

func elementOfDepth(t typ.Type, depth int) (typ.Type, bool) {
	if depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	t = unwrap.NormalizeNil(t)
	if t == nil {
		return nil, false
	}
	switch tt := t.(type) {
	case *typ.Annotated:
		return elementOfDepth(tt.Inner, depth+1)
	case *typ.Alias:
		return elementOfDepth(tt.UnaliasedTarget(), depth+1)
	case *typ.Optional:
		return elementOfDepth(tt.Inner, depth+1)
	case *typ.Recursive:
		if tt.Body == nil || tt.Body == t {
			return nil, false
		}
		return elementOfDepth(tt.Body, depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(tt)
		if expanded == nil || expanded == t {
			return nil, false
		}
		return elementOfDepth(expanded, depth+1)
	case *typ.Array:
		if unwrap.NormalizeNil(tt.Element) == nil {
			return nil, false
		}
		return tt.Element, true
	case *typ.Map:
		if unwrap.NormalizeNil(tt.Value) == nil {
			return nil, false
		}
		return tt.Value, true
	case *typ.ReadonlyMap:
		if unwrap.NormalizeNil(tt.Value) == nil {
			return nil, false
		}
		return tt.Value, true
	case *typ.Tuple:
		if len(tt.Elements) == 0 {
			return nil, false
		}
		if len(tt.Elements) == 1 {
			if unwrap.NormalizeNil(tt.Elements[0]) == nil {
				return nil, false
			}
			return tt.Elements[0], true
		}
		return normalize.UnionForEvidence(tt.Elements...), true
	case *typ.Union:
		members := make([]typ.Type, 0, len(tt.Members))
		for _, member := range tt.Members {
			member = unwrap.NormalizeNil(member)
			if member == nil {
				continue
			}
			if member.Kind() == kind.Nil {
				continue
			}
			elem, ok := elementOfDepth(member, depth+1)
			if !ok {
				return nil, false
			}
			members = append(members, elem)
		}
		if len(members) == 0 {
			return nil, false
		}
		return normalize.UnionForEvidence(members...), true
	default:
		return nil, false
	}
}
