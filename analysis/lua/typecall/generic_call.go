package typecall

import (
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// ArgumentConstraintViolation describes an inferred generic argument that does
// not satisfy the declared type-parameter constraint.
type ArgumentConstraintViolation struct {
	Index      int
	Got        typ.Type
	Constraint typ.Type
}

// InstantiateGenericCall infers type arguments for a generic function call from
// concrete argument types, validates type-parameter constraints, and returns the
// callable signature with inferred type parameters substituted.
func InstantiateGenericCall(fn *typ.Function, args []typ.Type) (*typ.Function, []ArgumentConstraintViolation) {
	if fn == nil || len(fn.TypeParams) == 0 {
		return fn, nil
	}
	bindings := make(map[*typ.TypeParam]inferredArg, len(fn.TypeParams))
	for i, arg := range args {
		if arg == nil {
			continue
		}
		formal, ok := callParamType(fn, i)
		if !ok || formal == nil {
			continue
		}
		inferTypeParamBindings(formal, arg, i, bindings, 0)
	}

	params := make([]*typ.TypeParam, 0, len(fn.TypeParams))
	argsToSubstitute := make([]typ.Type, 0, len(fn.TypeParams))
	for _, param := range fn.TypeParams {
		if param == nil {
			continue
		}
		if binding, ok := bindings[param]; ok {
			params = append(params, param)
			argsToSubstitute = append(argsToSubstitute, binding.typ)
		}
	}

	var violations []ArgumentConstraintViolation
	for _, param := range fn.TypeParams {
		if param == nil || param.Constraint == nil {
			continue
		}
		binding, ok := bindings[param]
		if !ok || binding.typ == nil {
			continue
		}
		if !subtype.IsSubtype(binding.typ, param.Constraint) {
			violations = append(violations, ArgumentConstraintViolation{
				Index:      binding.index,
				Got:        binding.typ,
				Constraint: param.Constraint,
			})
		}
	}

	instantiated, ok := subst.Params(fn, params, argsToSubstitute).(*typ.Function)
	if !ok {
		return fn, violations
	}
	return instantiated, violations
}

type inferredArg struct {
	typ   typ.Type
	index int
}

func callParamType(fn *typ.Function, index int) (typ.Type, bool) {
	if fn == nil || index < 0 {
		return nil, false
	}
	if index < len(fn.Params) {
		return fn.Params[index].Type, true
	}
	if fn.Variadic != nil {
		return fn.Variadic, true
	}
	return nil, false
}

func inferTypeParamBindings(formal typ.Type, actual typ.Type, index int, bindings map[*typ.TypeParam]inferredArg, depth int) {
	if formal == nil || actual == nil || depth > typ.DefaultRecursionDepth {
		return
	}

	formal = unwrap.Annotated(formal)
	actual = unwrap.Annotated(actual)

	if param, ok := formal.(*typ.TypeParam); ok {
		bindTypeParam(param, actual, index, bindings)
		return
	}

	switch f := formal.(type) {
	case *typ.Alias:
		inferTypeParamBindings(f.UnaliasedTarget(), actual, index, bindings, depth+1)
	case *typ.Optional:
		if a, ok := actual.(*typ.Optional); ok {
			inferTypeParamBindings(f.Inner, a.Inner, index, bindings, depth+1)
			return
		}
		inferTypeParamBindings(f.Inner, actual, index, bindings, depth+1)
	case *typ.Array:
		switch a := actual.(type) {
		case *typ.Array:
			inferTypeParamBindings(f.Element, a.Element, index, bindings, depth+1)
		case *typ.Tuple:
			for _, elem := range a.Elements {
				inferTypeParamBindings(f.Element, elem, index, bindings, depth+1)
			}
		}
	case *typ.Map:
		if a, ok := actual.(*typ.Map); ok {
			inferTypeParamBindings(f.Key, a.Key, index, bindings, depth+1)
			inferTypeParamBindings(f.Value, a.Value, index, bindings, depth+1)
		}
	case *typ.ReadonlyMap:
		switch a := actual.(type) {
		case *typ.ReadonlyMap:
			inferTypeParamBindings(f.Key, a.Key, index, bindings, depth+1)
			inferTypeParamBindings(f.Value, a.Value, index, bindings, depth+1)
		case *typ.Map:
			inferTypeParamBindings(f.Key, a.Key, index, bindings, depth+1)
			inferTypeParamBindings(f.Value, a.Value, index, bindings, depth+1)
		}
	case *typ.Tuple:
		if a, ok := actual.(*typ.Tuple); ok {
			for i, elem := range f.Elements {
				if i >= len(a.Elements) {
					break
				}
				inferTypeParamBindings(elem, a.Elements[i], index, bindings, depth+1)
			}
		}
	case *typ.Function:
		inferFunctionBindings(f, actual, index, bindings, depth+1)
	case *typ.Record:
		inferRecordBindings(f, actual, index, bindings, depth+1)
	case *typ.Instantiated:
		if a, ok := actual.(*typ.Instantiated); ok && sameGeneric(f.Generic, a.Generic) {
			for i, arg := range f.TypeArgs {
				if i >= len(a.TypeArgs) {
					break
				}
				inferTypeParamBindings(arg, a.TypeArgs[i], index, bindings, depth+1)
			}
			return
		}
		expanded := subst.ExpandInstantiated(f)
		if expanded != nil && expanded != formal {
			inferTypeParamBindings(expanded, actual, index, bindings, depth+1)
		}
	case *typ.Union:
		for _, member := range f.Members {
			inferTypeParamBindings(member, actual, index, bindings, depth+1)
		}
	case *typ.Intersection:
		for _, member := range f.Members {
			inferTypeParamBindings(member, actual, index, bindings, depth+1)
		}
	}
}

func inferFunctionBindings(formal *typ.Function, actual typ.Type, index int, bindings map[*typ.TypeParam]inferredArg, depth int) {
	if formal == nil || depth > typ.DefaultRecursionDepth {
		return
	}
	actual = unwrap.Annotated(actual)
	if alias, ok := actual.(*typ.Alias); ok {
		actual = alias.UnaliasedTarget()
	}
	if inst, ok := actual.(*typ.Instantiated); ok {
		expanded := subst.ExpandInstantiated(inst)
		if expanded != nil && expanded != actual {
			actual = expanded
		}
	}
	actualFn, ok := actual.(*typ.Function)
	if !ok || actualFn == nil {
		return
	}
	for i, param := range formal.Params {
		if i >= len(actualFn.Params) {
			break
		}
		inferTypeParamBindings(param.Type, actualFn.Params[i].Type, index, bindings, depth+1)
	}
	if formal.Variadic != nil {
		if actualFn.Variadic != nil {
			inferTypeParamBindings(formal.Variadic, actualFn.Variadic, index, bindings, depth+1)
		} else {
			for i := len(formal.Params); i < len(actualFn.Params); i++ {
				inferTypeParamBindings(formal.Variadic, actualFn.Params[i].Type, index, bindings, depth+1)
			}
		}
	}
	for i, ret := range formal.Returns {
		if i >= len(actualFn.Returns) {
			break
		}
		inferTypeParamBindings(ret, actualFn.Returns[i], index, bindings, depth+1)
	}
}

func inferRecordBindings(formal *typ.Record, actual typ.Type, index int, bindings map[*typ.TypeParam]inferredArg, depth int) {
	if formal == nil {
		return
	}
	actual = unwrap.Annotated(actual)
	if alias, ok := actual.(*typ.Alias); ok {
		actual = alias.UnaliasedTarget()
	}
	if inst, ok := actual.(*typ.Instantiated); ok {
		expanded := subst.ExpandInstantiated(inst)
		if expanded != nil && expanded != actual {
			actual = expanded
		}
	}
	record, ok := actual.(*typ.Record)
	if !ok || record == nil {
		return
	}
	for _, field := range formal.Fields {
		actualField := record.GetField(field.Name)
		if actualField == nil {
			continue
		}
		inferTypeParamBindings(field.Type, actualField.Type, index, bindings, depth+1)
	}
	if formal.HasMapComponent() && record.HasMapComponent() {
		inferTypeParamBindings(formal.MapKey, record.MapKey, index, bindings, depth+1)
		inferTypeParamBindings(formal.MapValue, record.MapValue, index, bindings, depth+1)
	}
	for _, member := range formal.StaticMembers {
		var actualMember *typ.StaticMember
		switch member.Kind {
		case typ.StaticMemberStringIndex:
			actualMember = record.GetStaticStringIndex(member.Name)
		case typ.StaticMemberIntIndex:
			actualMember = record.GetStaticIntIndex(member.Index)
		}
		if actualMember != nil {
			inferTypeParamBindings(member.Type, actualMember.Type, index, bindings, depth+1)
		}
	}
}

func sameGeneric(left *typ.Generic, right *typ.Generic) bool {
	if left == nil || right == nil {
		return false
	}
	return left == right || typ.SameNodeOrAcyclicEqual(left, right)
}

func bindTypeParam(param *typ.TypeParam, actual typ.Type, index int, bindings map[*typ.TypeParam]inferredArg) {
	if param == nil || actual == nil {
		return
	}
	if existing, ok := bindings[param]; ok {
		bindings[param] = inferredArg{typ: mergeInferredTypes(existing.typ, actual), index: existing.index}
		return
	}
	for known, existing := range bindings {
		if known != nil && known.Equals(param) {
			bindings[known] = inferredArg{typ: mergeInferredTypes(existing.typ, actual), index: existing.index}
			return
		}
	}
	bindings[param] = inferredArg{typ: actual, index: index}
}

func mergeInferredTypes(left typ.Type, right typ.Type) typ.Type {
	if left == nil {
		return right
	}
	if right == nil || typ.SameNodeOrAcyclicEqual(left, right) {
		return left
	}
	if subtype.IsSubtype(left, right) {
		return right
	}
	if subtype.IsSubtype(right, left) {
		return left
	}
	return normalize.UnionForEvidence(left, right)
}
