package value

import (
	"github.com/wippyai/go-lua/internal"
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
	node typ.Type
}

type methodScanKey struct {
	node  growthScanKey
	owner *typ.Record
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
	higherSeen     map[growthScanKey]bool
	higherValue    map[growthScanKey]bool
	callableSeen   map[growthScanKey]bool
	callableValue  map[growthScanKey]bool
	recordSeen     map[growthScanKey]bool
	recordValue    map[growthScanKey]bool
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
		higherSeen:     make(map[growthScanKey]bool),
		higherValue:    make(map[growthScanKey]bool),
		callableSeen:   make(map[growthScanKey]bool),
		callableValue:  make(map[growthScanKey]bool),
		recordSeen:     make(map[growthScanKey]bool),
		recordValue:    make(map[growthScanKey]bool),
	}
}

func growthKey(t typ.Type) growthScanKey {
	return growthScanKey{node: t}
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
	s.riskSeen[key] = true
	s.riskValue[key] = false
	next := guard

	result := false
	switch n := t.(type) {
	case *typ.Optional:
		result = s.hasHigherOrderGrowthRisk(n.Inner, next)
	case *typ.Union:
		result = s.anyTypeHasRisk(n.Members, next)
	case *typ.Intersection:
		result = s.anyTypeHasRisk(n.Members, next)
	case *typ.Array:
		result = s.hasHigherOrderCallableSurface(n.Element, next)
	case *typ.Map:
		result = s.hasHigherOrderCallableSurface(n.Key, next) || s.hasHigherOrderCallableSurface(n.Value, next)
	case *typ.Tuple:
		result = s.anyTypeHasCallableSurface(n.Elements, next)
	case *typ.Function:
		for _, ret := range n.Returns {
			if s.returnHasHigherOrderSurface(ret, next) {
				result = true
				break
			}
		}
		if !result {
			for _, p := range n.Params {
				if s.hasHigherOrderCallableSurface(p.Type, next) {
					result = true
					break
				}
			}
		}
		if !result {
			result = n.Variadic != nil && s.hasHigherOrderCallableSurface(n.Variadic, next)
		}
	case *typ.Record:
		if s.recordHasCallableSurface(n) {
			result = s.recordHasSelfRecursiveMethod(n) || s.recordHasHigherOrderCallableSurface(n, next)
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

func (s *growthScanState) recordHasHigherOrderCallableSurface(r *typ.Record, guard internal.RecursionGuard) bool {
	if r == nil {
		return false
	}
	for _, f := range r.Fields {
		if s.hasHigherOrderCallableSurface(f.Type, guard) {
			return true
		}
	}
	if r.Metatable != nil && s.hasHigherOrderCallableSurface(r.Metatable, guard) {
		return true
	}
	return r.HasMapComponent() && s.hasHigherOrderCallableSurface(r.MapValue, guard)
}

func (s *growthScanState) anyTypeHasCallableSurface(types []typ.Type, guard internal.RecursionGuard) bool {
	for _, t := range types {
		if s.hasHigherOrderCallableSurface(t, guard) {
			return true
		}
	}
	return false
}

func (s *growthScanState) hasHigherOrderCallableSurface(t typ.Type, guard internal.RecursionGuard) bool {
	t = stripAnnotated(t)
	if t == nil {
		return false
	}
	switch t.(type) {
	case *typ.Function, *typ.Optional, *typ.Union, *typ.Intersection, *typ.Alias:
	default:
		return false
	}
	if !s.hasCallableTypeSurface(t) {
		return false
	}
	key := growthKey(t)
	if s.higherSeen[key] {
		return s.higherValue[key]
	}
	s.higherSeen[key] = true
	s.higherValue[key] = false

	result := false
	switch n := t.(type) {
	case *typ.Function:
		result = s.hasHigherOrderGrowthRisk(n, guard)
	case *typ.Optional:
		result = s.hasHigherOrderCallableSurface(n.Inner, guard)
	case *typ.Union:
		result = s.anyTypeHasCallableSurface(n.Members, guard)
	case *typ.Intersection:
		result = s.anyTypeHasCallableSurface(n.Members, guard)
	case *typ.Alias:
		result = s.hasHigherOrderCallableSurface(n.Target, guard)
	}
	s.higherValue[key] = result
	return result
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
	s.functionSeen[key] = true
	s.functionValue[key] = false
	next := guard

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
	s.selfMethodSeen[key] = true
	s.selfMethodVal[key] = false
	if !s.recordHasCallableSurface(r) {
		return false
	}

	result := false
	for _, f := range r.Fields {
		if !s.hasCallableTypeSurface(f.Type) {
			continue
		}
		if s.methodTypeHasSelfRecursiveReturn(f.Type, r, typ.NewGuard()) {
			result = true
			break
		}
	}
	if !result && r.HasMapComponent() && s.hasCallableTypeSurface(r.MapValue) {
		result = s.methodTypeHasSelfRecursiveReturn(r.MapValue, r, typ.NewGuard())
	}

	s.selfMethodVal[key] = result
	return result
}

func (s *growthScanState) hasCallableTypeSurface(t typ.Type) bool {
	t = stripAnnotated(t)
	if t == nil {
		return false
	}
	switch t.(type) {
	case *typ.Function:
		return true
	case *typ.Optional, *typ.Union, *typ.Intersection, *typ.Alias:
	default:
		return false
	}
	key := growthKey(t)
	if s.callableSeen[key] {
		return s.callableValue[key]
	}
	s.callableSeen[key] = true
	s.callableValue[key] = false
	result := false
	switch n := t.(type) {
	case *typ.Optional:
		result = s.hasCallableTypeSurface(n.Inner)
	case *typ.Union:
		result = s.anyTypeHasCallableSurfaceOnly(n.Members)
	case *typ.Intersection:
		result = s.anyTypeHasCallableSurfaceOnly(n.Members)
	case *typ.Alias:
		result = s.hasCallableTypeSurface(n.Target)
	}
	s.callableValue[key] = result
	return result
}

func (s *growthScanState) anyTypeHasCallableSurfaceOnly(types []typ.Type) bool {
	for _, t := range types {
		if s.hasCallableTypeSurface(t) {
			return true
		}
	}
	return false
}

func (s *growthScanState) returnHasHigherOrderSurface(t typ.Type, guard internal.RecursionGuard) bool {
	t = stripAnnotated(t)
	if t == nil {
		return false
	}
	switch n := t.(type) {
	case *typ.Function:
		return true
	case *typ.Optional:
		return s.returnHasHigherOrderSurface(n.Inner, guard)
	case *typ.Union:
		for _, member := range n.Members {
			if s.returnHasHigherOrderSurface(member, guard) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, member := range n.Members {
			if s.returnHasHigherOrderSurface(member, guard) {
				return true
			}
		}
		return false
	case *typ.Array:
		return s.hasHigherOrderCallableSurface(n.Element, guard)
	case *typ.Map:
		return s.hasHigherOrderCallableSurface(n.Key, guard) ||
			s.hasHigherOrderCallableSurface(n.Value, guard)
	case *typ.Tuple:
		return s.anyTypeHasCallableSurface(n.Elements, guard)
	case *typ.Record:
		return s.recordHasCallableSurface(n)
	case *typ.Alias:
		return s.returnHasHigherOrderSurface(n.Target, guard)
	default:
		return false
	}
}

func (s *growthScanState) recordHasCallableSurface(r *typ.Record) bool {
	if r == nil {
		return false
	}
	if !typ.RecordHasCallableSurface(r) {
		return false
	}
	key := growthKey(r)
	if s.recordSeen[key] {
		return s.recordValue[key]
	}
	s.recordSeen[key] = true
	s.recordValue[key] = false

	result := false
	for _, field := range r.Fields {
		if s.hasCallableTypeSurface(field.Type) {
			result = true
			break
		}
	}
	if !result && r.Metatable != nil && s.hasCallableTypeSurface(r.Metatable) {
		result = true
	}
	if !result && r.HasMapComponent() && s.hasCallableTypeSurface(r.MapValue) {
		result = true
	}
	s.recordValue[key] = result
	return result
}

func (s *growthScanState) methodTypeHasSelfRecursiveReturn(t typ.Type, owner *typ.Record, guard internal.RecursionGuard) bool {
	t = stripAnnotated(t)
	if t == nil || owner == nil {
		return false
	}
	key := methodScanKey{node: growthKey(t), owner: owner}
	if s.methodSeen[key] {
		return s.methodValue[key]
	}
	s.methodSeen[key] = true
	s.methodValue[key] = false
	next := guard

	result := false
	switch n := t.(type) {
	case *typ.Interface:
		result = false
	case *typ.Function:
		for _, ret := range n.Returns {
			if ret == nil {
				continue
			}
			if returnTypeMentionsOwnerRecord(ret, owner, next) {
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
	case *typ.Alias:
		result = s.methodTypeHasSelfRecursiveReturn(n.Target, owner, next)
	}

	s.methodValue[key] = result
	return result
}

func returnTypeMentionsOwnerRecord(t typ.Type, owner *typ.Record, guard internal.RecursionGuard) bool {
	t = stripAnnotated(t)
	if t == nil || owner == nil {
		return false
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}
	switch n := UnwrapStructuralShape(t).(type) {
	case *typ.Record:
		return recordCouldBeOwnerShape(n, owner) || ContainsNestedStructuralShape(n, owner)
	case *typ.Optional:
		return returnTypeMentionsOwnerRecord(n.Inner, owner, next)
	case *typ.Union:
		for _, member := range n.Members {
			if returnTypeMentionsOwnerRecord(member, owner, next) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, member := range n.Members {
			if returnTypeMentionsOwnerRecord(member, owner, next) {
				return true
			}
		}
		return false
	case *typ.Array:
		return returnTypeMentionsOwnerRecord(n.Element, owner, next)
	case *typ.Map:
		return returnTypeMentionsOwnerRecord(n.Key, owner, next) ||
			returnTypeMentionsOwnerRecord(n.Value, owner, next)
	case *typ.Tuple:
		for _, elem := range n.Elements {
			if returnTypeMentionsOwnerRecord(elem, owner, next) {
				return true
			}
		}
		return false
	case *typ.Alias:
		return returnTypeMentionsOwnerRecord(n.Target, owner, next)
	case *typ.Instantiated:
		for _, arg := range n.TypeArgs {
			if returnTypeMentionsOwnerRecord(arg, owner, next) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func recordCouldBeOwnerShape(candidate, owner *typ.Record) bool {
	if candidate == nil || owner == nil {
		return false
	}
	if ShallowStructuralShapeEquals(candidate, owner) {
		return true
	}
	if owner.HasMapComponent() && !candidate.HasMapComponent() {
		return false
	}
	for _, field := range owner.Fields {
		if candidate.GetField(field.Name) == nil {
			return false
		}
	}
	return true
}

func (s *growthScanState) anyMethodTypeHasSelfRecursiveReturn(types []typ.Type, owner *typ.Record, guard internal.RecursionGuard) bool {
	for _, t := range types {
		if s.methodTypeHasSelfRecursiveReturn(t, owner, guard) {
			return true
		}
	}
	return false
}
