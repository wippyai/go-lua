package gradual

import (
	"github.com/wippyai/go-lua/analysis/internal/recursion"
	"github.com/wippyai/go-lua/analysis/type/kind"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// PruneSoftUnionMembers removes soft placeholder members from unions when
// any non-soft member is present. This helps inference avoid contaminating
// concrete types with placeholder unions.
func PruneSoftUnionMembers(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	state := getSoftPruneState()
	defer putSoftPruneState(state)
	guard := typ.NewGuard()
	return pruneSoftUnionMembersMemo(t, guard, state.memo, state.visiting, state.softMemo)
}

func pruneSoftUnionMembersMemo(
	t typ.Type,
	guard recursion.Guard,
	memo map[typ.Type]typ.Type,
	visiting map[typ.Type]struct{},
	softMemo map[typ.Type]bool,
) typ.Type {
	if t == nil {
		return t
	}
	if cached, ok := memo[t]; ok {
		return cached
	}
	if _, ok := visiting[t]; ok {
		return t
	}
	next, ok := guard.Enter(t)
	if !ok {
		return t
	}
	visiting[t] = struct{}{}

	var out typ.Type
	switch node := unwrapTransparentSoft(t).(type) {
	case *typ.Function:
		out = pruneSoftFunction(node, t, next, memo, visiting, softMemo)
	case *typ.Record:
		out = pruneSoftRecord(node, t, next, memo, visiting, softMemo)
	case *typ.Union:
		var rewritten []typ.Type
		softCount := 0
		changed := false
		for idx, m := range node.Members {
			pm := pruneSoftUnionMembersMemo(m, next, memo, visiting, softMemo)
			if pm != m {
				if rewritten == nil {
					rewritten = make([]typ.Type, len(node.Members))
					copy(rewritten, node.Members)
				}
				rewritten[idx] = pm
				changed = true
			} else if rewritten != nil {
				rewritten[idx] = m
			}
			if isSoftWithMemo(pm, SoftPlaceholderPolicy, softMemo) {
				softCount++
			}
		}
		if softCount > 0 && softCount < len(node.Members) {
			members := node.Members
			if rewritten != nil {
				members = rewritten
			}
			nonSoftMembers := make([]typ.Type, 0, len(node.Members)-softCount)
			hasNonNilConcreteMember := false
			for _, member := range members {
				if !isSoftWithMemo(member, SoftPlaceholderPolicy, softMemo) {
					nonSoftMembers = append(nonSoftMembers, member)
					if member != nil && member.Kind() != kind.Nil {
						hasNonNilConcreteMember = true
					}
				}
			}
			if hasNonNilConcreteMember {
				out = typ.NewUnion(nonSoftMembers...)
				break
			}
		}
		if !changed {
			out = t
			break
		}
		members := node.Members
		if rewritten != nil {
			members = rewritten
		}
		out = typ.NewUnion(members...)
	case *typ.Optional:
		if node.Inner == nil {
			out = t
			break
		}
		inner := pruneSoftUnionMembersMemo(node.Inner, next, memo, visiting, softMemo)
		if inner == node.Inner {
			out = t
			break
		}
		out = typ.NewOptional(inner)
	case *typ.Array:
		elem := pruneSoftUnionMembersMemo(node.Element, next, memo, visiting, softMemo)
		if elem == node.Element {
			out = t
			break
		}
		out = typ.NewArray(elem)
	case *typ.Map:
		key := pruneSoftUnionMembersMemo(node.Key, next, memo, visiting, softMemo)
		val := pruneSoftUnionMembersMemo(node.Value, next, memo, visiting, softMemo)
		if key == node.Key && val == node.Value {
			out = t
			break
		}
		out = typetable.NewMap(key, val)
	case *typ.ReadonlyMap:
		key := pruneSoftUnionMembersMemo(node.Key, next, memo, visiting, softMemo)
		val := pruneSoftUnionMembersMemo(node.Value, next, memo, visiting, softMemo)
		if key == node.Key && val == node.Value {
			out = t
			break
		}
		out = typetable.NewReadonlyMap(key, val)
	case *typ.Tuple:
		var elems []typ.Type
		for i, e := range node.Elements {
			newElem := pruneSoftUnionMembersMemo(e, next, memo, visiting, softMemo)
			if newElem != e {
				if elems == nil {
					elems = make([]typ.Type, len(node.Elements))
					copy(elems, node.Elements)
				}
				elems[i] = newElem
			} else if elems != nil {
				elems[i] = e
			}
		}
		if elems == nil {
			out = t
			break
		}
		out = typ.NewTuple(elems...)
	case *typ.Alias:
		target := pruneSoftUnionMembersMemo(node.Target, next, memo, visiting, softMemo)
		if target == node.Target {
			out = t
			break
		}
		out = typ.NewAlias(node.Name, target)
	case *typ.Instantiated:
		var args []typ.Type
		for idx, a := range node.TypeArgs {
			newArg := pruneSoftUnionMembersMemo(a, next, memo, visiting, softMemo)
			if newArg != a {
				if args == nil {
					args = make([]typ.Type, len(node.TypeArgs))
					copy(args, node.TypeArgs)
				}
				args[idx] = newArg
			} else if args != nil {
				args[idx] = a
			}
		}
		if args == nil {
			out = t
			break
		}
		out = typ.Instantiate(node.Generic, args...)
	default:
		out = t
	}

	delete(visiting, t)
	memo[t] = out
	return out
}

func pruneSoftFunction(
	f *typ.Function,
	orig typ.Type,
	next recursion.Guard,
	memo map[typ.Type]typ.Type,
	visiting map[typ.Type]struct{},
	softMemo map[typ.Type]bool,
) typ.Type {
	changed := false

	var params []typ.Param
	for i, p := range f.Params {
		newType := pruneSoftUnionMembersMemo(p.Type, next, memo, visiting, softMemo)
		if newType != p.Type {
			if params == nil {
				params = make([]typ.Param, len(f.Params))
				copy(params, f.Params)
			}
			changed = true
			params[i] = typ.Param{Name: p.Name, Type: newType, Optional: p.Optional}
		} else if params != nil {
			params[i] = p
		}
	}

	var returns []typ.Type
	for i, r := range f.Returns {
		newRet := pruneSoftUnionMembersMemo(r, next, memo, visiting, softMemo)
		if newRet != r {
			if returns == nil {
				returns = make([]typ.Type, len(f.Returns))
				copy(returns, f.Returns)
			}
			changed = true
			returns[i] = newRet
		} else if returns != nil {
			returns[i] = r
		}
	}

	variadic := f.Variadic
	if f.Variadic != nil {
		newVariadic := pruneSoftUnionMembersMemo(f.Variadic, next, memo, visiting, softMemo)
		if newVariadic != f.Variadic {
			changed = true
			variadic = newVariadic
		}
	}

	if !changed {
		return orig
	}

	paramSrc := f.Params
	if params != nil {
		paramSrc = params
	}
	returnsSrc := f.Returns
	if returns != nil {
		returnsSrc = returns
	}
	return typ.RebuildFunction(typ.FunctionParts{
		TypeParams: f.TypeParams,
		Params:     paramSrc,
		Variadic:   variadic,
		Returns:    returnsSrc,
	})
}

func pruneSoftRecord(
	r *typ.Record,
	orig typ.Type,
	next recursion.Guard,
	memo map[typ.Type]typ.Type,
	visiting map[typ.Type]struct{},
	softMemo map[typ.Type]bool,
) typ.Type {
	changed := false

	var fields []typ.Field
	for i, f := range r.Fields {
		newType := pruneSoftUnionMembersMemo(f.Type, next, memo, visiting, softMemo)
		if newType != f.Type {
			if fields == nil {
				fields = make([]typ.Field, len(r.Fields))
				copy(fields, r.Fields)
			}
			changed = true
			fields[i] = typ.Field{Name: f.Name, Type: newType, Optional: f.Optional, Readonly: f.Readonly}
		} else if fields != nil {
			fields[i] = f
		}
	}
	var staticMembers []typ.StaticMember
	for i, m := range r.StaticMembers {
		newType := pruneSoftUnionMembersMemo(m.Type, next, memo, visiting, softMemo)
		if newType != m.Type {
			if staticMembers == nil {
				staticMembers = make([]typ.StaticMember, len(r.StaticMembers))
				copy(staticMembers, r.StaticMembers)
			}
			changed = true
			staticMembers[i].Type = newType
		} else if staticMembers != nil {
			staticMembers[i] = m
		}
	}

	metatable := r.Metatable
	if r.Metatable != nil {
		newMetatable := pruneSoftUnionMembersMemo(r.Metatable, next, memo, visiting, softMemo)
		if newMetatable != r.Metatable {
			changed = true
			metatable = newMetatable
		}
	}

	mapKey := r.MapKey
	mapValue := r.MapValue
	if r.HasMapComponent() {
		newMapKey := pruneSoftUnionMembersMemo(r.MapKey, next, memo, visiting, softMemo)
		if newMapKey != r.MapKey {
			changed = true
			mapKey = newMapKey
		}
		newMapValue := pruneSoftUnionMembersMemo(r.MapValue, next, memo, visiting, softMemo)
		if newMapValue != r.MapValue {
			changed = true
			mapValue = newMapValue
		}
	}

	if !changed {
		return orig
	}

	fieldsSrc := r.Fields
	if fields != nil {
		fieldsSrc = fields
	}
	staticMembersSrc := r.StaticMembers
	if staticMembers != nil {
		staticMembersSrc = staticMembers
	}
	return typetable.RebuildRecord(typ.RecordParts{
		Fields:        fieldsSrc,
		StaticMembers: staticMembersSrc,
		Metatable:     metatable,
		MapKey:        mapKey,
		MapValue:      mapValue,
		Open:          r.Open,
		AssumeSorted:  true,
	})
}
