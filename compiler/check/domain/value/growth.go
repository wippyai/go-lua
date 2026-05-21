package value

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// HasHigherOrderGrowthRisk reports whether a type can produce non-monotone
// higher-order structural growth across abstract-interpretation iterations.
func HasHigherOrderGrowthRisk(t typ.Type) bool {
	if t == nil {
		return false
	}
	state := newGrowthScanState()
	return state.hasHigherOrderGrowthRisk(t, typ.NewGuard())
}

type growthScanKey struct {
	hash uint64
	kind kind.Kind
}

type methodScanKey struct {
	node  growthScanKey
	owner growthScanKey
}

type growthScanState struct {
	riskSeen       map[growthScanKey]bool
	riskValue      map[growthScanKey]bool
	functionSeen   map[growthScanKey]bool
	functionValue  map[growthScanKey]bool
	methodSeen     map[methodScanKey]bool
	methodValue    map[methodScanKey]bool
	selfMethodSeen map[growthScanKey]bool
	selfMethodVal  map[growthScanKey]bool
}

func newGrowthScanState() *growthScanState {
	return &growthScanState{
		riskSeen:       make(map[growthScanKey]bool),
		riskValue:      make(map[growthScanKey]bool),
		functionSeen:   make(map[growthScanKey]bool),
		functionValue:  make(map[growthScanKey]bool),
		methodSeen:     make(map[methodScanKey]bool),
		methodValue:    make(map[methodScanKey]bool),
		selfMethodSeen: make(map[growthScanKey]bool),
		selfMethodVal:  make(map[growthScanKey]bool),
	}
}

func growthKey(t typ.Type) growthScanKey {
	if t == nil {
		return growthScanKey{}
	}
	return growthScanKey{hash: t.Hash(), kind: t.Kind()}
}

func stripAnnotated(t typ.Type) typ.Type {
	for {
		ann, ok := t.(*typ.Annotated)
		if !ok || ann.Inner == nil || ann.Inner == t {
			return t
		}
		t = ann.Inner
	}
}

func (s *growthScanState) hasHigherOrderGrowthRisk(t typ.Type, guard internal.RecursionGuard) bool {
	t = stripAnnotated(t)
	if t == nil {
		return false
	}
	key := growthKey(t)
	if s.riskSeen[key] {
		return s.riskValue[key]
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}

	result := false
	switch n := t.(type) {
	case *typ.Optional:
		result = s.hasHigherOrderGrowthRisk(n.Inner, next)
	case *typ.Union:
		result = s.anyTypeHasRisk(n.Members, next)
	case *typ.Intersection:
		result = s.anyTypeHasRisk(n.Members, next)
	case *typ.Array:
		result = s.hasHigherOrderGrowthRisk(n.Element, next)
	case *typ.Map:
		result = s.hasHigherOrderGrowthRisk(n.Key, next) || s.hasHigherOrderGrowthRisk(n.Value, next)
	case *typ.Tuple:
		result = s.anyTypeHasRisk(n.Elements, next)
	case *typ.Function:
		for _, ret := range n.Returns {
			if s.containsFunction(ret, typ.NewGuard()) {
				result = true
				break
			}
		}
		if !result {
			for _, p := range n.Params {
				if s.hasHigherOrderGrowthRisk(p.Type, next) {
					result = true
					break
				}
			}
		}
		if !result {
			result = s.anyTypeHasRisk(n.Returns, next) ||
				(n.Variadic != nil && s.hasHigherOrderGrowthRisk(n.Variadic, next))
		}
	case *typ.Record:
		result = s.recordHasSelfRecursiveMethod(n)
		if !result {
			for _, f := range n.Fields {
				if s.hasHigherOrderGrowthRisk(f.Type, next) {
					result = true
					break
				}
			}
		}
		if !result {
			result = (n.Metatable != nil && s.hasHigherOrderGrowthRisk(n.Metatable, next)) ||
				(n.HasMapComponent() && (s.hasHigherOrderGrowthRisk(n.MapKey, next) || s.hasHigherOrderGrowthRisk(n.MapValue, next)))
		}
	case *typ.Alias:
		result = s.hasHigherOrderGrowthRisk(n.Target, next)
	case *typ.Instantiated:
		result = s.anyTypeHasRisk(n.TypeArgs, next)
	case *typ.Interface:
		for _, m := range n.Methods {
			if m.Type != nil && s.hasHigherOrderGrowthRisk(m.Type, next) {
				result = true
				break
			}
		}
	}

	s.riskSeen[key] = true
	s.riskValue[key] = result
	return result
}

func (s *growthScanState) anyTypeHasRisk(types []typ.Type, guard internal.RecursionGuard) bool {
	for _, t := range types {
		if s.hasHigherOrderGrowthRisk(t, guard) {
			return true
		}
	}
	return false
}

func (s *growthScanState) containsFunction(t typ.Type, guard internal.RecursionGuard) bool {
	t = stripAnnotated(t)
	if t == nil {
		return false
	}
	key := growthKey(t)
	if s.functionSeen[key] {
		return s.functionValue[key]
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}

	result := false
	switch n := t.(type) {
	case *typ.Function:
		result = true
	case *typ.Interface:
		result = false
	case *typ.Optional:
		result = s.containsFunction(n.Inner, next)
	case *typ.Union:
		result = s.anyTypeContainsFunction(n.Members, next)
	case *typ.Intersection:
		result = s.anyTypeContainsFunction(n.Members, next)
	case *typ.Array:
		result = s.containsFunction(n.Element, next)
	case *typ.Map:
		result = s.containsFunction(n.Key, next) || s.containsFunction(n.Value, next)
	case *typ.Tuple:
		result = s.anyTypeContainsFunction(n.Elements, next)
	case *typ.Record:
		for _, f := range n.Fields {
			if s.containsFunction(f.Type, next) {
				result = true
				break
			}
		}
		if !result {
			result = (n.Metatable != nil && s.containsFunction(n.Metatable, next)) ||
				(n.HasMapComponent() && (s.containsFunction(n.MapKey, next) || s.containsFunction(n.MapValue, next)))
		}
	case *typ.Alias:
		result = s.containsFunction(n.Target, next)
	case *typ.Instantiated:
		result = s.anyTypeContainsFunction(n.TypeArgs, next)
	}

	s.functionSeen[key] = true
	s.functionValue[key] = result
	return result
}

func (s *growthScanState) anyTypeContainsFunction(types []typ.Type, guard internal.RecursionGuard) bool {
	for _, t := range types {
		if s.containsFunction(t, guard) {
			return true
		}
	}
	return false
}

func (s *growthScanState) recordHasSelfRecursiveMethod(r *typ.Record) bool {
	if r == nil {
		return false
	}
	key := growthKey(r)
	if s.selfMethodSeen[key] {
		return s.selfMethodVal[key]
	}

	result := false
	for _, f := range r.Fields {
		if s.methodTypeHasSelfRecursiveReturn(f.Type, r, typ.NewGuard()) {
			result = true
			break
		}
	}
	if !result && r.HasMapComponent() {
		result = s.methodTypeHasSelfRecursiveReturn(r.MapValue, r, typ.NewGuard())
	}

	s.selfMethodSeen[key] = true
	s.selfMethodVal[key] = result
	return result
}

func (s *growthScanState) methodTypeHasSelfRecursiveReturn(t typ.Type, owner *typ.Record, guard internal.RecursionGuard) bool {
	t = stripAnnotated(t)
	if t == nil || owner == nil {
		return false
	}
	key := methodScanKey{node: growthKey(t), owner: growthKey(owner)}
	if _, ok := t.(*typ.Function); !ok && !s.containsFunction(t, typ.NewGuard()) {
		return false
	}
	if s.methodSeen[key] {
		return s.methodValue[key]
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}

	result := false
	switch n := t.(type) {
	case *typ.Interface:
		result = false
	case *typ.Function:
		for _, ret := range n.Returns {
			if ret == nil {
				continue
			}
			if subtype.IsSubtype(ret, owner) || subtype.IsSubtype(owner, ret) ||
				ExtendsRecord(ret, owner) || ExtendsRecord(owner, ret) {
				result = true
				break
			}
		}
	case *typ.Optional:
		result = s.methodTypeHasSelfRecursiveReturn(n.Inner, owner, next)
	case *typ.Union:
		result = s.anyMethodTypeHasSelfRecursiveReturn(n.Members, owner, next)
	case *typ.Intersection:
		result = s.anyMethodTypeHasSelfRecursiveReturn(n.Members, owner, next)
	case *typ.Array:
		result = s.methodTypeHasSelfRecursiveReturn(n.Element, owner, next)
	case *typ.Map:
		result = s.methodTypeHasSelfRecursiveReturn(n.Key, owner, next) ||
			s.methodTypeHasSelfRecursiveReturn(n.Value, owner, next)
	case *typ.Tuple:
		result = s.anyMethodTypeHasSelfRecursiveReturn(n.Elements, owner, next)
	case *typ.Record:
		for _, f := range n.Fields {
			if s.methodTypeHasSelfRecursiveReturn(f.Type, owner, next) {
				result = true
				break
			}
		}
		if !result {
			result = (n.Metatable != nil && s.methodTypeHasSelfRecursiveReturn(n.Metatable, owner, next)) ||
				(n.HasMapComponent() && (s.methodTypeHasSelfRecursiveReturn(n.MapKey, owner, next) ||
					s.methodTypeHasSelfRecursiveReturn(n.MapValue, owner, next)))
		}
	case *typ.Alias:
		result = s.methodTypeHasSelfRecursiveReturn(n.Target, owner, next)
	case *typ.Instantiated:
		result = s.anyMethodTypeHasSelfRecursiveReturn(n.TypeArgs, owner, next)
	}

	s.methodSeen[key] = true
	s.methodValue[key] = result
	return result
}

func (s *growthScanState) anyMethodTypeHasSelfRecursiveReturn(types []typ.Type, owner *typ.Record, guard internal.RecursionGuard) bool {
	for _, t := range types {
		if s.methodTypeHasSelfRecursiveReturn(t, owner, guard) {
			return true
		}
	}
	return false
}
