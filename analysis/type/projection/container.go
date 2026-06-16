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

func descendContainerDepth(t typ.Type, depth int, project func(typ.Type, int) (typ.Type, bool)) (typ.Type, bool) {
	if depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	t = unwrap.NormalizeNil(t)
	if t == nil {
		return nil, false
	}
	switch tt := t.(type) {
	case *typ.Annotated:
		return descendContainerDepth(tt.Inner, depth+1, project)
	case *typ.Alias:
		return descendContainerDepth(tt.UnaliasedTarget(), depth+1, project)
	case *typ.Optional:
		return descendContainerDepth(tt.Inner, depth+1, project)
	case *typ.Recursive:
		if tt.Body == nil || tt.Body == t {
			return nil, false
		}
		return descendContainerDepth(tt.Body, depth+1, project)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(tt)
		if expanded == nil || expanded == t {
			return nil, false
		}
		return descendContainerDepth(expanded, depth+1, project)
	default:
		return project(t, depth)
	}
}

func keyOfDepth(t typ.Type, depth int) (typ.Type, bool) {
	return descendContainerDepth(t, depth, keyOfDepthProject)
}

func keyOfDepthProject(t typ.Type, depth int) (typ.Type, bool) {
	switch tt := t.(type) {
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
	return descendContainerDepth(t, depth, elementOfDepthProject)
}

func elementOfDepthProject(t typ.Type, depth int) (typ.Type, bool) {
	switch tt := t.(type) {
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
