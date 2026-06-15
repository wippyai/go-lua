package typecall

import (
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/refinement"
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
	violations = append(violations, validateInstantiatedArguments(instantiated, args)...)
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

func validateInstantiatedArguments(fn *typ.Function, args []typ.Type) []ArgumentConstraintViolation {
	if fn == nil || len(args) == 0 {
		return nil
	}
	var violations []ArgumentConstraintViolation
	for i, actual := range args {
		if actual == nil {
			continue
		}
		if refinement.ContainsFreeTypeParam(actual) {
			continue
		}
		formal, ok := callParamType(fn, i)
		if !ok || formal == nil || refinement.ContainsFreeTypeParam(formal) {
			continue
		}
		if instantiatedArgumentAssignable(actual, formal, 0) {
			continue
		}
		violations = append(violations, ArgumentConstraintViolation{
			Index:      i,
			Got:        actual,
			Constraint: formal,
		})
	}
	return violations
}

func instantiatedArgumentAssignable(actual typ.Type, formal typ.Type, depth int) bool {
	if actual == nil || formal == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	actual = unwrap.Annotated(actual)
	formal = unwrap.Annotated(formal)
	if formal == nil || refinement.ContainsFreeTypeParam(formal) {
		return true
	}

	if actualInst, ok := actual.(*typ.Instantiated); ok {
		if formalInst, ok := formal.(*typ.Instantiated); ok && sameGeneric(actualInst.Generic, formalInst.Generic) {
			return instantiatedArgsAssignable(actualInst, formalInst, depth+1)
		}
	}

	switch f := formal.(type) {
	case *typ.Alias:
		return instantiatedArgumentAssignable(actual, f.UnaliasedTarget(), depth+1)
	case *typ.Optional:
		if a, ok := actual.(*typ.Optional); ok {
			return instantiatedArgumentAssignable(a.Inner, f.Inner, depth+1)
		}
		return instantiatedArgumentAssignable(actual, f.Inner, depth+1)
	case *typ.Instantiated:
		expanded := expandFormalInstantiatedForInference(f)
		if expanded != nil && expanded != formal {
			return instantiatedArgumentAssignable(actual, expanded, depth+1)
		}
	case *typ.Record:
		if actualRecord, ok := actualRecordForValidation(actual, depth+1); ok {
			return providedRecordFieldsAssignable(actualRecord, f, depth+1)
		}
	}
	if underSpecifiedFunctionLiteral(actual, formal) {
		return true
	}

	return subtype.IsFreshAssignable(actual, formal)
}

func underSpecifiedFunctionLiteral(actual typ.Type, formal typ.Type) bool {
	actualFn, actualOK := actual.(*typ.Function)
	formalFn, formalOK := formal.(*typ.Function)
	if !actualOK || !formalOK || actualFn == nil || formalFn == nil {
		return false
	}
	return len(actualFn.Returns) == 0 && len(formalFn.Returns) != 0
}

func instantiatedArgsAssignable(actual *typ.Instantiated, formal *typ.Instantiated, depth int) bool {
	if actual == nil || formal == nil || len(actual.TypeArgs) != len(formal.TypeArgs) {
		return false
	}
	for i, actualArg := range actual.TypeArgs {
		formalArg := formal.TypeArgs[i]
		if subtype.IsSubtype(actualArg, formalArg) && subtype.IsSubtype(formalArg, actualArg) {
			continue
		}
		if freshPrecisionArgumentAssignable(actualArg, formalArg, depth+1) {
			continue
		}
		return false
	}
	return true
}

func freshPrecisionArgumentAssignable(actual typ.Type, formal typ.Type, depth int) bool {
	if actual == nil || formal == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	if !subtype.IsFreshAssignable(actual, formal) {
		return false
	}
	return hasFreshPrecisionShape(actual, formal, depth+1)
}

func hasFreshPrecisionShape(actual typ.Type, formal typ.Type, depth int) bool {
	if actual == nil || formal == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	actual = unwrap.Annotated(actual)
	formal = unwrap.Annotated(formal)
	if alias, ok := actual.(*typ.Alias); ok {
		return hasFreshPrecisionShape(alias.UnaliasedTarget(), formal, depth+1)
	}
	if alias, ok := formal.(*typ.Alias); ok {
		return hasFreshPrecisionShape(actual, alias.UnaliasedTarget(), depth+1)
	}
	if subtype.IsSubtype(actual, formal) && subtype.IsSubtype(formal, actual) {
		return false
	}
	if _, ok := actual.(*typ.Literal); ok {
		return true
	}
	if opt, ok := actual.(*typ.Optional); ok {
		if formalOpt, ok := formal.(*typ.Optional); ok {
			return hasFreshPrecisionShape(opt.Inner, formalOpt.Inner, depth+1)
		}
		return hasFreshPrecisionShape(opt.Inner, formal, depth+1)
	}
	if opt, ok := formal.(*typ.Optional); ok {
		return hasFreshPrecisionShape(actual, opt.Inner, depth+1)
	}
	if union, ok := actual.(*typ.Union); ok {
		for _, member := range union.Members {
			if hasFreshPrecisionShape(member, formal, depth+1) {
				return true
			}
		}
		return false
	}
	if union, ok := formal.(*typ.Union); ok {
		for _, member := range union.Members {
			if subtype.IsFreshAssignable(actual, member) && hasFreshPrecisionShape(actual, member, depth+1) {
				return true
			}
		}
		return false
	}
	if actualInst, ok := actual.(*typ.Instantiated); ok {
		if formalInst, ok := formal.(*typ.Instantiated); ok && sameGeneric(actualInst.Generic, formalInst.Generic) {
			for i, actualArg := range actualInst.TypeArgs {
				if i >= len(formalInst.TypeArgs) {
					return false
				}
				if hasFreshPrecisionShape(actualArg, formalInst.TypeArgs[i], depth+1) {
					return true
				}
			}
			return false
		}
	}
	actualRecord, actualOK := actualRecordForValidation(actual, depth+1)
	formalRecord, formalOK := actualRecordForValidation(formal, depth+1)
	if actualOK && formalOK {
		for _, field := range actualRecord.Fields {
			formalField := formalRecord.GetField(field.Name)
			if formalField == nil || formalField.Type == nil || field.Type == nil {
				continue
			}
			if hasFreshPrecisionShape(field.Type, formalField.Type, depth+1) {
				return true
			}
		}
		for _, member := range actualRecord.StaticMembers {
			formalMember := formalRecord.GetStaticMember(member.Kind, member.Name, member.Index)
			if formalMember == nil || formalMember.Type == nil || member.Type == nil {
				continue
			}
			if hasFreshPrecisionShape(member.Type, formalMember.Type, depth+1) {
				return true
			}
		}
		if actualRecord.HasMapComponent() && formalRecord.HasMapComponent() {
			return hasFreshPrecisionShape(actualRecord.MapKey, formalRecord.MapKey, depth+1) ||
				hasFreshPrecisionShape(actualRecord.MapValue, formalRecord.MapValue, depth+1)
		}
		return false
	}
	actualFn, actualFnOK := actual.(*typ.Function)
	formalFn, formalFnOK := formal.(*typ.Function)
	if actualFnOK && formalFnOK {
		for i, ret := range actualFn.Returns {
			if i >= len(formalFn.Returns) {
				break
			}
			if hasFreshPrecisionShape(ret, formalFn.Returns[i], depth+1) {
				return true
			}
		}
		return false
	}
	return false
}

func actualRecordForValidation(actual typ.Type, depth int) (*typ.Record, bool) {
	if actual == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	actual = unwrap.Annotated(actual)
	switch a := actual.(type) {
	case *typ.Alias:
		return actualRecordForValidation(a.UnaliasedTarget(), depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(a)
		if expanded != nil && expanded != actual {
			return actualRecordForValidation(expanded, depth+1)
		}
	case *typ.Record:
		return a, true
	}
	return nil, false
}

func providedRecordFieldsAssignable(actual *typ.Record, formal *typ.Record, depth int) bool {
	if actual == nil || formal == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	for _, field := range actual.Fields {
		formalField := formal.GetField(field.Name)
		if formalField == nil || formalField.Type == nil || field.Type == nil {
			continue
		}
		if !instantiatedArgumentAssignable(field.Type, formalField.Type, depth+1) {
			return false
		}
	}
	for _, member := range actual.StaticMembers {
		formalMember := formal.GetStaticMember(member.Kind, member.Name, member.Index)
		if formalMember == nil || formalMember.Type == nil || member.Type == nil {
			continue
		}
		if !instantiatedArgumentAssignable(member.Type, formalMember.Type, depth+1) {
			return false
		}
	}
	if actual.HasMapComponent() && formal.HasMapComponent() {
		return instantiatedArgumentAssignable(actual.MapKey, formal.MapKey, depth+1) &&
			instantiatedArgumentAssignable(actual.MapValue, formal.MapValue, depth+1)
	}
	return true
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
		expanded := expandFormalInstantiatedForInference(f)
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

func expandFormalInstantiatedForInference(inst *typ.Instantiated) typ.Type {
	if inst == nil || inst.Generic == nil ||
		inst.Generic.Body == nil ||
		len(inst.TypeArgs) != len(inst.Generic.TypeParams) {
		return inst
	}
	body := subst.Params(inst.Generic.Body, inst.Generic.TypeParams, inst.TypeArgs)
	return subst.Self(body, inst)
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
	if union, ok := actual.(*typ.Union); ok {
		for _, member := range union.Members {
			inferRecordBindings(formal, member, index, bindings, depth+1)
		}
		return
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
