package subst

import (
	"github.com/wippyai/go-lua/analysis/type/inspect"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/transform"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// Self replaces Self type references with a concrete type.
// Does not recurse into Interface types because Self inside an Interface
// is a separate binding that refers to that Interface's implementor.
func Self(t typ.Type, selfType typ.Type) typ.Type {
	if t == nil || selfType == nil {
		return t
	}
	if !containsSubstitutableSelf(t) {
		return t
	}
	return transform.Rewrite(t, func(n typ.Type) (typ.Type, bool) {
		if n.Kind() == kind.Self {
			return selfType, true
		}
		if _, ok := n.(*typ.Interface); ok {
			return n, true
		}
		return nil, false
	})
}

type selfScanMode uint8

const (
	selfScanSubstitutable selfScanMode = iota
	selfScanSurface
)

func containsSubstitutableSelf(t typ.Type) bool {
	scan := newSelfContainsScan()
	return scan.contains(t, selfScanSubstitutable)
}

func containsSurfaceSelf(t typ.Type) bool {
	scan := newSelfContainsScan()
	return scan.contains(t, selfScanSurface)
}

type selfContainsScan struct {
	scanner *inspect.Scanner
}

func newSelfContainsScan() *selfContainsScan {
	return &selfContainsScan{
		scanner: inspect.NewScanner(inspect.ScanOptions{
			Seen:      inspect.NewIdentitySeen(nil),
			MaxEnters: selfSubstitutionNodeBudget,
		}),
	}
}

func (s *selfContainsScan) contains(t typ.Type, mode selfScanMode) bool {
	if t == nil {
		return false
	}
	t = unwrap.Annotated(t)
	if t == nil {
		return false
	}
	if t.Kind() == kind.Self {
		return true
	}
	if _, ok := t.(*typ.Interface); ok {
		return false
	}
	if _, ok := t.(*typ.Recursive); ok {
		return false
	}
	if mode == selfScanSubstitutable && inspect.ContainsRecursive(t) && !containsSurfaceSelf(t) {
		return false
	}
	if !s.scanner.Enter(t) {
		return false
	}
	switch v := t.(type) {
	case *typ.Optional:
		return s.contains(v.Inner, mode)
	case *typ.Union:
		for _, member := range v.Members {
			if s.contains(member, mode) {
				return true
			}
		}
	case *typ.Intersection:
		for _, member := range v.Members {
			if s.contains(member, mode) {
				return true
			}
		}
	case *typ.Array:
		return s.contains(v.Element, mode)
	case *typ.Map:
		return s.contains(v.Key, mode) ||
			s.contains(v.Value, mode)
	case *typ.ReadonlyMap:
		return s.contains(v.Key, mode) ||
			s.contains(v.Value, mode)
	case *typ.Tuple:
		for _, elem := range v.Elements {
			if s.contains(elem, mode) {
				return true
			}
		}
	case *typ.Function:
		for _, param := range v.Params {
			if s.contains(param.Type, mode) {
				return true
			}
		}
		if s.contains(v.Variadic, mode) {
			return true
		}
		for _, ret := range v.Returns {
			if s.contains(ret, mode) {
				return true
			}
		}
	case *typ.Record:
		for _, field := range v.Fields {
			if s.contains(field.Type, mode) {
				return true
			}
		}
		for _, member := range v.StaticMembers {
			if s.contains(member.Type, mode) {
				return true
			}
		}
		if s.contains(v.Metatable, mode) {
			return true
		}
		if v.HasMapComponent() {
			return s.contains(v.MapKey, mode) ||
				s.contains(v.MapValue, mode)
		}
	case *typ.Alias:
		return s.contains(v.Target, mode)
	case *typ.Instantiated:
		for _, arg := range v.TypeArgs {
			if s.contains(arg, mode) {
				return true
			}
		}
	}
	return false
}

const selfSubstitutionNodeBudget = 2048

// SelfValue replaces free Self references in a runtime value type. Nested
// function and interface types bind their own Self, so substitution stops at
// those boundaries.
func SelfValue(t typ.Type, selfType typ.Type) typ.Type {
	if t == nil || selfType == nil {
		return t
	}
	return transform.Rewrite(t, func(n typ.Type) (typ.Type, bool) {
		if n.Kind() == kind.Self {
			return selfType, true
		}
		switch n.(type) {
		case *typ.Function, *typ.Interface:
			return n, true
		}
		return nil, false
	})
}
