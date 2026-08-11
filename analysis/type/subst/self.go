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
	return rewriteSelf(t, selfType, selfRewriteMethodType)
}

type selfRewriteMode uint8

const (
	selfRewriteMethodType selfRewriteMode = iota
	selfRewriteRuntimeValue
)

type selfScanMode uint8

const (
	selfScanSubstitutable selfScanMode = iota
	selfScanSurface
)

func containsSubstitutableSelf(t typ.Type) bool {
	if typ.ContainsRecursive(t) && !containsSurfaceSelf(t) {
		return false
	}
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

func newSelfContainsScan() selfContainsScan {
	return selfContainsScan{
		scanner: inspect.NewScanner(inspect.ScanOptions{
			Seen: inspect.NewIdentitySeen(nil),
		}),
	}
}

func (s *selfContainsScan) contains(t typ.Type, mode selfScanMode) bool {
	stack := []typ.Type{t}
	for len(stack) != 0 {
		i := len(stack) - 1
		current := unwrap.Annotated(stack[i])
		stack = stack[:i]
		if current == nil {
			continue
		}
		if current.Kind() == kind.Self {
			return true
		}
		if _, ok := current.(*typ.Interface); ok {
			continue
		}
		if _, ok := current.(*typ.Recursive); ok {
			continue
		}
		if mode == selfScanSubstitutable && typ.ContainsRecursive(current) && !containsSurfaceSelf(current) {
			continue
		}
		if s.scanner != nil && !s.scanner.Enter(current) {
			continue
		}
		switch v := current.(type) {
		case *typ.Optional:
			stack = append(stack, v.Inner)
		case *typ.Union:
			stack = append(stack, v.Members...)
		case *typ.Intersection:
			stack = append(stack, v.Members...)
		case *typ.Array:
			stack = append(stack, v.Element)
		case *typ.Map:
			stack = append(stack, v.Key, v.Value)
		case *typ.ReadonlyMap:
			stack = append(stack, v.Key, v.Value)
		case *typ.Tuple:
			stack = append(stack, v.Elements...)
		case *typ.Function:
			for _, param := range v.Params {
				stack = append(stack, param.Type)
			}
			stack = append(stack, v.Variadic)
			stack = append(stack, v.Returns...)
		case *typ.Record:
			for _, field := range v.Fields {
				stack = append(stack, field.Type)
			}
			for _, member := range v.StaticMembers {
				stack = append(stack, member.Type)
			}
			stack = append(stack, v.Metatable)
			if v.HasMapComponent() {
				stack = append(stack, v.MapKey, v.MapValue)
			}
		case *typ.Meta:
			stack = append(stack, v.Of)
		case *typ.TypeParam:
			stack = append(stack, v.Constraint)
		case *typ.Generic:
			for _, parameter := range v.TypeParams {
				if parameter != nil {
					stack = append(stack, parameter.Constraint)
				}
			}
			stack = append(stack, v.Body)
		case *typ.Alias:
			stack = append(stack, v.Target)
		case *typ.Instantiated:
			stack = append(stack, v.TypeArgs...)
		}
	}
	return false
}

// SelfValue replaces free Self references in a runtime value type. Nested
// function and interface types bind their own Self, so substitution stops at
// those boundaries.
func SelfValue(t typ.Type, selfType typ.Type) typ.Type {
	if t == nil || selfType == nil {
		return t
	}
	return rewriteSelf(t, selfType, selfRewriteRuntimeValue)
}

func rewriteSelf(t typ.Type, selfType typ.Type, mode selfRewriteMode) typ.Type {
	return transform.Rewrite(t, func(n typ.Type) (typ.Type, bool) {
		if n.Kind() == kind.Self {
			return selfType, true
		}
		if _, ok := n.(*typ.Interface); ok {
			return n, true
		}
		if mode == selfRewriteRuntimeValue {
			if _, ok := n.(*typ.Function); ok {
				return n, true
			}
		}
		return nil, false
	})
}
